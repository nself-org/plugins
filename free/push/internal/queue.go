package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DispatchJob holds all data needed to dispatch a single push notification.
type DispatchJob struct {
	OutboxID    string
	DeviceToken string
	Platform    string
	Payload     json.RawMessage
	Attempts    int
}

// Dispatcher processes push notification dispatch jobs with retry logic.
// It uses a Postgres-backed retry loop (no external message broker required)
// so the push plugin has zero dependencies beyond Redis (for locking) and Postgres.
//
// Architecture note: the spec calls for a "BullMQ retry queue". Because the push
// plugin is Go (not Node.js), we implement the equivalent retry semantics directly:
// - Outbox row status tracks queue state (pending → queued → delivered/retrying/failed)
// - Exponential backoff is enforced via time.Sleep between attempts
// - Redis dependency is declared in plugin.json so G14-T03 auto-enables it;
//   the retry loop itself is in-process to avoid requiring a separate worker process.
type Dispatcher struct {
	pool       *pgxpool.Pool
	apns       *APNsClient
	fcm        *FCMClient
	cfg        *Config
}

// NewDispatcher creates a Dispatcher with the given clients.
// Either apns or fcm may be nil (provider not configured).
func NewDispatcher(pool *pgxpool.Pool, apns *APNsClient, fcm *FCMClient, cfg *Config) *Dispatcher {
	return &Dispatcher{pool: pool, apns: apns, fcm: fcm, cfg: cfg}
}

// Dispatch handles a single outbox job with retry + exponential backoff.
// It updates the outbox row status after each attempt.
// This is called from the HTTP handler (synchronous for the first attempt)
// and re-called from the retry goroutine for subsequent attempts.
func (d *Dispatcher) Dispatch(ctx context.Context, job DispatchJob) error {
	maxAttempts := d.cfg.RetryMaxAttempts
	backoffBase := time.Duration(d.cfg.RetryBackoffBaseMs) * time.Millisecond

	// Mark as queued.
	if err := UpdateOutboxStatus(ctx, d.pool, job.OutboxID, StatusQueued, job.Attempts, nil); err != nil {
		return fmt.Errorf("mark queued: %w", err)
	}

	for attempt := job.Attempts + 1; attempt <= maxAttempts; attempt++ {
		success, errMsg := d.sendToProvider(ctx, job.DeviceToken, job.Platform, job.Payload)

		if success {
			if err := UpdateOutboxStatus(ctx, d.pool, job.OutboxID, StatusDelivered, attempt, nil); err != nil {
				log.Printf("[push] WARNING: delivered but failed to update outbox %s: %v", job.OutboxID, err)
			}
			return nil
		}

		// Log errMsg but never include raw credential content (the msg comes from
		// our APNsResult/FCMResult which sanitizes provider tokens from outputs).
		log.Printf("[push] attempt %d/%d failed for outbox %s: %s", attempt, maxAttempts, job.OutboxID, errMsg)

		if attempt < maxAttempts {
			// Mark as retrying, then back off.
			errMsgPtr := &errMsg
			if err := UpdateOutboxStatus(ctx, d.pool, job.OutboxID, StatusRetrying, attempt, errMsgPtr); err != nil {
				log.Printf("[push] WARNING: failed to update retrying status for %s: %v", job.OutboxID, err)
			}
			backoff := exponentialBackoff(backoffBase, attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		} else {
			// Final attempt failed.
			errMsgPtr := &errMsg
			if err := UpdateOutboxStatus(ctx, d.pool, job.OutboxID, StatusFailed, attempt, errMsgPtr); err != nil {
				log.Printf("[push] WARNING: failed to update failed status for %s: %v", job.OutboxID, err)
			}
			return fmt.Errorf("push failed after %d attempts: %s", maxAttempts, errMsg)
		}
	}

	return nil
}

// sendToProvider sends the notification to the appropriate provider (APNs or FCM)
// and returns (success, errorMessage). The error message is safe to store in the DB
// (no credentials, no device tokens — just provider error codes and reason strings).
func (d *Dispatcher) sendToProvider(ctx context.Context, deviceToken, platform string, payload json.RawMessage) (bool, string) {
	switch platform {
	case "ios":
		if d.apns == nil {
			return false, "APNs not configured (set PUSH_APNS_TEAM_ID, PUSH_APNS_KEY_ID, PUSH_APNS_KEY_PEM, PUSH_APNS_BUNDLE_ID)"
		}
		result := d.apns.Send(ctx, deviceToken, payload)
		return result.Success, result.Error

	case "android":
		if d.fcm == nil {
			return false, "FCM not configured (set PUSH_FCM_PROJECT_ID, PUSH_FCM_SERVICE_ACCOUNT_JSON)"
		}
		result := d.fcm.Send(ctx, deviceToken, payload)
		return result.Success, result.Error

	default:
		return false, fmt.Sprintf("unknown platform %q (must be 'ios' or 'android')", platform)
	}
}

// exponentialBackoff returns base * 2^(attempt-1) capped at 30 seconds.
// attempt starts at 1.
func exponentialBackoff(base time.Duration, attempt int) time.Duration {
	exp := math.Pow(2, float64(attempt-1))
	d := time.Duration(float64(base) * exp)
	cap := 30 * time.Second
	if d > cap {
		return cap
	}
	return d
}
