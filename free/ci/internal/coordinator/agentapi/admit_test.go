package agentapi

// Purpose: tests of the job-token seam (P7-NODE-08): header parsing, the Call the hook sees, and a refusal
//          answered with a cancel while the stream stays open.
// Inputs:  the rig with the real lease engine and store; a recording Admit hook.
// Constraints: a refused frame never reaches the sink; the stream survives a refusal; the token never
//          appears in a frame the coordinator sends.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

type refusal struct{ msg string }

func (r refusal) Error() string { return r.msg }
func (r refusal) Refusal() bool { return true }

type admitRec struct {
	mu    sync.Mutex
	calls []Call
	err   error
}

func (a *admitRec) hook(_ context.Context, c Call) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, c)
	return a.err
}

func (a *admitRec) last() Call {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls[len(a.calls)-1]
}

func (a *admitRec) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
}

func (a *agent) openStreamTokens(lastSeq uint64, tokens ...string) *upStream {
	a.r.t.Helper()
	pr, pw := io.Pipe()
	rq := a.req(http.MethodPost, "/v1/agent/stream", pr, map[string]string{HeaderLastSeq: strconv.FormatUint(lastSeq, 10)})
	for _, tk := range tokens {
		rq.Header.Add(HeaderToken, tk)
	}
	ch := make(chan *http.Response, 1)
	go func() {
		resp, err := a.cl.Do(rq)
		if err != nil {
			a.r.t.Error(err)
		}
		ch <- resp
	}()
	var resp *http.Response
	select {
	case resp = <-ch:
	case <-time.After(5 * time.Second):
		a.r.t.Fatal("stream response headers never arrived")
	}
	if resp == nil || resp.StatusCode != 200 {
		a.r.t.Fatalf("stream = %v", resp)
	}
	a.r.t.Cleanup(func() { pw.Close(); resp.Body.Close() })
	return &upStream{w: pw, fr: newFrameReader(resp.Body), body: resp.Body, t: a.r.t, seq: lastSeq}
}

func TestParseTokens(t *testing.T) {
	got := parseTokens([]string{"lease-a=tok.en.one", "lease-b=x", "noequals", "=empty", "trailing=", "a=b=c"})
	want := map[string]string{"lease-a": "tok.en.one", "lease-b": "x", "a=b": "c"}
	if len(got) != len(want) {
		t.Fatalf("parsed %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("parsed %v, want %v", got, want)
		}
	}
	many := make([]string, 0, maxTokens+10)
	for i := 0; i < maxTokens+10; i++ {
		many = append(many, "l"+strconv.Itoa(i)+"=t")
	}
	if n := len(parseTokens(many)); n != maxTokens {
		t.Fatalf("kept %d tokens, want the cap %d", n, maxTokens)
	}
}

func TestAdmitSeesTokenSerialSessionAndType(t *testing.T) {
	rec := &admitRec{}
	r := newRig(t, func(c *Config) { c.Admit = rec.hook })
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	a.connect()
	u := a.openStreamTokens(0, leaseID+"=tok-1", "other=tok-2")
	u.send(protocol.Message{Type: "log", LeaseID: leaseID, Epoch: &epoch, Body: protocol.Log{LeaseID: leaseID, LineSeq: 1, Stream: "stdout", Text: "hi"}})
	eventually(t, 5*time.Second, "the log reaches the sink", func() bool { r.sink.mu.Lock(); defer r.sink.mu.Unlock(); return len(r.sink.logs) == 1 })
	c := rec.last()
	if c.Node != "n-1" || c.Session != a.session || c.LeaseID != leaseID || c.Epoch != epoch || c.Type != "log" || c.Token != "tok-1" || c.Reconcile {
		t.Fatalf("call = %+v", c)
	}
	if want := strings.ToLower(a.cert.Leaf.SerialNumber.Text(16)); c.Serial == "" || len(c.Serial)%2 != 0 || !strings.HasSuffix(want, strings.TrimLeft(c.Serial, "0")) {
		t.Fatalf("serial %q does not match the certificate %q", c.Serial, want)
	}
}

func TestAdmitHeartbeatReconcileFlag(t *testing.T) {
	rec := &admitRec{}
	r := newRig(t, func(c *Config) { c.Admit = rec.hook })
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	a.connect()
	u := a.openStreamTokens(0, leaseID+"=tok-1")
	u.send(heartbeat(protocol.LeaseState{LeaseID: leaseID, Epoch: epoch, State: "running"}))
	if m := u.read(); m.Type != "ack" {
		t.Fatalf("first heartbeat answer %s", m.Type)
	}
	u.send(heartbeat(protocol.LeaseState{LeaseID: leaseID, Epoch: epoch, State: "running"}, protocol.LeaseState{LeaseID: leaseID, Epoch: epoch, State: "refused"}))
	if m := u.read(); m.Type != "ack" {
		t.Fatalf("second heartbeat answer %s", m.Type)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.calls) != 2 || !rec.calls[0].Reconcile || rec.calls[1].Reconcile || rec.calls[0].Type != "heartbeat" {
		t.Fatalf("calls = %+v (a refused-state entry is not job work and is not vetted)", rec.calls)
	}
}

func TestAdmitRefusalCancelsLeaseAndKeepsStream(t *testing.T) {
	rec := &admitRec{err: refusal{"E661 wrong_session: the token is bound to another session"}}
	r := newRig(t, func(c *Config) { c.Admit = rec.hook })
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	a.connect()
	u := a.openStreamTokens(0, leaseID+"=SECRET-TOKEN-VALUE")
	u.send(resultMsg(leaseID, epoch))
	m := u.read()
	c, ok := m.Body.(*protocol.Cancel)
	if !ok || m.Type != "cancel" || c.LeaseID != leaseID || m.Epoch == nil || *m.Epoch != epoch || !strings.HasPrefix(c.Reason, "E661 wrong_session") {
		t.Fatalf("answer = %s %+v", m.Type, m.Body)
	}
	if strings.Contains(c.Reason, "SECRET-TOKEN-VALUE") {
		t.Fatal("the cancel names the token")
	}
	r.sink.mu.Lock()
	n := len(r.sink.results)
	r.sink.mu.Unlock()
	if n != 0 {
		t.Fatal("a refused result reached the sink")
	}
	rec.mu.Lock()
	rec.err = nil
	rec.mu.Unlock()
	u.send(heartbeat())
	if m = u.read(); m.Type != "ack" {
		t.Fatalf("after a refusal the stream answered %s, want it to stay usable", m.Type)
	}
}

func TestAdmitHeartbeatRefusalSkipsRenewalAndWithholdsAck(t *testing.T) {
	rec := &admitRec{err: refusal{"E661 expired: the token expired"}}
	r := newRig(t, func(c *Config) { c.Admit = rec.hook })
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	a.connect()
	u := a.openStreamTokens(0, leaseID+"=tok")
	u.send(heartbeat(protocol.LeaseState{LeaseID: leaseID, Epoch: epoch, State: "running"}))
	if m := u.read(); m.Type != "cancel" {
		t.Fatalf("answer %s, want a cancel", m.Type)
	}
	u.send(heartbeat())
	if m := u.read(); m.Type != "ack" {
		t.Fatalf("an empty heartbeat after a refusal answered %s", m.Type)
	}
	if rec.count() != 1 {
		t.Fatalf("hook called %d times", rec.count())
	}
}

func TestAdmitHookFailureIsUnavailableNotRefusal(t *testing.T) {
	rec := &admitRec{err: errors.New("database is locked")}
	r := newRig(t, func(c *Config) { c.Admit = rec.hook })
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	a.connect()
	u := a.openStreamTokens(0, leaseID+"=tok")
	u.send(protocol.Message{Type: "log", LeaseID: leaseID, Epoch: &epoch, Body: protocol.Log{LeaseID: leaseID, LineSeq: 1, Stream: "stdout", Text: "x"}})
	m := u.read()
	rj, ok := m.Body.(*protocol.Reject)
	if !ok || rj.Code != protocol.CodeInvalid || rj.Reason != "coordinator_unavailable" {
		t.Fatalf("answer = %s %+v", m.Type, m.Body)
	}
}

func TestNoAdmitHookAdmitsEverything(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	a.connect()
	u := a.openStreamTokens(0)
	u.send(protocol.Message{Type: "log", LeaseID: leaseID, Epoch: &epoch, Body: protocol.Log{LeaseID: leaseID, LineSeq: 1, Stream: "stdout", Text: "x"}})
	eventually(t, 5*time.Second, "the log reaches the sink", func() bool { r.sink.mu.Lock(); defer r.sink.mu.Unlock(); return len(r.sink.logs) == 1 })
}
