package saga

import (
	"context"
	"testing"
)

// TestProvisionOpts_NonNilFields validates that ProvisionOpts has expected fields.
// This is a structural smoke test — actual E2E tests require a real DB + Hetzner + Stripe.
func TestProvisionOpts_Fields(t *testing.T) {
	opts := ProvisionOpts{
		TenantID:         "tenant-1",
		InstanceID:       "inst-1",
		StripeCustomerID: "cus_test",
		AmountCents:      499,
		Currency:         "usd",
		ServerName:       "test-server",
		ServerType:       "cx23",
		Region:           "fsn1",
		SSHKeyID:         0,
		Labels:           map[string]string{"managed_by": "nself-cloud"},
	}
	if opts.TenantID == "" {
		t.Error("TenantID must be set")
	}
	if opts.InstanceID == "" {
		t.Error("InstanceID must be set")
	}
	if opts.AmountCents <= 0 {
		t.Error("AmountCents must be positive")
	}
}

// TestProvisionInstance_AlreadyReady ensures an already-ready instance is a no-op.
func TestProvisionInstance_AlreadyReady(t *testing.T) {
	db := &testDB{state: StateReady}
	m := New(db)
	_ = m // provisioner uses machine internally; test the state machine directly

	// Already ready: Transition to StateReady should be a no-op.
	ctx := context.Background()
	if err := m.Transition(ctx, "inst-ready", StateReady, nil); err != nil {
		t.Fatalf("transitioning already-ready instance should be no-op: %v", err)
	}
}

// TestProvisionInstance_FailedInstance ensures a failed instance cannot be re-provisioned.
func TestProvisionInstance_FailedInstance(t *testing.T) {
	db := &testDB{state: StateFailed}
	m := New(db)

	ctx := context.Background()
	// Any transition from StateFailed (except idempotent self) should fail.
	err := m.Transition(ctx, "inst-failed", StateCharging, nil)
	if err == nil {
		t.Fatal("expected error transitioning a failed instance to charging")
	}
}

// TestCompensatingFail_StripeThenHetznerFail simulates the CHAOS-CRITICAL scenario:
// charge succeeds but Hetzner fails → state must end in failed.
func TestCompensatingFail_StripeThenHetznerFail(t *testing.T) {
	db := &testDB{state: StatePending}
	m := New(db)
	ctx := context.Background()

	// Simulate: pending → charging (ok)
	if err := m.Transition(ctx, "inst-comp", StateCharging, nil); err != nil {
		t.Fatalf("→ charging: %v", err)
	}
	// Simulate: charging → charged (ok)
	if err := m.Transition(ctx, "inst-comp", StateCharged, map[string]interface{}{
		"stripe_charge_id": "ch_test_123",
	}); err != nil {
		t.Fatalf("→ charged: %v", err)
	}
	// Simulate: Hetzner failed → saga fails
	if err := m.Fail(ctx, "inst-comp", "hetzner: quota exceeded"); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if db.state != StateFailed {
		t.Fatalf("expected StateFailed after Hetzner failure, got %s", db.state)
	}
}
