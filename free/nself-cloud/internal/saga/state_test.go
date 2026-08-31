package saga

import (
	"context"
	"errors"
	"testing"
)

// --- ValidateTransition unit tests ---

func TestValidateTransition_AllowedPaths(t *testing.T) {
	cases := []struct{ from, to State }{
		{StatePending, StateCharging},
		{StateCharging, StateCharged},
		{StateCharged, StateProvisioning},
		{StateProvisioning, StateReady},
		{StatePending, StateFailed},
		{StateCharging, StateFailed},
		{StateCharged, StateFailed},
		{StateProvisioning, StateFailed},
	}
	for _, c := range cases {
		if err := ValidateTransition(c.from, c.to); err != nil {
			t.Errorf("expected %s→%s to be allowed, got: %v", c.from, c.to, err)
		}
	}
}

func TestValidateTransition_InvalidPaths(t *testing.T) {
	cases := []struct{ from, to State }{
		{StatePending, StateCharged},       // skip charging
		{StatePending, StateProvisioning},  // skip multiple steps
		{StateCharging, StateProvisioning}, // skip charged
		{StateCharged, StateReady},         // skip provisioning
		{StateProvisioning, StateCharging}, // backward
	}
	for _, c := range cases {
		if err := ValidateTransition(c.from, c.to); err == nil {
			t.Errorf("expected %s→%s to be disallowed, but got nil error", c.from, c.to)
		}
	}
}

func TestValidateTransition_TerminalStates(t *testing.T) {
	for _, terminal := range []State{StateReady, StateFailed} {
		err := ValidateTransition(terminal, StatePending)
		if err == nil {
			t.Errorf("expected transition from terminal state %s to be blocked", terminal)
		}
		var e ErrAlreadyTerminal
		if !errors.As(err, &e) {
			t.Errorf("expected ErrAlreadyTerminal, got %T", err)
		}
	}
}

func TestValidateTransition_Idempotent(t *testing.T) {
	for _, s := range []State{StatePending, StateCharging, StateCharged, StateProvisioning, StateReady, StateFailed} {
		if err := ValidateTransition(s, s); err != nil {
			t.Errorf("idempotent %s→%s should return nil, got: %v", s, s, err)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	if !IsTerminal(StateReady) {
		t.Error("StateReady must be terminal")
	}
	if !IsTerminal(StateFailed) {
		t.Error("StateFailed must be terminal")
	}
	for _, s := range []State{StatePending, StateCharging, StateCharged, StateProvisioning} {
		if IsTerminal(s) {
			t.Errorf("%s must NOT be terminal", s)
		}
	}
}

func TestErrInvalidTransition_Error(t *testing.T) {
	err := ErrInvalidTransition{From: StatePending, To: StateReady}
	if err.Error() == "" {
		t.Error("ErrInvalidTransition.Error() must not be empty")
	}
}

func TestErrAlreadyTerminal_Error(t *testing.T) {
	err := ErrAlreadyTerminal{Current: StateReady}
	if err.Error() == "" {
		t.Error("ErrAlreadyTerminal.Error() must not be empty")
	}
}

// --- Machine with mock DB ---

type testDB struct {
	state    State
	events   []BillingEvent
	transErr error
}

func (d *testDB) CurrentState(_ context.Context, _ string) (State, error) {
	return d.state, nil
}

func (d *testDB) TransitionState(_ context.Context, _ string, _, to State, _ map[string]interface{}) error {
	if d.transErr != nil {
		return d.transErr
	}
	d.state = to
	return nil
}

func (d *testDB) LogBillingEvent(_ context.Context, evt BillingEvent) error {
	d.events = append(d.events, evt)
	return nil
}

func TestMachine_Transition_HappyPath(t *testing.T) {
	db := &testDB{state: StatePending}
	m := New(db)
	ctx := context.Background()

	steps := []State{StateCharging, StateCharged, StateProvisioning, StateReady}
	for _, next := range steps {
		if err := m.Transition(ctx, "inst-1", next, nil); err != nil {
			t.Fatalf("transition to %s failed: %v", next, err)
		}
		if db.state != next {
			t.Fatalf("expected state %s, got %s", next, db.state)
		}
	}
}

func TestMachine_Transition_Idempotent(t *testing.T) {
	db := &testDB{state: StateCharged}
	m := New(db)
	ctx := context.Background()

	// Transitioning to current state should be a no-op.
	if err := m.Transition(ctx, "inst-1", StateCharged, nil); err != nil {
		t.Fatalf("idempotent transition should succeed: %v", err)
	}
	if db.state != StateCharged {
		t.Fatalf("state should remain charged, got %s", db.state)
	}
}

func TestMachine_Transition_InvalidSkip(t *testing.T) {
	db := &testDB{state: StatePending}
	m := New(db)
	ctx := context.Background()

	err := m.Transition(ctx, "inst-1", StateReady, nil)
	if err == nil {
		t.Fatal("expected error skipping multiple states")
	}
}

func TestMachine_Fail_FromAnyState(t *testing.T) {
	for _, from := range []State{StatePending, StateCharging, StateCharged, StateProvisioning} {
		db := &testDB{state: from}
		m := New(db)
		ctx := context.Background()

		if err := m.Fail(ctx, "inst-1", "test reason"); err != nil {
			t.Errorf("Fail from %s should succeed: %v", from, err)
		}
		if db.state != StateFailed {
			t.Errorf("state should be failed after Fail(), got %s", db.state)
		}
	}
}

func TestMachine_Fail_AlreadyTerminal(t *testing.T) {
	for _, terminal := range []State{StateReady, StateFailed} {
		db := &testDB{state: terminal}
		m := New(db)
		ctx := context.Background()

		// Fail on a terminal state must be a no-op.
		if err := m.Fail(ctx, "inst-1", "reason"); err != nil {
			t.Errorf("Fail on terminal state %s should be no-op, got: %v", terminal, err)
		}
		if db.state != terminal {
			t.Errorf("state should remain %s after no-op Fail, got %s", terminal, db.state)
		}
	}
}

// --- sagaToDBStatus / dbStatusToSaga round-trip ---

func TestStatusRoundTrip(t *testing.T) {
	cases := []struct {
		saga State
		db   string
	}{
		{StatePending, "pending"},
		{StateCharging, "pending"},
		{StateCharged, "pending"},
		{StateProvisioning, "provisioning"},
		{StateReady, "running"},
		{StateFailed, "provision_failed"},
	}
	for _, c := range cases {
		got := sagaToDBStatus(c.saga)
		if got != c.db {
			t.Errorf("sagaToDBStatus(%s) = %q, want %q", c.saga, got, c.db)
		}
	}
	// Reverse mapping for unambiguous cases.
	reverseMap := map[string]State{
		"provisioning":     StateProvisioning,
		"running":          StateReady,
		"provision_failed": StateFailed,
	}
	for db, want := range reverseMap {
		got := dbStatusToSaga(db)
		if got != want {
			t.Errorf("dbStatusToSaga(%q) = %s, want %s", db, got, want)
		}
	}
}
