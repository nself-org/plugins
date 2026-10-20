package agent

// Purpose: tests of the HTTP half of the client: the clock skew check on hello, the token travelling in a
//          header only (canary over everything the agent writes), the refusal and flow helpers.
// Inputs:  the e2e rig. Constraints: bounded waits; no real secret values.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func TestHelloRefusesAClockMoreThanTwoMinutesOff(t *testing.T) {
	e := newE2E(t, "")
	e.addNode("n-1")
	cert := e.nodeCert("n-1")
	hello, _ := encode(e.hello(), 1)
	for _, c := range []struct {
		name    string
		offset  time.Duration
		wantErr bool
	}{{"in step", 0, false}, {"119 s fast", 119 * time.Second, false}, {"121 s fast", 121 * time.Second, true}, {"121 s slow", -121 * time.Second, true}, {"10 min slow", -10 * time.Minute, true}} {
		tlsc, _ := pki.TLSClientConfig(pki.ClientOptions{RootDER: e.ca.RootDER(), Cert: &cert, ServerName: "127.0.0.1"})
		var seen time.Duration
		cl := newClient(Options{BaseURL: e.base, TLS: tlsc, Now: func() time.Time { return time.Now().Add(-c.offset) }, OnSkew: func(d time.Duration) { seen = d }})
		_, _, err := cl.hello(context.Background(), hello)
		cl.close()
		if (err != nil) != c.wantErr {
			t.Fatalf("%s: err = %v", c.name, err)
		}
		if c.wantErr && pki.ReasonOf(err) != pki.ReasonClockSkew {
			t.Fatalf("%s: err = %v, want E660 clock_skew", c.name, err)
		}
		if d := seen - c.offset; d < -3*time.Second || d > 3*time.Second {
			t.Fatalf("%s: reported skew %v, want about %v", c.name, seen, c.offset)
		}
	}
}

// recordingTransport keeps every request body and header an agent sends.
type recordingTransport struct {
	rt      http.RoundTripper
	mu      sync.Mutex
	bodies  bytes.Buffer
	headers []http.Header
}

func (r *recordingTransport) RoundTrip(rq *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.headers = append(r.headers, rq.Header.Clone())
	r.mu.Unlock()
	if rq.Body != nil {
		rq.Body = io.NopCloser(io.TeeReader(rq.Body, &lockedWriter{r}))
	}
	return r.rt.RoundTrip(rq)
}

type lockedWriter struct{ r *recordingTransport }

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.r.mu.Lock()
	defer w.r.mu.Unlock()
	return w.r.bodies.Write(p)
}

func TestJobTokenTravelsInAHeaderAndNeverInAFrame(t *testing.T) {
	e := newE2E(t, "")
	e.addNode("n-1")
	cert := e.nodeCert("n-1")
	tlsc, _ := pki.TLSClientConfig(pki.ClientOptions{RootDER: e.ca.RootDER(), Cert: &cert, ServerName: "127.0.0.1"})
	rec := &recordingTransport{rt: &http.Transport{TLSClientConfig: tlsc, ForceAttemptHTTP2: true}}
	c := e.carrier(cert, func(cfg *CarrierConfig) { cfg.Client.Transport = rec })
	if err := c.Send(e.hello()); err != nil {
		t.Fatal(err)
	}
	recvType(t, c, "welcome", 5*time.Second)
	run := e.startLease(t, c, "n-1", "1")
	for i := 1; i <= 3; i++ {
		run.log("line")
	}
	run.beat()
	recvType(t, c, "heartbeat", 5*time.Second)
	run.finish()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.bodies.Len() == 0 {
		t.Fatal("recorded no request body")
	}
	if bytes.Contains(rec.bodies.Bytes(), []byte(run.token)) {
		t.Fatal("the job token appears in a frame the agent sent")
	}
	carried := false
	for _, h := range rec.headers {
		for _, v := range h.Values(agentapi.HeaderToken) {
			carried = carried || strings.HasSuffix(v, "="+run.token)
		}
	}
	if !carried {
		t.Fatal("no request carried the job token header")
	}
}

func TestRefusalCarriesTheBodyAndTheLastSeq(t *testing.T) {
	resp := &http.Response{StatusCode: 409, Header: http.Header{agentapi.HeaderLastSeq: {"17"}}, Body: io.NopCloser(strings.NewReader(`{"code":"E653","reason":"resume_seq","message":"x"}`))}
	err := refusal(resp)
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != 409 || he.Reason != "resume_seq" || he.Code != "E653" || he.LastSeq != "17" || he.noSession() {
		t.Fatalf("refusal = %+v", err)
	}
	resp = &http.Response{StatusCode: 409, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"code":"E653","reason":"no_session"}`))}
	if !refusal(resp).(*HTTPError).noSession() {
		t.Fatal("no_session not recognised")
	}
	resp = &http.Response{StatusCode: 502, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("<html>bad gateway"))}
	he = refusal(resp).(*HTTPError)
	if he.Status != 502 || he.Reason != "" || !strings.Contains(he.Error(), "502") {
		t.Fatalf("non-JSON refusal = %+v", he)
	}
}

func TestFlowReaderStartsAtWhateverSeqTheCoordinatorSends(t *testing.T) {
	var buf bytes.Buffer
	enc, _ := protocol.NewResumedEncoder(&buf, 6, 6)
	for i := uint64(0); i < 2; i++ {
		if err := enc.Encode(protocol.Message{V: 1, Type: "drain", Seq: 7 + i, Body: protocol.Drain{Deadline: "2030-01-01T00:00:00Z"}}); err != nil {
			t.Fatal(err)
		}
	}
	fr := &flowReader{body: &buf}
	for want := uint64(7); want <= 8; want++ {
		m, err := fr.next()
		if err != nil || m.Seq != want {
			t.Fatalf("frame %d: %+v %v", want, m, err)
		}
	}
	if _, err := fr.next(); !errors.Is(err, io.EOF) {
		t.Fatalf("after the last frame: %v", err)
	}
}

func TestFirstFrameRefusesGarbageAndHugeLines(t *testing.T) {
	for name, in := range map[string]string{
		"not json":  "garbage\n",
		"no seq":    `{"v":1,"type":"drain","seq":0,"body":{}}` + "\n",
		"too large": strings.Repeat("x", protocol.MaxFrame+10) + "\n",
	} {
		if _, err := firstFrame(bufio.NewReader(strings.NewReader(in)), protocol.CarrierPolicy{Carrier: "B"}); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := firstFrame(strings.NewReader(`{"v":1,"type":"drain","seq":3`), protocol.CarrierPolicy{Carrier: "B"}); err == nil {
		t.Fatal("an unterminated line was accepted")
	}
}

func TestClientTLSUsesTLS13OnlyAndTheGivenConfig(t *testing.T) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS13}
	cl := newClient(Options{BaseURL: "https://127.0.0.1:1", TLS: cfg})
	defer cl.close()
	tr := cl.hc.Transport.(*http.Transport)
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS13 || tr.TLSClientConfig == cfg {
		t.Fatalf("transport TLS %+v (must be a clone with TLS 1.3)", tr.TLSClientConfig)
	}
}
