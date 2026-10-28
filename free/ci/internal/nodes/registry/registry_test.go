package registry

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func fixture(t *testing.T) model.Capability {
	t.Helper()
	b, err := os.ReadFile("../../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c
}
func setup(t *testing.T) (*Registry, model.Capability) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	r := New(s)
	c := fixture(t)
	if _, err = r.Register(context.Background(), c, "operator"); err != nil {
		t.Fatal(err)
	}
	return r, c
}
func TestLifecycleSeparate(t *testing.T) {
	r, c := setup(t)
	ctx := context.Background()
	n, err := r.Get(ctx, c.Identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !n.Capability.Lifecycle.Discovered || n.Capability.Lifecycle.Eligible || len(n.Capability.Lifecycle.AuthorizedProjects) != 0 || n.Capability.Lifecycle.RevokedAt != nil {
		t.Fatal("discovery authorized or enabled node")
	}
	n, err = r.Authorize(ctx, c.Identity.ID, "project-1", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Capability.Lifecycle.AuthorizedProjects) != 1 || n.Capability.Lifecycle.Eligible {
		t.Fatal("authorization changed eligibility")
	}
	n, err = r.Deauthorize(ctx, c.Identity.ID, "project-1", "admin")
	if err != nil || len(n.Capability.Lifecycle.AuthorizedProjects) != 0 {
		t.Fatalf("deauthorize: %v %+v", err, n)
	}
}

func TestTrustAndDrift(t *testing.T) {
	r, c := setup(t)
	ctx := context.Background()
	id := c.Identity.ID
	n, err := r.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	strong := model.Isolation("vm")
	trust := n.Capability.Trust
	trust.Isolation = model.Fact[model.Isolation]{Value: &strong, Source: "assigned", ObservedAt: time.Now().UTC(), Confidence: "known"}
	admin, err := store.NewAdminAuthority("admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.SetTrust(ctx, id, trust, admin); err != nil {
		t.Fatal(err)
	}
	in := c
	in.Trust.Accepts.Value = &[]model.TrustClass{"untrusted"}
	in.Trust.Isolation.Value = &strong
	weak := model.Isolation("process")
	in.Trust.Isolation.Value = &weak
	in.Tools.Docker.Value = new(bool)
	n, diff, err := r.UpdateCapability(ctx, id, in, "probe")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(diff, []string{"/tools"}) {
		t.Fatalf("diff: %v", diff)
	}
	if len(*n.Capability.Trust.Accepts.Value) != 0 || *n.Capability.Trust.Isolation.Value != "process" {
		t.Fatal("trust payload was accepted or downgrade lost")
	}
	in.Trust.Isolation.Value = &strong
	n, _, err = r.UpdateCapability(ctx, id, in, "probe")
	if err != nil {
		t.Fatal(err)
	}
	if *n.Capability.Trust.Isolation.Value != "process" {
		t.Fatal("probe raised isolation")
	}
	baseTrust := n.Capability.Trust
	baseOwnership := n.Capability.Identity.Ownership
	for _, tc := range []struct {
		name   string
		change func(*model.Capability)
	}{
		{"accepts", func(c *model.Capability) { v := []model.TrustClass{"untrusted"}; c.Trust.Accepts.Value = &v }},
		{"secret_classes", func(c *model.Capability) { v := []model.SecretClass{"release"}; c.Trust.SecretClasses.Value = &v }},
		{"privacy_zone", func(c *model.Capability) { v := model.PrivacyZone("hosted-allowed"); c.Trust.PrivacyZone.Value = &v }},
		{"ownership", func(c *model.Capability) { v := "team"; c.Identity.Ownership.Value = &v }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := c
			tc.change(&payload)
			got, _, e := r.UpdateCapability(ctx, id, payload, "agent")
			if e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(got.Capability.Trust, baseTrust) || !reflect.DeepEqual(got.Capability.Identity.Ownership, baseOwnership) {
				t.Fatal("node payload changed coordinator-assigned fields")
			}
		})
	}
}

// TestAgeRecipientPin proves the first hello pin, mismatch E660, unchanged pin and maintenance flag.
func TestAgeRecipientPin(t *testing.T) {
	r, c := setup(t)
	ctx := context.Background()
	id := c.Identity.ID
	if v, err := r.AgeRecipient(ctx, id); err != nil || v != nil {
		t.Fatalf("initial recipient: %v %v", v, err)
	}
	if err := r.CheckAgeRecipient(ctx, id, "age1first", "agent"); err != nil {
		t.Fatal(err)
	}
	if err := r.CheckAgeRecipient(ctx, id, "age1first", "agent"); err != nil {
		t.Fatal(err)
	}
	err := r.CheckAgeRecipient(ctx, id, "age1changed", "agent")
	var coded *Error
	if !errors.As(err, &coded) || coded.Code != "E660" || coded.Reason != "age_recipient_changed" {
		t.Fatalf("mismatch: %v", err)
	}
	n, err := r.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if n.Record.State != "maintenance" || n.Record.Reason != "age_recipient_changed" {
		t.Fatal("node not flagged")
	}
	if v, _ := r.AgeRecipient(ctx, id); v == nil || *v != "age1first" {
		t.Fatal("pin replaced")
	}
}

// D-1212: a quarantined node cannot be enabled, drained or put in another maintenance reason through
// SetState, even when the caller read the node before the quarantine was set.
func TestSetStateRefusesToClearTheQuarantine(t *testing.T) {
	r, c := setup(t)
	ctx := context.Background()
	id := c.Identity.ID
	if _, err := r.SetState(ctx, id, "online", "enabled by admin", "operator"); err != nil {
		t.Fatal(err)
	}
	if err := r.CheckAgeRecipient(ctx, id, "age1first", "agent"); err != nil {
		t.Fatal(err)
	}
	if err := r.CheckAgeRecipient(ctx, id, "age1changed", "agent"); err == nil {
		t.Fatal("the changed recipient was accepted")
	}
	for _, st := range []struct{ state, reason string }{
		{"online", "enabled by admin"}, {"draining", "drained by admin"}, {"maintenance", "three consecutive infra failures"},
	} {
		_, err := r.SetState(ctx, id, st.state, st.reason, "operator")
		var coded *Error
		if !errors.As(err, &coded) || coded.Code != "E651" || !strings.Contains(coded.Reason, "quarantined") {
			t.Fatalf("%s: err = %v, want E651 quarantined", st.state, err)
		}
	}
	n, err := r.Get(ctx, id)
	if err != nil || n.Record.State != "maintenance" || n.Record.Reason != QuarantineReason {
		t.Fatalf("quarantine changed: %+v %v", n.Record, err)
	}
	// the same reason (the flag itself, an upgrade's undo) and a revoke stay possible
	if _, err = r.SetState(ctx, id, "maintenance", QuarantineReason, "operator"); err != nil {
		t.Fatalf("re-flag: %v", err)
	}
	if n, err = r.SetState(ctx, id, "revoked", "revoked", "operator"); err != nil || n.Record.State != "revoked" {
		t.Fatalf("revoke: %v %+v", err, n.Record)
	}
}

// TestSecretsEligible proves a non-separated node cannot gain eligibility from a hello.
func TestSecretsEligible(t *testing.T) {
	r, c := setup(t)
	ctx := context.Background()
	id := c.Identity.ID
	no := false
	c.Separation.UIDSeparation.Value = &no
	c.SecretsEligible = true
	n, _, err := r.UpdateCapability(ctx, id, c, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if n.Capability.SecretsEligible {
		t.Fatal("non-separated node eligible")
	}
	yes := true
	c.Separation.UIDSeparation.Value = &yes
	n, _, err = r.UpdateCapability(ctx, id, c, "system-install")
	if err != nil {
		t.Fatal(err)
	}
	if !n.Capability.SecretsEligible {
		t.Fatal("separated node ineligible")
	}
}

func TestConcurrentCapabilityUpdates(t *testing.T) {
	r, c := setup(t)
	ctx := context.Background()
	const total = 100
	var wg sync.WaitGroup
	errs := make(chan error, total)
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < total/4; i++ {
				_, _, err := r.UpdateCapability(ctx, c.Identity.ID, c, "probe")
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	n, err := r.Get(ctx, c.Identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n.Record.Version != total+1 {
		t.Fatalf("lost update: version %d", n.Record.Version)
	}
}

func TestInvalidCapability(t *testing.T) {
	r, c := setup(t)
	c.Identity.Provider = "unknown"
	_, _, err := r.UpdateCapability(context.Background(), c.Identity.ID, c, "probe")
	if err == nil || !strings.Contains(err.Error(), "/properties/identity/properties/provider") {
		t.Fatalf("expected pointer error: %v", err)
	}
}

func TestRegistrationClearsOwnership(t *testing.T) {
	r, c := setup(t)
	n, err := r.Get(context.Background(), c.Identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n.Capability.Identity.Ownership.Value != nil || n.Capability.Identity.Ownership.Source != "assigned" {
		t.Fatalf("caller ownership persisted: %+v", n.Capability.Identity.Ownership)
	}
}

func TestRevokedNodeRequiresAdminRecovery(t *testing.T) {
	r, c := setup(t)
	ctx := context.Background()
	id := c.Identity.ID
	if _, err := r.SetState(ctx, id, "revoked", "operator decision", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SetState(ctx, id, "online", "hello", "agent"); err == nil {
		t.Fatal("agent resurrected node")
	}
	n, err := r.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if n.Record.State != "revoked" || n.Capability.Lifecycle.RevokedAt == nil {
		t.Fatal("revocation lost")
	}
	admin, err := store.NewAdminAuthority("admin")
	if err != nil {
		t.Fatal(err)
	}
	n, err = r.Recover(ctx, id, "offline", admin)
	if err != nil {
		t.Fatal(err)
	}
	if n.Record.State != "offline" || n.Capability.Lifecycle.RevokedAt != nil {
		t.Fatal("recovery did not clear revocation")
	}
}

func TestTrustRequiresAdminAuthority(t *testing.T) {
	r, c := setup(t)
	n, err := r.Get(context.Background(), c.Identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.NewAdminAuthority("agent"); err == nil {
		t.Fatal("agent acquired admin authority")
	}
	if _, err := r.SetTrust(context.Background(), c.Identity.ID, n.Capability.Trust, store.AdminAuthority{}); err == nil {
		t.Fatal("zero authority assigned trust")
	}
	if err := r.Store.NodeSetTrust(context.Background(), n.Record, store.AdminAuthority{}); err == nil {
		t.Fatal("store accepted zero authority")
	}
}

// TestNodeReportCannotSetHostedOrProviderLabels: a node report may not make itself hosted (which skips the
// AuthorizedProjects check) or claim an nself.* label; the burst policy actor still can (D-1402).
func TestNodeReportCannotSetHostedOrProviderLabels(t *testing.T) {
	r, c := setup(t)
	ctx := context.Background()
	id := c.Identity.ID
	hosted := "hosted"
	report := c
	report.Location.Kind = model.Fact[string]{Value: &hosted, Source: "self", ObservedAt: time.Now().UTC(), Confidence: "known"}
	report.Identity.Labels = append(append([]string(nil), c.Identity.Labels...), "nself.repo=acme/app", "nself.provider=github", "gpu=none")
	for _, actor := range []string{"agent", "probe", "system-install"} {
		n, _, err := r.UpdateCapability(ctx, id, report, actor)
		if err != nil {
			t.Fatalf("%s: %v", actor, err)
		}
		if k := n.Capability.Location.Kind.Value; k != nil && *k == "hosted" {
			t.Fatalf("%s: a node report made the node hosted", actor)
		}
		for _, l := range n.Capability.Identity.Labels {
			if strings.HasPrefix(l, "nself.") {
				t.Fatalf("%s: a node report added label %q", actor, l)
			}
		}
		if !slices.Contains(n.Capability.Identity.Labels, "gpu=none") {
			t.Fatalf("%s: an ordinary label was dropped: %v", actor, n.Capability.Identity.Labels)
		}
	}
	// The coordinator-side policy actor sets both, and a later node report can neither clear nor replace them.
	n, _, err := r.UpdateCapability(ctx, id, report, PolicyActorPrefix+"burst")
	if err != nil {
		t.Fatal(err)
	}
	if k := n.Capability.Location.Kind.Value; k == nil || *k != "hosted" || !slices.Contains(n.Capability.Identity.Labels, "nself.repo=acme/app") {
		t.Fatalf("policy actor could not set hosted and labels: %v %v", n.Capability.Location.Kind.Value, n.Capability.Identity.Labels)
	}
	back := c
	back.Identity.Labels = []string{"nself.repo=evil/other"}
	n, _, err = r.UpdateCapability(ctx, id, back, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if k := n.Capability.Location.Kind.Value; k == nil || *k != "hosted" {
		t.Fatal("a node report cleared the hosted location")
	}
	if !slices.Contains(n.Capability.Identity.Labels, "nself.repo=acme/app") || slices.Contains(n.Capability.Identity.Labels, "nself.repo=evil/other") {
		t.Fatalf("a node report replaced a provider label: %v", n.Capability.Identity.Labels)
	}
}
