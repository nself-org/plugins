package demand_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/demand"
)

// TestMaxPendingKey: the key is registered operator-only (Spend, fixed), accepts a whole number from 1 to
// 50 and nothing else, and the default is 2.
func TestMaxPendingKey(t *testing.T) {
	if errs := demand.RegistrationErrors(); len(errs) != 0 {
		t.Fatalf("registration: %v", errs)
	}
	k := demand.Key()
	if k.Key != "providers.github.jit.max_pending" || !k.Spend || k.Kind != policy.TightenFixed || k.Owner != "P7-HOST-19" {
		t.Fatalf("key spec %+v", k)
	}
	for _, ok := range []string{"1", "2", "50"} {
		if v, err := k.Decode(json.RawMessage(ok)); err != nil || v == nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"0", "-1", "51", "2.5", `"2"`, "null", "true", "[2]", "2 3", "1e2"} {
		if _, err := k.Decode(json.RawMessage(bad)); err == nil || !strings.Contains(err.Error(), "E672") {
			t.Errorf("%s: want E672, got %v", bad, err)
		}
	}
	if d := demand.DefaultSettings(); d.MaxPending != 2 || d.Label != "nself" {
		t.Fatalf("defaults %+v", d)
	}
	demand.UsePolicy(nil)
	if s, err := demand.CurrentSettings(context.Background()); err != nil || s != demand.DefaultSettings() {
		t.Fatalf("no policy source: %+v %v", s, err)
	}
	if _, err := demand.SettingsFrom(nil); err == nil {
		t.Fatal("a nil effective policy must not silently mean the defaults")
	}
}
