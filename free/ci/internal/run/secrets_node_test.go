package run

// Purpose: D-1534 on the coordinator side: a TEAM job that declares secrets is placed on a leased node only,
// carries the run's container and timeout controls in its node form, and its lease holds references, never
// a value; every other job keeps the local-only rules it had.

import (
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

func teamRunState(team bool) *runState {
	return &runState{c: &Coordinator{Opts: Options{TeamRun: team, Container: true, Timeout: 10 * time.Minute}}, sess: &Session{}}
}

func TestTeamSecretJobRunsOnALeasedNodeOnly(t *testing.T) {
	for _, tc := range []struct {
		name               string
		team               bool
		job                model.Job
		node               bool
		isolation, timeout string
	}{
		{"team secrets", true, model.Job{Secrets: []string{"project/FOO"}}, true, "container", "10m0s"},
		{"team secrets keep a stricter job", true, model.Job{Secrets: []string{"project/FOO"}, Isolation: "vm", Timeout: "5m"}, true, "vm", "5m"},
		{"team secrets cap a longer timeout", true, model.Job{Secrets: []string{"project/FOO"}, Timeout: "2h"}, true, "container", "10m0s"},
		{"team release stays local and refused", true, model.Job{Kind: "release", Secrets: []string{"project/FOO"}}, false, "", ""},
		{"team without secrets", true, model.Job{}, false, "", ""},
		{"personal secrets stay local", false, model.Job{Secrets: []string{"project/FOO"}}, false, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rs := teamRunState(tc.team)
			got, node := rs.nodeSecretJob(tc.job)
			onNode := tc.node || tc.team && len(tc.job.Secrets) == 0 // an ordinary TEAM job runs on a node too (P7-WIRE-01)
			if node != tc.node || rs.localOnly(tc.job) == onNode {
				t.Fatalf("node=%v localOnly=%v; want node=%v", node, rs.localOnly(tc.job), tc.node)
			}
			if node && (got.Isolation != model.Isolation(tc.isolation) || got.Timeout != tc.timeout) {
				t.Fatalf("node form isolation=%q timeout=%q; want %q %q", got.Isolation, got.Timeout, tc.isolation, tc.timeout)
			}
		})
	}
}

func TestTeamSecretJobIsPlacedRemoteWithItsNodeForm(t *testing.T) {
	rs := teamRunState(true)
	sess := &Session{RunID: "r1", LocalOnly: rs.localOnly, NodeJob: rs.nodeSecretJob}
	rd := &runDispatch{sess: sess}
	job := model.Job{Secrets: []string{"project/FOO"}}
	spec := AttemptSpec{Job: &job, N: 1}
	sj := rd.schedJob(spec, "r1.j.a1", "r1.j", 1, 256)
	if !sj.RemoteOnly || sj.Requirements.IsolationMin != "container" {
		t.Fatalf("sched job remote=%v isolation_min=%q; want remote-only at container", sj.RemoteOnly, sj.Requirements.IsolationMin)
	}
	for _, l := range sj.Requirements.Labels {
		if l == world.LocalLabel {
			t.Fatal("a TEAM secret job was pinned to the coordinator's own machine")
		}
	}
	q := queueJob(sess, spec, sj, "r1.j.a1", 1, 256)
	if q.Model.Isolation != "container" || q.Model.Timeout != "10m0s" || job.Isolation != "" {
		t.Fatalf("queued model isolation=%q timeout=%q (session job isolation %q)", q.Model.Isolation, q.Model.Timeout, job.Isolation)
	}
}

func TestTeamLeaseCarriesReferencesOnly(t *testing.T) {
	d := dispatch.Delivery{AttemptID: "a1", Trust: "internal", Job: model.Job{Secrets: []string{"project/FOO"}}}
	d.Node.SecretsEligible = true
	refs, err := leaseSecretRefs(d)
	if err != nil || len(refs) != 1 || refs[0].Ref != "project/FOO" || refs[0].Class != "project" {
		t.Fatalf("refs %+v err %v", refs, err)
	}
	d.Trust = "untrusted"
	refs, err = leaseSecretRefs(d)
	if secrets.CodeOf(err) != "E677" || len(refs) != 0 {
		t.Fatalf("an untrusted TEAM lease got refs %+v err %v", refs, err)
	}
}
