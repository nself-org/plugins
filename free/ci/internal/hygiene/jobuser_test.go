package hygiene

import (
	"fmt"
	"regexp"
	"testing"
)

// bootstrapJobUser is the account shape `nodes bootstrap --system` creates and job-shim accepts:
// nself-ci-job-1..N, no zero, no leading zero (D-1231).
var bootstrapJobUser = regexp.MustCompile(`^nself-ci-job-([1-9][0-9]{0,2})$`)

// TestJobUserNameMatchesBootstrap: slot s (0-based) is the user bootstrap creates as number s+1, so
// every slot below Slots resolves to an existing account and slot 0 never maps to nself-ci-job-0.
func TestJobUserNameMatchesBootstrap(t *testing.T) {
	const slots = 64
	for slot := 0; slot < slots; slot++ {
		name := JobUserName(slot)
		m := bootstrapJobUser.FindStringSubmatch(name)
		if m == nil {
			t.Fatalf("slot %d: %q is not a bootstrap-created user", slot, name)
		}
		if want := fmt.Sprint(slot + 1); m[1] != want {
			t.Fatalf("slot %d: %q, want nself-ci-job-%s", slot, name, want)
		}
	}
}

// TestSeparationUserIsBootstrapUser: the plan's User is a created account for every slot.
func TestSeparationUserIsBootstrapUser(t *testing.T) {
	f := baseFacts()
	f.JobShim, f.JobUIDs = true, []int{2000, 2001, 2002, 2003}
	for slot := 0; slot < f.Slots; slot++ {
		p, err := Separation(f, slot)
		if err != nil || p.User != fmt.Sprintf("nself-ci-job-%d", slot+1) {
			t.Errorf("slot %d: %+v %v", slot, p, err)
		}
	}
}
