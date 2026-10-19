//go:build integration

package ssh

// Purpose: TestSSHTransport, the acceptance scenarios of P7-NODE-03 against a real
// sshd and the real nself-ci-agent. One subtest per acceptance line.
// Inputs:  INTEGRATION=1 (see integration_rig_test.go).
// Constraints: no bare sleeps (polls with a deadline); the host-key change runs
// last because it replaces the node's key.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

func TestSSHTransport(t *testing.T) {
	r := newITRig(t)
	t.Run("streams_line_by_line_and_job_root_absent_after_finalize", r.streams)
	t.Run("cancel_kills_the_remote_process_group_within_5s", r.cancel)
	t.Run("operator_ssh_config_cannot_forward_multiplex_or_run_commands", r.hostileConfig)
	t.Run("session_drop_is_lost_and_the_node_kills_the_process_group", r.sessionDrop)
	t.Run("secret_value_never_reaches_argv_env_process_list_logs_or_job_root", r.secretCanary)
	t.Run("checkout_with_a_wrong_digest_is_refused_before_any_step", r.digestMismatch)
	t.Run("blob_flipped_byte_rejected_and_interrupted_blob_resumes", func(t *testing.T) {
		TestSSHTransportBlobWithAFlippedByteIsRejectedByDigest(t)
		TestSSHTransportInterruptedBlobResumesFromTheReportedOffset(t)
	})
	t.Run("black_holed_node_fails_with_E656_within_15s", r.blackHole)
	t.Run("changed_host_key_refuses_with_E655_before_any_command_runs", r.hostKeyChange)
}

func (r *itRig) streams(t *testing.T) {
	sink := newLineSink(nil)
	l := r.lease("it-stream", "sh", "-c", `git rev-parse HEAD; for i in 1 2 3; do echo "line-$i $(date +%s%N)"; sleep 0.5; done`)
	if err := r.drv.Run(context.Background(), r.node("n1"), l, sink); err != nil {
		t.Fatal(err)
	}
	if got := sink.lines(); len(got) < 4 || got[0] != r.rev {
		t.Fatalf("job did not run on the leased revision: %q", got)
	}
	var first, last time.Time
	for text, at := range sink.at {
		f := strings.Fields(text)
		if len(f) != 2 || !strings.HasPrefix(f[0], "line-") {
			continue
		}
		ns, _ := strconv.ParseInt(f[1], 10, 64)
		if lag := at.Sub(time.Unix(0, ns)); lag > time.Second {
			t.Errorf("%s arrived %v after it was printed", f[0], lag)
		}
		if f[0] == "line-1" {
			first = at
		}
		if f[0] == "line-3" {
			last = at
		}
	}
	if last.Sub(first) < 800*time.Millisecond {
		t.Errorf("lines arrived together (%v apart), not as they were printed", last.Sub(first))
	}
	deadline := time.Now().Add(5 * time.Second)
	for r.sh("n1", "ls -A "+agentDir+"/jobs "+agentDir+"/checkout 2>/dev/null | wc -l") != "0" {
		if time.Now().After(deadline) {
			t.Fatal("a job root is left on the node after finalize")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (r *itRig) cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var once sync.Once
	var cancelled time.Time
	sink := newLineSink(func(s string) {
		if s == "started" {
			once.Do(func() { cancelled = time.Now(); cancel() })
		}
	})
	err := r.drv.Run(ctx, r.node("n1"), r.lease("it-cancel", "sh", "-c", "echo started; sleep 301 & sleep 302"), sink)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if !r.pgrepGone("[s]leep 30[12]", 5*time.Second-time.Since(cancelled)+time.Second) {
		t.Fatalf("job processes survived cancel: %s", r.sh("n1", "ps -eo pid,pgid,args | grep '[s]leep 30'"))
	}
}

func (r *itRig) sessionDrop(t *testing.T) {
	d := r.driver()
	var conn *Conn
	var once sync.Once
	d.Start = func(ctx context.Context, tg remote.Target, c string) (*Conn, error) {
		cn, err := startRemote(ctx, tg, c)
		conn = cn
		return cn, err
	}
	sink := newLineSink(func(s string) {
		if s == "started" {
			once.Do(func() { go func() { _ = conn.Kill() }() })
		}
	})
	err := d.Run(context.Background(), r.node("n1"), r.lease("it-drop", "sh", "-c", "echo started; sleep 303"), sink)
	var te *Error
	if !IsLost(err) || !errors.As(err, &te) || te.Code != CodeTransport || len(sink.results) != 0 {
		t.Fatalf("err = %v, want %s %s", err, CodeTransport, ReasonSessionLost)
	}
	if !r.pgrepGone("[s]leep 303", 5*time.Second) {
		t.Fatal("the node kept the job after the session was lost")
	}
}

func (r *itRig) digestMismatch(t *testing.T) {
	d := r.driver()
	d.afterBundle = func(p string) {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		b[len(b)/2] ^= 0xff
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sink := newLineSink(nil)
	_ = d.Run(context.Background(), r.node("n1"), r.lease("it-digest", "sh", "-c", "echo STEP-RAN"), sink)
	if strings.Contains(strings.Join(sink.lines(), "\n"), "STEP-RAN") {
		t.Fatal("a job step ran on a checkout with a wrong digest")
	}
	for _, res := range sink.results {
		if strings.Contains(string(res.Evidence), `"result":"pass"`) {
			t.Fatal("a passing result for a refused checkout")
		}
	}
}

func (r *itRig) blackHole(t *testing.T) {
	r.fleet.Netem(t, "n2", 0, 100)
	start := time.Now()
	err := r.drv.Run(context.Background(), r.node("n2"), r.lease("it-hole", "true"), newLineSink(nil))
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeTransport {
		t.Fatalf("err = %v, want %s", err, CodeTransport)
	}
	if took := time.Since(start); took > 15*time.Second {
		t.Fatalf("a black-holed node took %v to fail", took)
	}
}

func (r *itRig) hostKeyChange(t *testing.T) {
	logs := func() string {
		out, _ := exec.Command("docker", "logs", r.fleet.Node(t, "n1").Container).CombinedOutput()
		return string(out)
	}
	r.sh("n1", "rm -f /etc/ssh/ssh_host_*")
	r.fleet.Restart(t, "n1")
	before := strings.Count(logs(), "Accepted publickey")
	err := r.drv.Run(context.Background(), r.node("n1"), r.lease("it-key", "sh", "-c", "touch /tmp/should-not-run"), newLineSink(nil))
	var te *Error
	if !errors.As(err, &te) || te.Code != CodeHostKey || !strings.Contains(te.Message, "nodes rotate") {
		t.Fatalf("err = %v, want %s with the rotate remediation", err, CodeHostKey)
	}
	if after := strings.Count(logs(), "Accepted publickey"); after != before {
		t.Fatalf("sshd accepted a login (%d -> %d) before the host key was checked", before, after)
	}
	if r.sh("n1", "test -e /tmp/should-not-run && echo ran || echo no") != "no" {
		t.Fatal("a command ran on the node")
	}
}
