// Package saga implements the provisioning saga state machine for nself-cloud.
// CHAOS-CRITICAL: Stripe charge occurs ONLY after Hetzner success; Stripe refund is
// issued as compensating action on Hetzner failure.
package saga

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// State represents the provisioning saga state for a cloud instance.
// Valid transitions:
//
//	pending     → charging
//	charging    → charged
//	charged     → provisioning
//	provisioning → ready
//	*           → failed  (from any non-terminal state)
type State string

const (
	StatePending      State = "pending"
	StateCharging     State = "charging"
	StateCharged      State = "charged"
	StateProvisioning State = "provisioning"
	StateReady        State = "ready"
	StateFailed       State = "failed"
)

// terminalStates are states that cannot transition further.
var terminalStates = map[State]bool{
	StateReady:  true,
	StateFailed: true,
}

// allowedTransitions maps source state → set of valid target states.
var allowedTransitions = map[State]map[State]bool{
	StatePending:      {StateCharging: true, StateFailed: true},
	StateCharging:     {StateCharged: true, StateFailed: true},
	StateCharged:      {StateProvisioning: true, StateFailed: true},
	StateProvisioning: {StateReady: true, StateFailed: true},
}

// ErrInvalidTransition is returned when a state transition is not allowed.
type ErrInvalidTransition struct {
	From State
	To   State
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("saga: invalid transition %s → %s", e.From, e.To)
}

// ErrAlreadyTerminal is returned when attempting to transition a terminal state.
type ErrAlreadyTerminal struct {
	Current State
}

func (e ErrAlreadyTerminal) Error() string {
	return fmt.Sprintf("saga: state %s is already terminal", e.Current)
}

// ValidateTransition checks whether the transition from → to is allowed.
// Returns nil if from == to (idempotent no-op — already at target).
func ValidateTransition(from, to State) error {
	if from == to {
		return nil // idempotent: already at target state
	}
	if terminalStates[from] {
		return ErrAlreadyTerminal{Current: from}
	}
	if !allowedTransitions[from][to] {
		return ErrInvalidTransition{From: from, To: to}
	}
	return nil
}

// DB is the minimal interface needed by the state machine for persistence.
type DB interface {
	TransitionState(ctx context.Context, instanceID string, from, to State, meta map[string]interface{}) error
	CurrentState(ctx context.Context, instanceID string) (State, error)
	LogBillingEvent(ctx context.Context, evt BillingEvent) error
}

// BillingEvent records a transition in np_cloud_billing_events.
type BillingEvent struct {
	TenantID        string
	InstanceID      string
	EventType       string // 'subscription_created', 'refund_issued', etc.
	Provider        string // 'stripe' | 'lemonsqueezy'
	ProviderEventID string
	AmountCents     int
	Currency        string
	IdempotencyKey  string
	Metadata        map[string]interface{}
}

// Machine is the provisioning saga state machine.
type Machine struct {
	db DB
}

// New creates a new saga Machine backed by db.
func New(db DB) *Machine {
	return &Machine{db: db}
}

// Transition advances instanceID from its current state to next.
// If the instance is already at next (idempotent retry), returns nil without writing.
// All transitions are logged to np_cloud_billing_events.
func (m *Machine) Transition(ctx context.Context, instanceID string, next State, meta map[string]interface{}) error {
	current, err := m.db.CurrentState(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("saga: read current state: %w", err)
	}

	if err := ValidateTransition(current, next); err != nil {
		return err
	}

	if current == next {
		return nil // idempotent no-op
	}

	return m.db.TransitionState(ctx, instanceID, current, next, meta)
}

// Fail marks instanceID as failed, recording reason in metadata.
// Safe to call from any non-terminal state; no-ops if already terminal.
func (m *Machine) Fail(ctx context.Context, instanceID, reason string) error {
	current, err := m.db.CurrentState(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("saga: read current state for fail: %w", err)
	}
	if terminalStates[current] {
		return nil // already terminal — no-op
	}
	meta := map[string]interface{}{
		"fail_reason": reason,
		"failed_at":   time.Now().UTC().Format(time.RFC3339),
	}
	return m.db.TransitionState(ctx, instanceID, current, StateFailed, meta)
}

// IsTerminal returns true if state is a terminal (ready or failed).
func IsTerminal(s State) bool {
	return terminalStates[s]
}

// SQLDB implements DB using *sql.DB.
type SQLDB struct {
	db *sql.DB
}

// NewSQLDB wraps a *sql.DB for use with the saga Machine.
func NewSQLDB(db *sql.DB) *SQLDB {
	return &SQLDB{db: db}
}

// CurrentState reads the current saga state from np_cloud_instances.
func (s *SQLDB) CurrentState(ctx context.Context, instanceID string) (State, error) {
	var dbStatus string
	err := s.db.QueryRowContext(ctx,
		`SELECT status FROM np_cloud_instances WHERE id = $1`,
		instanceID,
	).Scan(&dbStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("saga: instance %s not found", instanceID)
	}
	if err != nil {
		return "", fmt.Errorf("saga: query current state: %w", err)
	}
	return dbStatusToSaga(dbStatus), nil
}

// TransitionState atomically updates np_cloud_instances.status and appends an
// event to np_cloud_billing_events within a single transaction.
func (s *SQLDB) TransitionState(ctx context.Context, instanceID string, from, to State, meta map[string]interface{}) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("saga: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	dbTo := sagaToDBStatus(to)
	dbFrom := sagaToDBStatus(from)

	// Optimistic lock: only update if current db status matches the expected `from`.
	res, err := tx.ExecContext(ctx,
		`UPDATE np_cloud_instances
		    SET status = $1, updated_at = now()
		  WHERE id = $2 AND status = $3`,
		dbTo, instanceID, dbFrom,
	)
	if err != nil {
		return fmt.Errorf("saga: update status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("saga: rows affected: %w", err)
	}
	if n == 0 {
		// Concurrent update won; verify target was already reached (idempotent).
		var cur string
		if scanErr := s.db.QueryRowContext(ctx,
			`SELECT status FROM np_cloud_instances WHERE id = $1`, instanceID,
		).Scan(&cur); scanErr != nil {
			return fmt.Errorf("saga: concurrency check: %w", scanErr)
		}
		if dbStatusToSaga(cur) != to {
			return fmt.Errorf("saga: concurrent state change (expected from=%s got=%s)", dbFrom, cur)
		}
		return tx.Commit() // already at target — idempotent
	}

	// Log the transition as a billing event.
	if evtType := stateToEventType(to); evtType != "" {
		metaJSON, _ := json.Marshal(meta)
		_, err = tx.ExecContext(ctx,
			`INSERT INTO np_cloud_billing_events
			    (tenant_id, instance_id, event_type, provider, metadata, status)
			 SELECT tenant_id, $1, $2, 'stripe', $3::jsonb, 'processed'
			   FROM np_cloud_instances WHERE id = $1`,
			instanceID, evtType, string(metaJSON),
		)
		if err != nil {
			return fmt.Errorf("saga: log billing event: %w", err)
		}
	}

	return tx.Commit()
}

// LogBillingEvent appends a row to np_cloud_billing_events.
func (s *SQLDB) LogBillingEvent(ctx context.Context, evt BillingEvent) error {
	currency := evt.Currency
	if currency == "" {
		currency = "usd"
	}
	metaJSON, _ := json.Marshal(evt.Metadata)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO np_cloud_billing_events
		    (tenant_id, instance_id, event_type, provider, provider_event_id,
		     amount_cents, currency, idempotency_key, metadata, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,'processed')
		 ON CONFLICT (idempotency_key) DO NOTHING`,
		evt.TenantID, nilIfEmpty(evt.InstanceID), evt.EventType, evt.Provider,
		nilIfEmpty(evt.ProviderEventID), evt.AmountCents, currency,
		nilIfEmpty(evt.IdempotencyKey), string(metaJSON),
	)
	if err != nil {
		return fmt.Errorf("saga: log billing event: %w", err)
	}
	return nil
}

// --- mapping helpers ---

// sagaToDBStatus maps saga State to np_cloud_instances.status CHECK values.
func sagaToDBStatus(s State) string {
	switch s {
	case StatePending, StateCharging, StateCharged:
		return "pending"
	case StateProvisioning:
		return "provisioning"
	case StateReady:
		return "running"
	case StateFailed:
		return "provision_failed"
	default:
		return string(s)
	}
}

// dbStatusToSaga maps np_cloud_instances.status back to a saga State.
func dbStatusToSaga(dbStatus string) State {
	switch dbStatus {
	case "pending":
		return StatePending
	case "provisioning", "bootstrapping":
		return StateProvisioning
	case "running":
		return StateReady
	case "provision_failed":
		return StateFailed
	default:
		return State(dbStatus)
	}
}

// stateToEventType returns the billing event_type for a state transition, or "" to skip logging.
func stateToEventType(to State) string {
	switch to {
	case StateCharged:
		return "subscription_created"
	case StateReady:
		return "invoice_paid"
	case StateFailed:
		return "invoice_failed"
	default:
		return ""
	}
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
