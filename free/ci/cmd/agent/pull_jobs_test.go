package main

// Purpose: end-to-end properties of `pull` that need a real agent process group: a cancel kills the job's
//          processes within 5 s, a coordinator that stops acknowledging heartbeats is outlived by no job (the
//          agent fences itself before the lease expires and old and new attempts never overlap), and a secret
//          resolved on the node reaches the job and never the coordinator.
// Inputs:  the pull rig with a proxy in front of the listener; shell jobs with unique markers.
// Constraints: unix only; bounded waits; fake secret values.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
)

func shellLease(leaseID, att string, ep uint64, script string, secrets ...protocol.SecretRef) protocol.Lease {
	if secrets == nil {
		secrets = []protocol.SecretRef{}
	}
	declared := []string{}
	for _, r := range secrets {
		declared = append(declared, r.Ref[strings.LastIndex(r.Ref, "/")+1:]) // a job lists the variable names
	}
	return protocol.Lease{LeaseID: leaseID, AttemptID: att, Epoch: ep, CompatMode: protocol.Compat15,
		Job:      model.Job{Kind: "static", Run: []string{"sh", "-c", script}, Secrets: declared},
		Checkout: protocol.Checkout{Revision: strings.Repeat("a", 40)}, Secrets: secrets, ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}
}

func (r *pullRig) deliver(l protocol.Lease) <-chan error {
	var node model.Capability
	node.Identity.ID = "n-1"
	ch := make(chan error, 1)
	return func() <-chan error {
		go func() { ch <- r.drv.Run(context.Background(), node, l, r.sink) }()
		return ch
	}()
}

func pgrepCount(pattern string) int {
	out, _ := exec.Command("pgrep", "-f", pattern).Output()
	return len(strings.Fields(string(out)))
}

func TestPullCancelKillsTheJobsProcessGroupWithinFiveSeconds(t *testing.T) {
	skipUnixOnly(t)
	state := useState(t)
	r := newPullRig(t)
	r.enroll(state, "n-1", r.base)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startPull(t, ctx)
	pullWait(t, 15*time.Second, "the agent to connect", func() bool { return len(r.srv.NodesConnected()) == 1 })

	marker := "sleep " + strconv.Itoa(40000+os.Getpid()%9000)
	leaseID, att, ep := r.seed("1", "n-1")
	l := shellLease(leaseID, att, ep, "echo job-started; "+marker+" & wait")
	var node model.Capability
	node.Identity.ID = "n-1"
	runCtx, stopRun := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.drv.Run(runCtx, node, l, r.sink) }()
	pullWait(t, 15*time.Second, "the job to start", func() bool { return strings.Contains(r.sink.text(), "job-started") && pgrepCount(marker) > 0 })

	stopRun()
	began := time.Now()
	pullWait(t, 5*time.Second, "the job's processes to be gone", func() bool { return pgrepCount(marker) == 0 })
	if took := time.Since(began); took > 5*time.Second {
		t.Fatalf("the process group lived %v after the cancel", took)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after a cancel with a result = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after the cancel")
	}
}

func TestPullFencesItsJobBeforeTheLeaseExpiresAndAttemptsNeverOverlap(t *testing.T) {
	skipUnixOnly(t)
	state := useState(t)
	r := newPullRigWith(t, rigOpts{heartbeat: time.Second, coordTTL: 5 * time.Second, leaseTTL: 5 * time.Second, proxy: true})
	r.enroll(state, "n-1", r.base)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startPull(t, ctx)
	pullWait(t, 15*time.Second, "the agent to connect", func() bool { return len(r.srv.NodesConnected()) == 1 })

	stamps := filepath.Join(t.TempDir(), "stamps")
	loop := "while :; do date +%s%N >> " + stamps + "; sleep 0.1; done"
	leaseID, att, ep := r.seed("1", "n-1")
	done := r.deliver(shellLease(leaseID, att, ep, loop))
	pullWait(t, 15*time.Second, "the job to write timestamps", func() bool { return len(readStamps(t, stamps)) > 5 })
	// let a heartbeat or two be acknowledged so the fence is armed with a fresh ack
	time.Sleep(2500 * time.Millisecond)

	r.px.partition() // heartbeats and their acknowledgements stop
	cut := time.Now()
	pullWait(t, 15*time.Second, "the agent to kill its job", func() bool { return pgrepCount("date \\+%s%N") == 0 && stampsStopped(t, stamps) })
	lastOld := lastStamp(t, stamps)
	killedAfter := time.Since(cut)
	if killedAfter > 4500*time.Millisecond {
		t.Fatalf("the job lived %v after the partition; the agent fences at agent_ttl (3 s) after the last ack", killedAfter)
	}
	// the coordinator's lease for the old attempt is still valid when the job is already gone
	l1, err := r.st.LeaseGet(context.Background(), leaseID)
	if err != nil || l1.ExpiresAt == 0 {
		t.Fatalf("the lease must still be on record when the job is gone: %+v %v", l1, err)
	}
	if time.Unix(0, lastOld).After(time.Unix(0, l1.ExpiresAt)) {
		t.Fatalf("the job outlived its lease: last output %v, lease expiry %v", time.Unix(0, lastOld), time.Unix(0, l1.ExpiresAt))
	}

	r.px.heal()
	// the old attempt is lost on the coordinator; a new attempt starts on the node
	if err := r.eng.Reconnect(context.Background(), leaseID, int64(ep), "self_fenced"); err != nil {
		t.Logf("reconnect report: %v (the agent's own heartbeat may have reported it first)", err)
	}
	_ = done
	leaseID2, att2, ep2 := r.seed("2", "n-1")
	pullWait(t, 20*time.Second, "the agent to reconnect", func() bool { return len(r.srv.NodesConnected()) == 1 })
	before := len(readStamps(t, stamps))
	done2 := r.deliver(shellLease(leaseID2, att2, ep2, loop))
	pullWait(t, 20*time.Second, "the new attempt to write", func() bool { return len(readStamps(t, stamps)) > before+3 })
	all := readStamps(t, stamps)
	for i := 1; i < len(all); i++ {
		if all[i] < all[i-1] {
			t.Fatalf("timestamps out of order at %d: the old and the new attempt overlapped", i)
		}
	}
	if gap := time.Duration(all[before] - lastOld); gap <= 0 {
		t.Fatalf("the new attempt wrote %v before the old one stopped", -gap)
	}
	_ = done2
}

func readStamps(t *testing.T, path string) []int64 {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []int64
	for _, f := range strings.Fields(string(b)) {
		if n, err := strconv.ParseInt(f, 10, 64); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func lastStamp(t *testing.T, path string) int64 {
	t.Helper()
	s := readStamps(t, path)
	if len(s) == 0 {
		t.Fatal("no timestamps")
	}
	return s[len(s)-1]
}

// stampsStopped says no timestamp arrived in the last 700 ms.
func stampsStopped(t *testing.T, path string) bool {
	t.Helper()
	s := readStamps(t, path)
	return len(s) > 0 && time.Since(time.Unix(0, s[len(s)-1])) > 700*time.Millisecond
}

func TestPullResolvesSecretsOnTheNodeAndTheCoordinatorNeverSeesThem(t *testing.T) {
	skipUnixOnly(t)
	state := useState(t)
	const canary = "canary-secret-value-9f3a77"
	old := pullResolver
	pullResolver = func(string, pullFlags, string, string) agent.SecretResolver {
		return func(_ context.Context, _ string, _ model.TrustClass, refs []string) (map[string]string, error) {
			if len(refs) != 1 || refs[0] != "project/API_TOKEN" {
				t.Errorf("refs = %v", refs)
			}
			return map[string]string{"API_TOKEN": canary}, nil
		}
	}
	defer func() { pullResolver = old }()
	r := newPullRig(t)
	r.enroll(state, "n-1", r.base)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, errb := startPull(t, ctx)
	pullWait(t, 15*time.Second, "the agent to connect", func() bool { return len(r.srv.NodesConnected()) == 1 })

	leaseID, att, ep := r.seed("1", "n-1")
	done := r.deliver(shellLease(leaseID, att, ep, `echo "length:${#API_TOKEN}"; echo "echoed:$API_TOKEN"`, protocol.SecretRef{Ref: "project/API_TOKEN", Class: "project"}))
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("the job never finished:\n%s", r.sink.text())
	}
	out := r.sink.text()
	for _, res := range r.sink.results {
		t.Logf("evidence: %s", res.Evidence)
	}
	if !strings.Contains(out, "length:"+strconv.Itoa(len(canary))) {
		t.Fatalf("the job did not receive the secret (the node resolver ran, the value must reach the job):\n%s\nagent stderr:\n%s", out, errb.String())
	}
	r.sink.mu.Lock()
	defer r.sink.mu.Unlock()
	if strings.Contains(out, canary) {
		t.Fatalf("the secret value reached the coordinator in the job output:\n%s", out)
	}
	for _, res := range r.sink.results {
		if strings.Contains(string(res.Evidence), canary) {
			t.Fatal("the secret value reached the coordinator in the evidence")
		}
	}
}
