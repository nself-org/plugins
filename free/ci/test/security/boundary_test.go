//go:build security

package security

// Purpose: the production boundary and the telemetry promise.
//   I15 (TM-41): a deploy host is never selected without the operator's override; an override never
//   admits untrusted work, release or deploy secrets or isolation below vm, and every placement on an
//   overridden host leaves an audit row inside the admission transaction; the coordinator host needs
//   vm for team mode and for other people's work.
//   X15 (TM-36): no request reaches a telemetry endpoint from the plugin or the agent in any mode.
//   The telemetry fake of main_test.go is both the ping_api endpoint and the process proxy; this test
//   drives the code that records scheduling decisions with the default sink and asserts it stayed quiet.
// Inputs:  the dispatcher rig; trust.Admissible over the enumerated classes.
// Outputs: pass or fail.
// Constraints: the default sink is the production telemetry sink (internal/schedtel registers it);
//          the suite as a whole fails if the fake ever records a request (main_test.go).

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	_ "github.com/nself-org/plugins/free/ci/internal/schedtel" // registers the telemetry sink the dispatcher uses by default
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// deployHostDecl is the declaration of a LAN node the inventory matched as a deploy host.
func deployHostDecl(isolation model.Isolation, override bool) trust.Declaration {
	d := lanRunner("deploy1", 4).Decl
	d.Isolation = isolation
	d.DeployHost = trust.DeployHost{Matched: true, Override: override}
	return d
}

// TestDeployHostNeverSelectedWithoutOverride (I15): over every trust class, isolation level and
// secret class the product defines, the rule's answer is the one the invariant states.
func TestDeployHostNeverSelectedWithoutOverride(t *testing.T) {
	eff := repoPolicy(t, "private", "owner")
	checked := 0
	for _, class := range model.EnumValues("TrustClass") {
		for _, iso := range model.EnumValues("Isolation") {
			for _, secret := range append([]string{"none"}, model.EnumValues("SecretClass")...) {
				for _, override := range []bool{false, true} {
					decl := deployHostDecl(model.Isolation(iso), override)
					job := trust.JobFacts{Trust: model.TrustClass(class), Network: "restricted", SecretClasses: []model.SecretClass{model.SecretClass(secret)}}
					reasons := trust.Admissible(job, decl, eff)
					boundary := false
					for _, r := range reasons {
						boundary = boundary || r == trust.BoundaryDeployHost || r == trust.BoundaryOverrideMinimum
					}
					own := class == "owner" || class == "internal"
					releaseOrDeploy := secret == "release" || secret == "deploy" || secret != "none" && secret != "project" && secret != "environment" && secret != "team"
					allowed := override && own && trust.RankIsolation(model.Isolation(iso)) >= trust.RankIsolation("vm") && !releaseOrDeploy
					if allowed == boundary {
						t.Fatalf("class=%s isolation=%s secret=%s override=%v: boundary refusal=%v, want %v (%v)", class, iso, secret, override, boundary, !allowed, reasons)
					}
					checked++
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("nothing was checked")
	}
}

// TestDeployHostPlacementThroughDispatcher: the dispatcher keeps work off a deploy host, places an
// owner job on an overridden one only with its audit row, and refuses an untrusted job there.
func TestDeployHostPlacementThroughDispatcher(t *testing.T) {
	plain := lanRunner("lan-ok", 1)
	deploy := lanRunner("deploy1", 4)
	deploy.Decl.DeployHost = trust.DeployHost{Matched: true}

	// 1. Without an override the deploy host is never chosen, even though it has the most room.
	r := newRig(t, repoPolicy(t, "private", "owner"), deploy, plain)
	for i := 0; i < 3; i++ {
		id, a, err := r.submit(newJob("work"+string(rune('a'+i)), "owner", "lan"), "owner", settle)
		if i == 0 {
			if got := runnerOf(t, a, err); got != "lan-ok" {
				t.Fatalf("job placed on %q, want lan-ok", got)
			}
			continue
		}
		waitsBecause(t, r, id, a, err) // the ordinary node has one slot: the rest wait, they do not spill to the deploy host
		if d, ok := r.sink.last(id); ok && d.Placement != nil && d.Placement.RunnerID == "deploy1" {
			t.Fatal("work spilled onto a deploy host without an override")
		}
	}
	// 2. With the override, an owner job on vm is placed there and the audit row exists.
	deploy.Decl.DeployHost.Override = true
	r = newRig(t, repoPolicy(t, "private", "owner"), deploy)
	id, a, err := r.submit(newJob("deploy-owner", "owner", "lan"), "owner", 2*time.Second)
	if got := runnerOf(t, a, err); got != "deploy1" {
		t.Fatalf("overridden deploy host not used by an owner job: %q", got)
	}
	rows, err := r.st.AuditEntries(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		found = found || row.Action == dispatch.DeployHostAuditAction && row.Target == id && strings.Contains(row.Detail, "runner=deploy1")
	}
	if !found {
		t.Fatalf("no %s audit row for %s among %d rows", dispatch.DeployHostAuditAction, id, len(rows))
	}
	// 3. Even overridden, untrusted work and release secrets are refused there.
	r = newRig(t, repoPolicy(t, "public", "untrusted"), deploy)
	id2, a2, err2 := r.submit(newJob("deploy-fork", "untrusted", "restricted"), "untrusted", settle)
	waitsBecause(t, r, id2, a2, err2, "boundary.override_minimum")
	r = newRig(t, repoPolicy(t, "private", "owner"), deploy)
	id3, a3, err3 := r.submit(newJob("deploy-release", "owner", "lan", "release"), "owner", settle)
	waitsBecause(t, r, id3, a3, err3, "boundary.override_minimum")
}

// TestCoordinatorHostFloor (D28): a team coordinator host below vm takes nobody's work, and a
// personal coordinator host below vm takes only its operator's.
func TestCoordinatorHostFloor(t *testing.T) {
	eff := repoPolicy(t, "private", "owner")
	host := lanRunner("coord", 4).Decl
	host.Isolation, host.CoordinatorHost = "container", true
	host.CoordinatorMode = trust.Team
	if got := trust.Admissible(trust.JobFacts{Trust: "owner", Network: "lan"}, host, eff); !hasTrustReason(got, trust.BoundaryCoordinatorHost) {
		t.Errorf("team coordinator host below vm took an owner job: %v", got)
	}
	host.CoordinatorMode = trust.Personal
	if got := trust.Admissible(trust.JobFacts{Trust: "collaborator", Network: "internet"}, host, eff); !hasTrustReason(got, trust.BoundaryCoordinatorHost) {
		t.Errorf("personal coordinator host below vm took a collaborator's job: %v", got)
	}
	if got := trust.Admissible(trust.JobFacts{Trust: "owner", Network: "lan"}, host, eff); hasTrustReason(got, trust.BoundaryCoordinatorHost) {
		t.Errorf("control: a personal coordinator host refused its own operator's job: %v", got)
	}
	host.Isolation = "vm"
	host.CoordinatorMode = trust.Team
	if got := trust.Admissible(trust.JobFacts{Trust: "owner", Network: "lan"}, host, eff); hasTrustReason(got, trust.BoundaryCoordinatorHost) {
		t.Errorf("control: a vm coordinator host was refused: %v", got)
	}
}

// TestNoRequestReachesTelemetry (X15, TM-36): scheduling decisions are recorded through the default
// telemetry sink while the endpoint variables and the process proxy all point at the telemetry
// fake; the fake sees nothing. The fake's own count is also asserted at the end of the suite.
func TestNoRequestReachesTelemetry(t *testing.T) {
	st := openStore(t)
	d, err := dispatch.New(dispatch.Config{Store: st, Host: "host", Transport: &fabricTransport{}, Poll: 10 * time.Millisecond,
		World: worldOf(st, repoPolicy(t, "private", "owner"), lanRunner("lan1", 4), hostedRunner("hosted1"))}) // Sink nil: the production default
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	for i := 0; i < 5; i++ {
		id := queueAttempt(t, st, "tele"+string(rune('a'+i)), "project", "job", "owner")
		j := newJob(id, "owner", "lan")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		a, err := d.Submit(ctx, dispatch.Job{AttemptID: id, Sched: j, Revision: strings.Repeat("a", 40)})
		cancel()
		if err != nil {
			t.Fatalf("job %d: %v", i, err)
		}
		pass(t, st, a.AttemptID)
	}
	time.Sleep(200 * time.Millisecond) // a background sender, if one existed, would have fired by now
	if seen := telemetryRequests(); len(seen) != 0 {
		t.Fatalf("the plugin sent %d request(s) to the telemetry endpoint or proxy: %v", len(seen), seen)
	}
}
