package saga

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nself-org/plugins-pro/paid/nself-cloud/internal/hetzner"
	"github.com/nself-org/plugins-pro/paid/nself-cloud/internal/stripe"
)

// ProvisionOpts holds all parameters required to provision a new cloud instance.
type ProvisionOpts struct {
	TenantID        string
	InstanceID      string // UUID, must already exist in np_cloud_instances as 'pending'
	StripeCustomerID string
	AmountCents     int
	Currency        string
	ServerName      string // unique slug-based name
	ServerType      string // e.g. "cx23"
	Region          string // e.g. "fsn1"
	SSHKeyID        int64
	UserData        string
	Labels          map[string]string
}

// Provisioner orchestrates the full provisioning saga:
//
//	charging → charged → provisioning → ready
//
// CHAOS-CRITICAL #2: Stripe charge only after Hetzner success.
// Compensating cancel (Stripe refund) on Hetzner failure.
//
// Wait — the brief specifies Stripe FIRST, then Hetzner, with refund compensation.
// Keeping that order as specified.
type Provisioner struct {
	machine *Machine
	sdb     *sql.DB
	stripe  *stripe.Client
	hetzner *hetzner.Client
	// pollInterval controls how often we check Hetzner server status.
	pollInterval time.Duration
	// provisionTimeout is the maximum time to wait for a server to become running.
	provisionTimeout time.Duration
}

// NewProvisioner creates a Provisioner with the given dependencies.
func NewProvisioner(sdb *sql.DB, stripeClient *stripe.Client, hetznerClient *hetzner.Client) *Provisioner {
	return &Provisioner{
		machine:          New(NewSQLDB(sdb)),
		sdb:              sdb,
		stripe:           stripeClient,
		hetzner:          hetznerClient,
		pollInterval:     10 * time.Second,
		provisionTimeout: 10 * time.Minute,
	}
}

// ProvisionInstance executes the full provisioning saga for a new instance.
// Steps:
//  1. Set state=charging; charge Stripe
//  2. On Stripe success → state=charged
//  3. On Stripe fail → state=failed (no compensation needed, no server yet)
//  4. Create server on Hetzner
//  5. On Hetzner success → state=provisioning, store hetzner_server_id + IPs
//  6. On Hetzner fail → refund Stripe → state=failed
//  7. Poll Hetzner until server_status=running → state=ready
//
// The function is idempotent: if a step is already complete (state already advanced),
// it resumes from the current state.
func (p *Provisioner) ProvisionInstance(ctx context.Context, opts ProvisionOpts) error {
	current, err := p.machine.db.CurrentState(ctx, opts.InstanceID)
	if err != nil {
		return fmt.Errorf("provision: read state: %w", err)
	}

	// Resume from current state — idempotent.
	switch current {
	case StatePending:
		if err := p.doCharge(ctx, opts); err != nil {
			return err
		}
		fallthrough
	case StateCharged:
		if err := p.doHetzner(ctx, opts); err != nil {
			return err
		}
		fallthrough
	case StateProvisioning:
		return p.doPollReady(ctx, opts.InstanceID)
	case StateReady:
		return nil // already done
	case StateFailed:
		return fmt.Errorf("provision: instance %s is in failed state", opts.InstanceID)
	default:
		// StateCharging = in-flight; treat as pending and re-try charge.
		if err := p.doCharge(ctx, opts); err != nil {
			return err
		}
		if err := p.doHetzner(ctx, opts); err != nil {
			return err
		}
		return p.doPollReady(ctx, opts.InstanceID)
	}
}

// doCharge handles the charging → charged transition.
func (p *Provisioner) doCharge(ctx context.Context, opts ProvisionOpts) error {
	// Transition to charging.
	if err := p.machine.Transition(ctx, opts.InstanceID, StateCharging, nil); err != nil {
		return fmt.Errorf("provision: → charging: %w", err)
	}

	// Idempotency key per instance — safe to retry.
	idempotencyKey := fmt.Sprintf("provision:%s", opts.InstanceID)
	currency := opts.Currency
	if currency == "" {
		currency = "usd"
	}

	result, err := p.stripe.Charge(ctx,
		opts.StripeCustomerID,
		opts.AmountCents,
		currency,
		idempotencyKey,
		fmt.Sprintf("nself-cloud instance %s", opts.ServerName),
	)
	if err != nil {
		_ = p.machine.Fail(ctx, opts.InstanceID, fmt.Sprintf("stripe charge failed: %v", err))
		// Log the failed billing event.
		_ = p.machine.db.LogBillingEvent(ctx, BillingEvent{
			TenantID:    opts.TenantID,
			InstanceID:  opts.InstanceID,
			EventType:   "invoice_failed",
			Provider:    "stripe",
			AmountCents: opts.AmountCents,
			Currency:    currency,
			Metadata:    map[string]interface{}{"error": err.Error()},
		})
		return fmt.Errorf("provision: stripe charge: %w", err)
	}

	// Log the successful charge.
	_ = p.machine.db.LogBillingEvent(ctx, BillingEvent{
		TenantID:        opts.TenantID,
		InstanceID:      opts.InstanceID,
		EventType:       "subscription_created",
		Provider:        "stripe",
		ProviderEventID: result.ChargeID,
		AmountCents:     opts.AmountCents,
		Currency:        currency,
		IdempotencyKey:  idempotencyKey,
		Metadata:        map[string]interface{}{"charge_id": result.ChargeID, "paid": result.Paid},
	})

	// Transition to charged — store charge ID in metadata.
	return p.machine.Transition(ctx, opts.InstanceID, StateCharged, map[string]interface{}{
		"stripe_charge_id": result.ChargeID,
	})
}

// doHetzner handles the charged → provisioning transition with Hetzner server creation.
func (p *Provisioner) doHetzner(ctx context.Context, opts ProvisionOpts) error {
	// Read existing charge ID for compensating refund if needed.
	chargeID, err := p.readChargeID(ctx, opts.InstanceID)
	if err != nil {
		// Non-fatal: we can still attempt refund with what we have.
		chargeID = ""
	}

	serverResult, err := p.hetzner.CreateServer(ctx, hetzner.ServerOpts{
		Name:       opts.ServerName,
		ServerType: opts.ServerType,
		Image:      "ubuntu-22.04",
		Location:   opts.Region,
		SSHKeyID:   opts.SSHKeyID,
		UserData:   opts.UserData,
		Labels:     opts.Labels,
	})
	if err != nil {
		// Compensating action: refund the Stripe charge.
		if chargeID != "" {
			refundKey := fmt.Sprintf("refund:%s", opts.InstanceID)
			refundErr := p.stripe.Refund(ctx, chargeID, refundKey)
			if refundErr != nil {
				// Log the refund failure but return the Hetzner error.
				_ = p.machine.db.LogBillingEvent(ctx, BillingEvent{
					TenantID:   opts.TenantID,
					InstanceID: opts.InstanceID,
					EventType:  "refund_issued",
					Provider:   "stripe",
					Metadata:   map[string]interface{}{"error": refundErr.Error(), "refund_failed": true},
				})
			} else {
				_ = p.machine.db.LogBillingEvent(ctx, BillingEvent{
					TenantID:   opts.TenantID,
					InstanceID: opts.InstanceID,
					EventType:  "refund_issued",
					Provider:   "stripe",
					Metadata:   map[string]interface{}{"charge_id": chargeID, "reason": "hetzner_provision_failed"},
				})
			}
		}

		_ = p.machine.Fail(ctx, opts.InstanceID, fmt.Sprintf("hetzner create server: %v", err))
		return fmt.Errorf("provision: hetzner create: %w", err)
	}

	// Store hetzner_server_id and IPs, transition to provisioning.
	if dbErr := p.storeServerDetails(ctx, opts.InstanceID, serverResult); dbErr != nil {
		return fmt.Errorf("provision: store server details: %w", dbErr)
	}

	return p.machine.Transition(ctx, opts.InstanceID, StateProvisioning, map[string]interface{}{
		"hetzner_server_id": serverResult.ServerID,
		"ipv4":              serverResult.IPv4,
		"ipv6":              serverResult.IPv6,
	})
}

// doPollReady polls Hetzner until the server reaches 'running' status.
func (p *Provisioner) doPollReady(ctx context.Context, instanceID string) error {
	serverID, err := p.readServerID(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("provision: read hetzner server id: %w", err)
	}

	deadline := time.Now().Add(p.provisionTimeout)
	for {
		if time.Now().After(deadline) {
			_ = p.machine.Fail(ctx, instanceID, "provision timeout waiting for Hetzner server running")
			return fmt.Errorf("provision: timeout waiting for server %d to become running", serverID)
		}

		status, err := p.hetzner.GetServerStatus(ctx, serverID)
		if err != nil {
			// Non-fatal poll error — continue.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(p.pollInterval):
				continue
			}
		}

		if status == hetzner.ServerStatusRunning {
			return p.machine.Transition(ctx, instanceID, StateReady, map[string]interface{}{
				"server_status": string(status),
			})
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(p.pollInterval):
		}
	}
}

// storeServerDetails writes hetzner_server_id and IPs to np_cloud_instances.
func (p *Provisioner) storeServerDetails(ctx context.Context, instanceID string, sr hetzner.ServerResult) error {
	_, err := p.sdb.ExecContext(ctx,
		`UPDATE np_cloud_instances
		    SET hetzner_server_id = $1, ipv4 = $2, ipv6 = $3, updated_at = now()
		  WHERE id = $4`,
		sr.ServerID, sr.IPv4, sr.IPv6, instanceID,
	)
	return err
}

// readServerID reads hetzner_server_id from np_cloud_instances.
func (p *Provisioner) readServerID(ctx context.Context, instanceID string) (int64, error) {
	var id sql.NullInt64
	err := p.sdb.QueryRowContext(ctx,
		`SELECT hetzner_server_id FROM np_cloud_instances WHERE id = $1`,
		instanceID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("readServerID: %w", err)
	}
	if !id.Valid {
		return 0, fmt.Errorf("readServerID: hetzner_server_id is NULL for instance %s", instanceID)
	}
	return id.Int64, nil
}

// readChargeID reads the most recent Stripe charge ID from np_cloud_billing_events.
func (p *Provisioner) readChargeID(ctx context.Context, instanceID string) (string, error) {
	var evtID sql.NullString
	err := p.sdb.QueryRowContext(ctx,
		`SELECT provider_event_id
		   FROM np_cloud_billing_events
		  WHERE instance_id = $1
		    AND event_type = 'subscription_created'
		    AND provider = 'stripe'
		  ORDER BY created_at DESC
		  LIMIT 1`,
		instanceID,
	).Scan(&evtID)
	if err != nil {
		return "", err
	}
	if !evtID.Valid {
		return "", nil
	}
	return evtID.String, nil
}
