//go:build security

package security

// Purpose: scenarios 7 and 8 of the CI fabric directive, run through the real dispatcher
// (store, admission, scheduler, trust rules) with the hosted provider fake behind the transport.
//   Scenario 7: a public fork PR is untrusted, gets no secret, lands only on a hosted-disposable
//   runner or a gVisor runner with no LAN route, and never on a LAN node.
//   Scenario 8: private-infrastructure work never reaches a hosted provider, whatever the
//   pressure on the LAN.
// Inputs:  the fixtures of fixtures_test.go.
// Outputs: pass or fail; the matrix maps the test names to the threat rows.
// Constraints: no test here skips; a job that must wait is submitted with a short budget and
//          the decision sink says why it stayed queued.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

const settle = 400 * time.Millisecond

func runnerOf(t *testing.T, a dispatch.Admitted, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("job was not admitted: %v", err)
	}
	return a.RunnerID
}

// awaitDecision waits (bounded) for the sink to hold a decision for the job: a loaded host may
// run the tick after the caller's budget ended.
func awaitDecision(t *testing.T, r *rig, id string) dispatch.Decision {
	t.Helper()
	for tries := 0; tries < 500; tries++ {
		if d, ok := r.sink.last(id); ok {
			return d
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no decision recorded for %s", id)
	return dispatch.Decision{}
}

// waitsBecause asserts the job stayed queued and that the sink recorded a reason code in want.
func waitsBecause(t *testing.T, r *rig, id string, a dispatch.Admitted, err error, want ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("job %s was admitted on %s, it must wait", id, a.RunnerID)
	}
	d := awaitDecision(t, r, id)
	if d.Placement != nil {
		t.Fatalf("job %s was placed on %s although it must wait", id, d.Placement.RunnerID)
	}
	var codes []string
	for _, x := range d.Explanation.Reasons {
		codes = append(codes, x.Code)
	}
	for _, alt := range d.Explanation.Alternatives {
		for _, x := range alt.Reasons {
			codes = append(codes, x.Code)
		}
	}
	all := strings.Join(codes, ",")
	for _, w := range want {
		if !strings.Contains(all, w) {
			t.Fatalf("job %s waits but the reasons %q do not name %q", id, all, w)
		}
	}
}

// TestScenario7_PublicForkPR drives a public fork PR (source class untrusted) through the
// dispatcher over a mixed pool.
func TestScenario7_PublicForkPR(t *testing.T) {
	eff := repoPolicy(t, "public", "untrusted")
	r := newRig(t, eff, lanRunner("lan1", 4), gvisorRunner("gv1", 2), hostedRunner("hosted1"))

	// 1. Four untrusted jobs: every one lands on gVisor or the hosted-disposable fake, none on the LAN node.
	onGV, onHosted := 0, 0
	for i := 0; i < 4; i++ {
		id, a, err := r.submit(newJob("fork"+string(rune('a'+i)), "untrusted", "restricted"), "untrusted", 2*time.Second)
		if err != nil {
			d, _ := r.sink.last(id)
			t.Fatalf("fork job %d not admitted: %v; outcome %q reason %q explanation %+v", i, err, d.Outcome, d.Reason, d.Explanation)
		}
		switch got := runnerOf(t, a, err); got {
		case "gv1":
			onGV++
		case "hosted1":
			onHosted++
		default:
			t.Fatalf("untrusted fork PR job placed on %q, want gv1 or hosted1 only", got)
		}
	}
	if onGV+onHosted != 4 {
		t.Fatalf("placements: gv=%d hosted=%d", onGV, onHosted)
	}

	// 2. A fork PR that asks for any secret class is placed nowhere.
	for _, class := range []model.SecretClass{"project", "environment", "team", "release", "deploy"} {
		id, a, err := r.submit(newJob("sec-"+string(class), "untrusted", "restricted", class), "untrusted", settle)
		waitsBecause(t, r, id, a, err, "secrets.untrusted")
	}

	// 3. A fork PR that asks for LAN or privileged network is placed nowhere.
	for _, scope := range []model.NetworkScope{"lan", "privileged"} {
		id, a, err := r.submit(newJob("net-"+string(scope), "untrusted", scope), "untrusted", settle)
		waitsBecause(t, r, id, a, err, "network.lan_for_untrusted")
	}

	// 4. Every delivery to the hosted fake carried the untrusted attempt only, and no delivery went to the LAN node.
	for _, d := range r.tr.deliveries() {
		if !d.Hosted && d.Node.Identity.ID == "lan1" {
			t.Fatalf("a fork PR lease was delivered to the LAN node: %+v", d)
		}
	}
	if onHosted > 0 && !hostedSawRequest(r.hosted) {
		t.Fatal("the hosted fake saw no dispatch although jobs were placed on it")
	}
}

// TestScenario7_OnlyLANNodeLeft shows the fork PR waits when the only capacity is a LAN node.
func TestScenario7_OnlyLANNodeLeft(t *testing.T) {
	r := newRig(t, repoPolicy(t, "public", "untrusted"), lanRunner("lan1", 4))
	id, a, err := r.submit(newJob("forkwait", "untrusted", "restricted"), "untrusted", settle)
	waitsBecause(t, r, id, a, err, "network.lan_for_untrusted")
	if n := len(r.tr.deliveries()); n != 0 {
		t.Fatalf("%d deliveries although nothing may run", n)
	}
}

// TestScenario7_NoGVisorNoHostedWaits: with neither a sandbox nor a hosted-disposable runner
// the isolation floor keeps the job queued; it is never lowered to a container or a VM-less node.
func TestScenario7_NoGVisorNoHostedWaits(t *testing.T) {
	weak := gvisorRunner("weak1", 2)
	weak.Decl.Isolation = "container"
	r := newRig(t, repoPolicy(t, "public", "untrusted"), weak)
	id, a, err := r.submit(newJob("forkfloor", "untrusted", "restricted"), "untrusted", settle)
	waitsBecause(t, r, id, a, err, "isolation.below_minimum")
}

// TestScenario8_PrivateNeverHosted saturates the LAN and checks private work waits for it
// rather than going to the hosted provider.
func TestScenario8_PrivateNeverHosted(t *testing.T) {
	eff := repoPolicy(t, "private", "owner")
	r := newRig(t, eff, lanRunner("lan1", 1), hostedRunner("hosted1"))

	// The one LAN slot is taken by the first job.
	_, a, err := r.submit(newJob("held", "owner", "lan"), "owner", 2*time.Second)
	if got := runnerOf(t, a, err); got != "lan1" {
		t.Fatalf("first private job on %q, want lan1", got)
	}
	// Saturation: the next private jobs (owner and internal, with and without secrets) wait.
	cases := []sched.Job{
		newJob("p-owner", "owner", "lan"),
		newJob("p-internal", "internal", "internet"),
		newJob("p-secret", "owner", "lan", "project"),
		newJob("p-collab", "collaborator", "internet"),
	}
	for _, j := range cases {
		id, a, err := r.submit(j, string(j.Trust), settle)
		waitsBecause(t, r, id, a, err, "privacy.hosted_not_authorized")
	}
	// Nothing was handed to the hosted provider: no lease, no source revision, no dispatch.
	for _, d := range r.tr.deliveries() {
		if d.Hosted {
			t.Fatalf("a private job (source revision %s) was delivered to the hosted provider", d.Revision)
		}
	}
	if hostedSawRequest(r.hosted) {
		t.Fatal("hosted provider fake received a request for private work")
	}
	for _, dec := range r.sink.all() {
		if dec.Placement != nil && dec.Placement.RunnerID == "hosted1" {
			t.Fatalf("job %s was placed on the hosted runner", dec.Job.Key)
		}
	}
}

// TestScenario8_LocalOnlyStaysOnPersonalNode: with the project's privacy zone set to local-only by the
// operator, work runs on the operator's own personal node and nowhere else: not on another LAN node,
// not on the hosted provider.
func TestScenario8_LocalOnlyStaysOnPersonalNode(t *testing.T) {
	st := openStore(t)
	put(t, st, "project", "project", "trust.privacy_zone", `"local-only"`)
	res := policy.Resolver{Store: st, Run: gitRunner}
	got, err := res.Resolve(context.Background(), policy.Repo{Root: t.TempDir(), Project: "project", Visibility: "private", Class: "owner"}, policy.Revision{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Values["trust.privacy_zone"] != "local-only" {
		t.Fatalf("zone %v", got.Values["trust.privacy_zone"])
	}
	other, mine := lanRunner("lan-other", 2), lanRunner("mine", 2)
	mine.Decl.PersonalOwnNode = true

	r := newRig(t, got.Effective, other, hostedRunner("hosted1"))
	id, a, err := r.submit(newJob("local-nobody", "owner", "lan"), "owner", settle)
	waitsBecause(t, r, id, a, err, "privacy.local_only")

	r = newRig(t, got.Effective, other, mine, hostedRunner("hosted1"))
	for i := 0; i < 2; i++ {
		_, a, err = r.submit(newJob("local-mine"+string(rune('a'+i)), "owner", "lan"), "owner", 2*time.Second)
		if runnerOf(t, a, err) != "mine" {
			t.Fatalf("local-only work placed on %q", a.RunnerID)
		}
	}
	if hostedSawRequest(r.hosted) {
		t.Fatal("local-only work reached the hosted provider")
	}
}
