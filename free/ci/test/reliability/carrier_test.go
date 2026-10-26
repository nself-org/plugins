package reliability

// Purpose: carrier faults. Agent carrier: an interrupted connection (network drop and restore, kill -9 of
//          the coordinator mid log stream, a partial log upload) resumes with no duplicate log line,
//          result or artifact reference. SSH carrier: session loss marks the attempt lost and it is
//          retried elsewhere; no resume is claimed.
// Inputs:  the coordinator's lease log writer (internal/transport/agent) and the sim world.
// Outputs: assertions only.
// Constraints: no sleeps; artifact references are recorded only behind the lease fence, as the hub does.

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	agentcarrier "github.com/nself-org/plugins/free/ci/internal/transport/agent"
	"github.com/nself-org/plugins/free/ci/internal/transport/ssh"
)

// logSeqs reads the line_seq values of a lease log file in order.
func logSeqs(t *testing.T, path string) []uint64 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out []uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var l struct {
			LineSeq uint64 `json:"line_seq"`
		}
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatalf("log line %q: %v", sc.Text(), err)
		}
		out = append(out, l.LineSeq)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func wantSeqs(t *testing.T, path string, n uint64) {
	t.Helper()
	got := logSeqs(t, path)
	if uint64(len(got)) != n {
		t.Fatalf("log has %d lines %v, want %d", len(got), got, n)
	}
	for i, s := range got {
		if s != uint64(i)+1 {
			t.Fatalf("log line %d has line_seq %d: %v", i, s, got)
		}
	}
}

// send appends lines from..to of a lease; it returns how many were written (not duplicates).
func send(t *testing.T, lw *agentcarrier.LogWriter, leaseID string, from, to uint64) int {
	t.Helper()
	n := 0
	for s := from; s <= to; s++ {
		ok, err := lw.Append(protocol.Log{LeaseID: leaseID, LineSeq: s, Stream: "stdout", Text: fmt.Sprintf("line %d", s)})
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			n++
		}
	}
	return n
}

func openLog(t *testing.T, path string) *agentcarrier.LogWriter {
	t.Helper()
	lw, err := agentcarrier.OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lw.Close() })
	return lw
}

// artifacts is the coordinator's artifact index: a reference is kept only while the lease fence holds.
type artifacts map[string]int

func (x artifacts) add(w *world, a *agent, path string) error {
	if err := w.engine.Fence(w.ctx, a.leaseID, a.epoch); err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(path))
	x[fmt.Sprintf("%s/%s/%x", a.leaseID, path, sum[:4])]++
	return nil
}

// wantOnce asserts one artifact reference, recorded once.
func (x artifacts) wantOnce(t *testing.T) {
	t.Helper()
	for _, n := range x {
		if n != 1 || len(x) != 1 {
			t.Fatalf("artifact index %v", x)
		}
	}
	if len(x) != 1 {
		t.Fatalf("artifact index %v", x)
	}
}

func TestFaultAgentCarrierResume(t *testing.T) {
	w := newWorld(t)
	a1 := w.job("jres", "test", true, 1)
	a := w.startAgent("node-a", "lease-a", w.grant(a1, "lease-a", "node-a", 1))
	path := agentcarrier.LogFile(t.TempDir(), "lease-a")
	lw := openLog(t, path)
	idx := artifacts{}
	if n := send(t, lw, "lease-a", 1, 4); n != 4 {
		t.Fatalf("first burst wrote %d", n)
	}
	if err := idx.add(w, a, "dist/app.tar"); err != nil {
		t.Fatal(err)
	}
	a.partitioned = true // Wi-Fi drop: the acks for lines 3-4 and the artifact never reach the agent
	w.run(heartbeat, a)
	a.partitioned = false
	if err := a.reconnect(); err != nil {
		t.Fatal(err)
	}
	if n := send(t, lw, "lease-a", 3, 6); n != 2 { // the agent resends from its last acknowledged line
		t.Fatalf("resend wrote %d lines, want 2", n)
	}
	idx.wantOnce(t)
	if ran, err := a.finish("jres"); !ran || err != nil {
		t.Fatalf("finish: ran=%t err=%v", ran, err)
	}
	wantCode(t, "resent result", a.lateResult(), "E605") // the finalizing CAS refuses a second result
	wantCode(t, "resent artifact reference", idx.add(w, a, "dist/app.tar"), "E662")
	idx.wantOnce(t)
	if err := lw.Sync(); err != nil {
		t.Fatal(err)
	}
	wantSeqs(t, path, 6)
	w.wantAttempts("jres", "finalizing/")
	w.wantEffect("jres", "node-a")
}

func TestFaultCoordinatorKilledMidLogStream(t *testing.T) {
	path := agentcarrier.LogFile(t.TempDir(), "lease-k")
	lw, err := agentcarrier.OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	send(t, lw, "lease-k", 1, 3)
	if err := lw.Sync(); err != nil {
		t.Fatal(err)
	}
	_ = lw.Close()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"line_seq":4,"stream":"std`); err != nil { // kill -9 mid write
		t.Fatal(err)
	}
	_ = f.Close()
	lw = openLog(t, path)
	if lw.Last() != 3 {
		t.Fatalf("recovered tail %d, want 3", lw.Last())
	}
	if n := send(t, lw, "lease-k", 2, 5); n != 2 {
		t.Fatalf("resume wrote %d, want 2", n)
	}
	wantSeqs(t, path, 5)
}

func TestFaultPartialLogUpload(t *testing.T) {
	path := agentcarrier.LogFile(t.TempDir(), "lease-p")
	lw := openLog(t, path)
	send(t, lw, "lease-p", 1, 2)
	// The upload of lines 3-5 was cut after line 3 reached the coordinator.
	send(t, lw, "lease-p", 3, 3)
	if n := send(t, lw, "lease-p", 3, 5); n != 2 {
		t.Fatalf("re-upload wrote %d, want 2", n)
	}
	if err := lw.Sync(); err != nil {
		t.Fatal(err)
	}
	wantSeqs(t, path, 5)
}

func TestFaultSSHSessionLoss(t *testing.T) {
	w := newWorld(t)
	a1 := w.job("jssh", "test", true, 1)
	a := w.startAgent("node-a", "lease-a", w.grant(a1, "lease-a", "node-a", 1))
	w.run(heartbeat, a)
	l, err := w.store.LeaseGet(w.ctx, "lease-a")
	if err != nil {
		t.Fatal(err)
	}
	a.dead = true // the ssh session carried the job; it ends with the session
	if err := w.store.LeaseFail(w.ctx, l, w.clock(), ssh.ReasonSessionLost, false); err != nil {
		t.Fatal(err)
	}
	if got := w.retryReasons(a1); len(got) != 1 || got[0] != ssh.ReasonSessionLost {
		t.Fatalf("retry reasons %v", got)
	}
	wantCode(t, "resume over a new session", w.engine.Reconnect(w.ctx, "lease-a", a.epoch, "running"), "E662")
	wantCode(t, "late result", a.lateResult(), "E662")
	rows := w.wantAttempts("jssh", "retryable/lost", "queued/")
	retryElsewhere(t, w, "jssh", rows)
	w.wantAttempts("jssh", "retryable/lost", "finalizing/")
	w.wantEffect("jssh", "node-b")
}
