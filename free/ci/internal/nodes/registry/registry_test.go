package registry

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
	if _, err = r.SetTrust(ctx, id, trust, "admin"); err != nil {
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
