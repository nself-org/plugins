package agentapi

// Purpose: tests of the stream, control and lease routes over the real listener: stale epoch E662,
//          reconnect reconciliation (running -> renewed, self_fenced -> lost), resume, cancel delivery
//          and the lease long-poll.
// Inputs:  the rig with the real lease engine and store. Outputs: pass or fail.
// Constraints: a stale result never reaches the sink; time-based waits are bounded and short.

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// upStream is an open POST /v1/agent/stream: frames go in through w, answers come out of fr.
type upStream struct {
	w    *io.PipeWriter
	fr   *frameReader
	body io.ReadCloser
	t    *testing.T
	seq  uint64
}

func (a *agent) openStream(lastSeq uint64) *upStream {
	a.r.t.Helper()
	pr, pw := io.Pipe()
	rq := a.req(http.MethodPost, "/v1/agent/stream", pr, map[string]string{HeaderLastSeq: strconv.FormatUint(lastSeq, 10)})
	type out struct {
		resp *http.Response
		err  error
	}
	ch := make(chan out, 1)
	go func() { resp, err := a.cl.Do(rq); ch <- out{resp, err} }()
	// the response headers arrive once the handler wrote them; send nothing yet
	var o out
	select {
	case o = <-ch:
	case <-time.After(5 * time.Second):
		a.r.t.Fatal("stream response headers never arrived")
	}
	if o.err != nil || o.resp.StatusCode != 200 {
		a.r.t.Fatalf("stream = %v %v", o.resp, o.err)
	}
	a.r.t.Cleanup(func() { pw.Close(); o.resp.Body.Close() })
	return &upStream{w: pw, fr: newFrameReader(o.resp.Body), body: o.resp.Body, t: a.r.t, seq: lastSeq}
}

func (u *upStream) send(m protocol.Message) {
	u.t.Helper()
	u.seq++
	m.Seq = u.seq
	if _, err := u.w.Write(frame(u.t, m)); err != nil {
		u.t.Fatalf("write frame: %v", err)
	}
}

func (u *upStream) read() protocol.Message {
	u.t.Helper()
	type res struct {
		m   protocol.Message
		err error
	}
	ch := make(chan res, 1)
	go func() { m, err := u.fr.next(); ch <- res{m, err} }()
	select {
	case r := <-ch:
		if r.err != nil {
			u.t.Fatalf("read answer: %v", r.err)
		}
		return r.m
	case <-time.After(5 * time.Second):
		u.t.Fatal("no answer from the coordinator")
		return protocol.Message{}
	}
}

func heartbeat(leases ...protocol.LeaseState) protocol.Message {
	if leases == nil {
		leases = []protocol.LeaseState{}
	}
	return protocol.Message{Type: "heartbeat", Body: protocol.Heartbeat{Leases: leases, Slots: protocol.Slots{Total: 2}}}
}

func resultMsg(leaseID string, epoch uint64) protocol.Message {
	return protocol.Message{Type: "result", LeaseID: leaseID, Epoch: &epoch, Body: protocol.Result{LeaseID: leaseID, Epoch: epoch, Evidence: []byte(`{}`), Signature: protocol.Signature{KeyID: "k", Value: "v"}}}
}

func TestStaleEpochRejected(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	a.connect()
	u := a.openStream(0)

	u.send(resultMsg(leaseID, epoch+1)) // a fenced-out attempt reports with an epoch the coordinator never issued
	m := u.read()
	c, ok := m.Body.(*protocol.Cancel)
	if !ok || m.Type != "cancel" || c.LeaseID != leaseID || m.Epoch == nil || *m.Epoch != epoch+1 {
		t.Fatalf("answer = %s %+v, want a cancel for the stale lease", m.Type, m.Body)
	}
	if want := "E662"; len(c.Reason) < 4 || c.Reason[:4] != want {
		t.Fatalf("cancel reason %q, want it to start with E662 and tell the agent to clean its workspace", c.Reason)
	}
	if len(r.sink.results) != 0 {
		t.Fatal("a stale result reached the sink")
	}
	l, err := r.st.LeaseGet(t.Context(), leaseID)
	if err != nil || l.State != model.JobState("running") {
		t.Fatalf("lease after a stale result: %+v %v", l, err)
	}
	if r.eng.StaleCount() == 0 {
		t.Fatal("the stale result was not counted")
	}

	u.send(resultMsg(leaseID, epoch)) // the current epoch is accepted and acknowledged
	ack, ok := u.read().Body.(*protocol.Ack)
	if !ok || ack.AckSeq != 2 || ack.LeaseID != leaseID {
		t.Fatalf("ack = %+v, want ack_seq 2 for %s", ack, leaseID)
	}
	if len(r.sink.results) != 1 {
		t.Fatalf("sink has %d results, want 1", len(r.sink.results))
	}
	if l, _ = r.st.LeaseGet(t.Context(), leaseID); l.State != model.JobState("finalizing") {
		t.Fatalf("attempt state %s after the result, want finalizing", l.State)
	}
}

func TestStaleLogAndUnknownLease(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	a.connect()
	u := a.openStream(0)
	logMsg := func(id string, e uint64) protocol.Message {
		return protocol.Message{Type: "log", LeaseID: id, Epoch: &e, Body: protocol.Log{LeaseID: id, LineSeq: 1, Stream: "stdout", Text: "hello"}}
	}
	u.send(logMsg("no-such-lease", epoch))
	if m := u.read(); m.Type != "cancel" {
		t.Fatalf("unknown lease answered %s, want cancel", m.Type)
	}
	u.send(logMsg(leaseID, epoch))
	u.send(heartbeat()) // the heartbeat's ack proves the log before it was handled
	if m := u.read(); m.Type != "ack" {
		t.Fatalf("answer = %s, want ack", m.Type)
	}
	if len(r.sink.logs) != 1 || r.sink.logs[0].Text != "hello" {
		t.Fatalf("sink logs = %+v", r.sink.logs)
	}
}

func TestReconnectReportsLeases(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	running, e1 := r.seedLease("1")
	fenced, e2 := r.seedLease("2", "n-1")
	a.connect()
	u := a.openStream(0)
	u.send(heartbeat(
		protocol.LeaseState{LeaseID: running, Epoch: e1, State: "running"},
		protocol.LeaseState{LeaseID: fenced, Epoch: e2, State: "self_fenced"}))
	if m := u.read(); m.Type != "ack" {
		t.Fatalf("answer = %s %+v, want ack", m.Type, m.Body)
	}
	if l, _ := r.st.LeaseGet(t.Context(), running); l.State != model.JobState("running") {
		t.Fatalf("running lease is %s, want it renewed and still running", l.State)
	}
	if l, err := r.st.LeaseGet(t.Context(), fenced); err == nil && l.State == model.JobState("running") {
		t.Fatalf("self-fenced lease is still %s, want lost", l.State)
	}
	found := false
	for _, d := range r.auditDetail("a2") {
		found = found || strings.Contains(d, "agent.self_fenced")
	}
	if !found {
		t.Fatalf("audit of a2 = %v, want a row with agent.self_fenced", r.auditDetail("a2"))
	}
	// later heartbeats renew; a stale epoch is answered with a cancel and no ack is withheld for the others
	u.send(heartbeat(protocol.LeaseState{LeaseID: running, Epoch: e1, State: "running"}))
	if m := u.read(); m.Type != "ack" {
		t.Fatalf("renewal answered %s", m.Type)
	}
}

func TestStreamResume(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	a.connect()
	u := a.openStream(0)
	u.send(heartbeat())
	u.read()
	u.w.Close()

	rq := a.req(http.MethodPost, "/v1/agent/stream", io.NopCloser(&emptyReader{}), map[string]string{HeaderLastSeq: "0"})
	resp, err := a.cl.Do(rq) // the coordinator accepted seq 1, so a claim of 0 is refused
	if err != nil || resp.StatusCode != http.StatusConflict || resp.Header.Get(HeaderLastSeq) != "1" {
		t.Fatalf("resume with the wrong seq: %v %v", resp, err)
	}
	resp.Body.Close()
	u2 := a.openStream(1)
	u2.send(heartbeat())
	if m := u2.read(); m.Type != "ack" || m.Body.(*protocol.Ack).AckSeq != 2 {
		t.Fatalf("resumed stream answered %s %+v, want ack of seq 2", m.Type, m.Body)
	}
}

type emptyReader struct{}

func (*emptyReader) Read([]byte) (int, error) { return 0, io.EOF }

func TestStreamRefusesUnusedFrameType(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	a.connect()
	u := a.openStream(0)
	u.send(protocol.Message{Type: "resume", Body: protocol.Resume{LastSeq: 0}})
	m := u.read()
	rej, ok := m.Body.(*protocol.Reject)
	if !ok || rej.Code != protocol.CodeInvalid || rej.Reason != "frame_type" {
		t.Fatalf("answer = %s %+v, want reject E653 frame_type", m.Type, m.Body)
	}
}

func TestCancelAndControlStream(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	if err := r.srv.Cancel("n-1", leaseID, "user"); err == nil {
		t.Fatal("Cancel without a connected node succeeded")
	}
	a.connect()
	if err := r.srv.Cancel("n-1", leaseID, "user"); err == nil {
		t.Fatal("Cancel without a control stream succeeded")
	}
	resp, err := a.cl.Do(a.req(http.MethodGet, "/v1/agent/control", nil, nil))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("control: %v %v", resp, err)
	}
	defer resp.Body.Close()
	eventually(t, 2*time.Second, "the control stream to count as live", func() bool {
		return r.srv.Cancel("n-1", leaseID, "user cancel") == nil // the lease epoch comes from the store
	})
	fr := newFrameReader(resp.Body)
	m, err := fr.next()
	if err != nil {
		t.Fatal(err)
	}
	c, ok := m.Body.(*protocol.Cancel)
	if !ok || c.LeaseID != leaseID || c.Reason != "user cancel" || m.Epoch == nil || *m.Epoch != epoch || m.Seq != 1 {
		t.Fatalf("control frame = %+v %+v, want cancel for %s epoch %d", m, m.Body, leaseID, epoch)
	}
	if err = r.srv.Drain("n-1", time.Unix(1_900_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	if m, err = fr.next(); err != nil || m.Type != "drain" || m.Seq != 2 {
		t.Fatalf("second control frame = %+v %v, want drain seq 2", m, err)
	}
	if got := r.srv.NodesConnected(); len(got) != 1 || got[0] != "n-1" {
		t.Fatalf("NodesConnected = %v", got)
	}
}

func TestLeaseLongPoll(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	a.connect()
	start := time.Now()
	resp, err := a.cl.Do(a.req(http.MethodGet, "/v1/agent/leases", nil, map[string]string{HeaderWait: "1"}))
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("empty poll: %v %v", resp, err)
	}
	resp.Body.Close()
	if d := time.Since(start); d < 900*time.Millisecond || d > 3*time.Second {
		t.Fatalf("poll took %v, want about 1 s", d)
	}

	got := make(chan *http.Response, 1)
	go func() {
		resp, err := a.cl.Do(a.req(http.MethodGet, "/v1/agent/leases", nil, map[string]string{HeaderWait: "20"}))
		if err != nil {
			t.Error(err)
		}
		got <- resp
	}()
	time.Sleep(100 * time.Millisecond)
	lease := protocol.Lease{LeaseID: "l-9", AttemptID: "a-9", Epoch: 4294967297, Token: "opaque", CompatMode: protocol.Compat15, Job: model.Job{}, Secrets: []protocol.SecretRef{}, ExpiresAt: "2030-01-01T00:00:00Z"}
	if err = r.srv.Offer("n-1", lease); err != nil {
		t.Fatal(err)
	}
	select {
	case resp = <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("the offered lease was not delivered")
	}
	defer resp.Body.Close()
	m, err := newFrameReader(resp.Body).next()
	if err != nil {
		t.Fatal(err)
	}
	l, ok := m.Body.(*protocol.Lease)
	if !ok || l.LeaseID != "l-9" || m.Epoch == nil || *m.Epoch != 4294967297 || m.LeaseID != "l-9" {
		t.Fatalf("delivered %s %+v", m.Type, m.Body)
	}
	if r.srv.Offer("", lease) == nil || r.srv.Offer("n-1", protocol.Lease{}) == nil {
		t.Fatal("Offer accepted a lease without a node or an id")
	}
}

func TestOnAckReportsLeaseAcks(t *testing.T) {
	type got struct{ node, lease string }
	acks := make(chan got, 4)
	r := newRig(t, func(c *Config) { c.OnAck = func(n, l string) { acks <- got{n, l} } })
	a := r.newAgent("n-1")
	held, _ := r.seedLease("7", "n-1")
	foreign, _ := r.seedLease("8", "n-other") // a lease of another node: its ack from n-1 is dropped
	a.connect()
	u := a.openStream(0)
	u.send(protocol.Message{Type: "ack", Body: protocol.Ack{AckSeq: 3}}) // no lease id: not reported
	u.send(protocol.Message{Type: "ack", Body: protocol.Ack{AckSeq: 5, LeaseID: foreign}})
	u.send(protocol.Message{Type: "ack", Body: protocol.Ack{AckSeq: 4, LeaseID: held}})
	u.send(heartbeat())
	u.read() // the heartbeat ack proves both acks were handled
	select {
	case g := <-acks:
		if g != (got{"n-1", held}) {
			t.Fatalf("OnAck = %+v, want node n-1 lease %s", g, held)
		}
	default:
		t.Fatal("OnAck was not called")
	}
	select {
	case g := <-acks:
		t.Fatalf("unexpected extra OnAck %+v", g)
	default:
	}
}

// TestNodeCannotSpeakForAnotherNodesLease: a result, a log and a heartbeat for a lease another node holds end
// the stream (E653 lease_not_held) and reach neither the lease engine nor the sink.
func TestNodeCannotSpeakForAnotherNodesLease(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  func(id string, epoch uint64) protocol.Message
	}{
		{"result", resultMsg},
		{"heartbeat", func(id string, epoch uint64) protocol.Message {
			return heartbeat(protocol.LeaseState{LeaseID: id, Epoch: epoch, State: "running"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, nil)
			a := r.newAgent("n-1")
			foreign, epoch := r.seedLease("9", "n-other")
			a.connect()
			u := a.openStream(0)
			u.send(tc.msg(foreign, epoch))
			if m := u.read(); m.Type != "reject" {
				t.Fatalf("answer = %s %+v, want a reject", m.Type, m.Body)
			}
			if len(r.sink.results) != 0 {
				t.Fatalf("the sink received a result for a lease the node does not hold")
			}
			if l, _ := r.st.LeaseGet(t.Context(), foreign); l.State != model.JobState("running") {
				t.Fatalf("foreign lease moved to %s", l.State)
			}
		})
	}
}

// TestStopOfferingHandsOutNothing: a draining coordinator answers the lease long-poll with 204 and keeps the
// queued lease out of the agent's hands.
func TestStopOfferingHandsOutNothing(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	a.connect()
	lease := protocol.Lease{LeaseID: "l-10", AttemptID: "a-10", Epoch: 4294967297, Token: "opaque", CompatMode: protocol.Compat15, Job: model.Job{}, Secrets: []protocol.SecretRef{}, ExpiresAt: "2030-01-01T00:00:00Z"}
	if err := r.srv.Offer("n-1", lease); err != nil {
		t.Fatal(err)
	}
	r.srv.StopOffering()
	resp, err := a.cl.Do(a.req(http.MethodGet, "/v1/agent/leases", nil, map[string]string{HeaderWait: "5"}))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d while draining, want 204", resp.StatusCode)
	}
}
