package world

import (
	"context"
	"reflect"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func hasReason(rs []trust.Reason, want trust.Reason) bool {
	for _, r := range rs {
		if r == want {
			return true
		}
	}
	return false
}

// TestBuildFillsRunningFromActiveAttempts is the TM-11 regression (finding A05): the five fixture attempts
// are leased to "local". With the jobs known, the local runner reports what actually runs there, and the
// co-tenancy rule rejects an untrusted job beside a secret-bearing attempt.
func TestBuildFillsRunningFromActiveAttempts(t *testing.T) {
	d, p := worldFixture(t)
	jobs := func(secretOn string, trustClass model.TrustClass) []sched.Job {
		var out []sched.Job
		for _, id := range []string{"joba", "jobb", "jobc", "jobd", "jobe"} {
			j := sched.Job{ID: id, Key: id, Trust: trustClass, SecretClasses: []model.SecretClass{"none"}}
			if id == secretOn {
				j.SecretClasses = []model.SecretClass{"project"}
			}
			out = append(out, j)
		}
		return out
	}
	local := func(js []sched.Job) sched.Runner {
		d.Jobs = js
		w, _, err := Build(context.Background(), d, p)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range w.Runners {
			if r.ID == "local" {
				return r
			}
		}
		t.Fatal("no local runner")
		return sched.Runner{}
	}
	untrustedJob := trust.JobFacts{Trust: "untrusted", Network: "none"}
	secretJob := trust.JobFacts{Trust: "owner", SecretClasses: []model.SecretClass{"project"}, Network: "none"}
	plainJob := trust.JobFacts{Trust: "owner", Network: "none"}

	secret := local(jobs("jobc", "owner"))
	if got := secret.Decl.Running; got != (trust.Running{SecretBearing: true}) {
		t.Fatalf("Running = %+v, want secret-bearing only", got)
	}
	if !hasReason(trust.Admissible(untrustedJob, secret.Decl, trust.Effective{}), trust.IsolationCotenancy) {
		t.Fatal("an untrusted job beside a secret-bearing attempt must fail isolation.cotenancy")
	}
	if hasReason(trust.Admissible(plainJob, secret.Decl, trust.Effective{}), trust.IsolationCotenancy) {
		t.Fatal("a trusted job without secrets must not trip the co-tenancy rule")
	}

	untrusted := local(jobs("", "untrusted"))
	if got := untrusted.Decl.Running; got != (trust.Running{Untrusted: true}) {
		t.Fatalf("Running = %+v, want untrusted only", got)
	}
	if !hasReason(trust.Admissible(secretJob, untrusted.Decl, trust.Effective{}), trust.IsolationCotenancy) {
		t.Fatal("a secret-bearing job beside an untrusted attempt must fail isolation.cotenancy")
	}

	quiet := local(jobs("", "owner"))
	if got := quiet.Decl.Running; got != (trust.Running{}) {
		t.Fatalf("Running = %+v, want empty", got)
	}
	// An unrecognised trust class decodes to untrusted (most restrictive).
	if got := local(jobs("", "mystery")).Decl.Running; !got.Untrusted {
		t.Fatalf("unknown trust class must read as untrusted: %+v", got)
	}
}

// TestFillRunningUnknownAttemptIsRestrictive: an active attempt whose job the caller is not placing has no
// recorded trust or secret classes, so it counts as untrusted and secret-bearing; other nodes stay empty.
func TestFillRunningUnknownAttemptIsRestrictive(t *testing.T) {
	runners := []sched.Runner{{ID: "n1"}, {ID: "n2"}}
	active := []store.SchedAttempt{{JobID: "stranger", RunnerID: "n1"}, {JobID: "unplaced", RunnerID: ""}}
	fillRunning(runners, active, nil)
	if runners[0].Decl.Running != (trust.Running{Untrusted: true, SecretBearing: true}) || runners[1].Decl.Running != (trust.Running{}) {
		t.Fatalf("running = %+v / %+v", runners[0].Decl.Running, runners[1].Decl.Running)
	}
}

// TestPreviousIsolation is the TM-08 input (finding A04): the floor is the previous runner's isolation.
func TestPreviousIsolation(t *testing.T) {
	r := func(id string, iso model.Isolation) sched.Runner {
		return sched.Runner{ID: id, Decl: trust.Declaration{Isolation: iso}}
	}
	jobs := []sched.Job{{ID: "j1", Key: "k1"}, {ID: "j2", Key: "k2"}, {ID: "j3", Key: "k3"}, {ID: "j4", Key: "k4"}}
	prev := map[string]string{"j1": "vm1", "j2": "hosted1", "j3": "gone"}
	got := previousIsolation(jobs, prev, []sched.Runner{r("vm1", "vm"), r("blank", "")}, []sched.Runner{r("hosted1", "hosted-disposable")})
	want := map[string]model.Isolation{"k1": "vm", "k2": "hosted-disposable"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("floor = %v, want %v", got, want)
	}
	if previousIsolation(jobs, nil, nil) != nil {
		t.Fatal("no previous attempts must give no floor")
	}
}
