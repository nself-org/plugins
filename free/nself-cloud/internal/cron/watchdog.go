// Package cron provides background job runners for nself-cloud.
package cron

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

const (
	// watchdogInterval is how often the watchdog scans for stuck instances.
	watchdogInterval = 5 * time.Minute
	// stuckThreshold is how long an instance must be in a non-terminal state before
	// the watchdog considers it stuck.
	stuckThreshold = 15 * time.Minute
)

// Alerter is the interface for sending watchdog alerts.
// Implemented by the nself-alert-router plugin integration or a log-only stub.
type Alerter interface {
	Alert(ctx context.Context, instanceID, reason string) error
}

// logAlerter is a no-op alerter that logs to stderr (used when no alert router is wired).
type logAlerter struct{}

func (l *logAlerter) Alert(_ context.Context, instanceID, reason string) error {
	log.Printf("[watchdog] ALERT instance=%s reason=%q", instanceID, reason)
	return nil
}

// StuckInstance describes an instance found stuck by the watchdog.
type StuckInstance struct {
	InstanceID      string
	TenantID        string
	State           string
	StuckSince      time.Time
	StripeChargeID  string
	HetznerServerID sql.NullInt64
}

// Watchdog monitors np_cloud_instances for saga jobs stuck in non-terminal states.
type Watchdog struct {
	db      *sql.DB
	alerter Alerter
}

// NewWatchdog creates a new Watchdog.
// If alerter is nil, a log-only alerter is used.
func NewWatchdog(db *sql.DB, alerter Alerter) *Watchdog {
	if alerter == nil {
		alerter = &logAlerter{}
	}
	return &Watchdog{db: db, alerter: alerter}
}

// Run starts the watchdog loop. Blocks until ctx is cancelled.
func (w *Watchdog) Run(ctx context.Context) {
	ticker := time.NewTicker(watchdogInterval)
	defer ticker.Stop()

	log.Printf("[watchdog] started (interval=%s, stuck_threshold=%s)", watchdogInterval, stuckThreshold)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[watchdog] stopping: %v", ctx.Err())
			return
		case <-ticker.C:
			if err := w.scan(ctx); err != nil {
				log.Printf("[watchdog] scan error: %v", err)
			}
		}
	}
}

// scan finds stuck instances and applies compensating actions.
func (w *Watchdog) scan(ctx context.Context) error {
	stuck, err := w.findStuck(ctx)
	if err != nil {
		return fmt.Errorf("watchdog: find stuck: %w", err)
	}

	for _, inst := range stuck {
		log.Printf("[watchdog] stuck instance=%s state=%s since=%s",
			inst.InstanceID, inst.State, inst.StuckSince.Format(time.RFC3339))

		if compErr := w.compensate(ctx, inst); compErr != nil {
			log.Printf("[watchdog] compensate error instance=%s: %v", inst.InstanceID, compErr)
		}

		if alertErr := w.alerter.Alert(ctx, inst.InstanceID,
			fmt.Sprintf("stuck in state %s since %s", inst.State, inst.StuckSince.Format(time.RFC3339)),
		); alertErr != nil {
			log.Printf("[watchdog] alert error instance=%s: %v", inst.InstanceID, alertErr)
		}
	}
	return nil
}

// findStuck queries np_cloud_instances for rows stuck in non-terminal states > stuckThreshold.
func (w *Watchdog) findStuck(ctx context.Context) ([]StuckInstance, error) {
	cutoff := time.Now().UTC().Add(-stuckThreshold)
	rows, err := w.db.QueryContext(ctx,
		`SELECT i.id, i.tenant_id, i.status, i.updated_at,
		        i.hetzner_server_id,
		        COALESCE(b.provider_event_id, '') AS stripe_charge_id
		   FROM np_cloud_instances i
		   LEFT JOIN LATERAL (
		        SELECT provider_event_id
		          FROM np_cloud_billing_events
		         WHERE instance_id = i.id
		           AND event_type = 'subscription_created'
		           AND provider = 'stripe'
		         ORDER BY created_at DESC
		         LIMIT 1
		   ) b ON true
		  WHERE i.status NOT IN ('running', 'provision_failed', 'torn_down', 'suspended')
		    AND i.updated_at < $1`,
		cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("watchdog: query stuck: %w", err)
	}
	defer rows.Close()

	var result []StuckInstance
	for rows.Next() {
		var inst StuckInstance
		if err := rows.Scan(
			&inst.InstanceID, &inst.TenantID, &inst.State, &inst.StuckSince,
			&inst.HetznerServerID, &inst.StripeChargeID,
		); err != nil {
			return nil, fmt.Errorf("watchdog: scan row: %w", err)
		}
		result = append(result, inst)
	}
	return result, rows.Err()
}

// compensate applies the appropriate compensating action for a stuck instance.
//
//	pending/charging: Stripe may have been charged — if charge exists, refund it.
//	charged:          Hetzner creation in flight — mark failed.
//	provisioning:     Hetzner server exists — mark failed (server left running, alert operator).
func (w *Watchdog) compensate(ctx context.Context, inst StuckInstance) error {
	switch inst.State {
	case "pending":
		// No charge yet — just mark failed.
		return w.markFailed(ctx, inst.InstanceID, "watchdog: stuck in pending")

	case "charging", "provisioning":
		// charging: charge may have been issued; attempt refund if we have a charge ID.
		// provisioning: Hetzner server exists; alert is sent but we don't auto-delete (operator decision).
		if inst.StripeChargeID != "" {
			if err := w.refundCharge(ctx, inst); err != nil {
				log.Printf("[watchdog] refund failed instance=%s: %v", inst.InstanceID, err)
				// Continue to mark failed even if refund fails.
			}
		}
		return w.markFailed(ctx, inst.InstanceID,
			fmt.Sprintf("watchdog: stuck in %s for >%s", inst.State, stuckThreshold))

	default:
		return w.markFailed(ctx, inst.InstanceID,
			fmt.Sprintf("watchdog: stuck in %s for >%s", inst.State, stuckThreshold))
	}
}

// markFailed sets np_cloud_instances.status to 'provision_failed' and logs a billing event.
func (w *Watchdog) markFailed(ctx context.Context, instanceID, reason string) error {
	_, err := w.db.ExecContext(ctx,
		`UPDATE np_cloud_instances
		    SET status = 'provision_failed', updated_at = now()
		  WHERE id = $1
		    AND status NOT IN ('running', 'provision_failed', 'torn_down', 'suspended')`,
		instanceID,
	)
	if err != nil {
		return fmt.Errorf("watchdog: mark failed: %w", err)
	}

	// Log the watchdog failure event.
	_, err = w.db.ExecContext(ctx,
		`INSERT INTO np_cloud_billing_events
		    (tenant_id, instance_id, event_type, provider, metadata, status)
		 SELECT tenant_id, $1, 'invoice_failed', 'stripe',
		        jsonb_build_object('reason', $2, 'source', 'watchdog'),
		        'processed'
		   FROM np_cloud_instances WHERE id = $1`,
		instanceID, reason,
	)
	if err != nil {
		return fmt.Errorf("watchdog: log failure event: %w", err)
	}
	return nil
}

// refundCharge issues a Stripe refund via a direct API call.
// The Stripe client is not injected into Watchdog to keep it lightweight;
// it reads credentials from env directly.
func (w *Watchdog) refundCharge(ctx context.Context, inst StuckInstance) error {
	// Log the refund attempt even if Stripe client fails to build.
	_, logErr := w.db.ExecContext(ctx,
		`INSERT INTO np_cloud_billing_events
		    (tenant_id, instance_id, event_type, provider, provider_event_id, metadata, status)
		 VALUES ($1, $2, 'refund_issued', 'stripe', $3,
		         jsonb_build_object('reason', 'watchdog_stuck_compensation',
		                            'charge_id', $3),
		         'pending')`,
		inst.TenantID, inst.InstanceID, inst.StripeChargeID,
	)
	if logErr != nil {
		log.Printf("[watchdog] log refund event error: %v", logErr)
	}

	// Best-effort HTTP refund call. In production the nself-stripe plugin handles this;
	// watchdog uses a direct HTTP call as a fallback.
	return issueStripeRefundHTTP(ctx, inst.StripeChargeID,
		fmt.Sprintf("watchdog-refund:%s", inst.InstanceID))
}
