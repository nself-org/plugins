package agent

// Purpose: tests of the client's four requests against a scripted HTTPS server: what each request carries, how
//          each status is read, and the frame-size bound of the line reader.
// Inputs:  httptest TLS server. Constraints: bounded waits.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

type scriptedServer struct {
	srv  *httptest.Server
	mu   sync.Mutex
	last *http.Request
	body []byte
	resp func(w http.ResponseWriter, r *http.Request)
}

func newScripted2(t *testing.T, resp func(w http.ResponseWriter, r *http.Request)) (*scriptedServer, *client) {
	t.Helper()
	s := &scriptedServer{resp: resp}
	s.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		s.mu.Lock()
		s.last, s.body = r, b
		s.mu.Unlock()
		s.resp(w, r)
	}))
	s.srv.EnableHTTP2 = true
	s.srv.StartTLS()
	t.Cleanup(s.srv.Close)
	pool := x509.NewCertPool()
	pool.AddCert(s.srv.Certificate())
	cl := newClient(Options{BaseURL: s.srv.URL + "/", TLS: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13}, Wait: 7 * time.Second})
	t.Cleanup(cl.close)
	return s, cl
}

func frameBytes(t *testing.T, typ string, seq uint64, body any) []byte {
	t.Helper()
	var b bytes.Buffer
	enc, _ := protocol.NewResumedEncoder(&b, seq-1, seq-1)
	if err := enc.Encode(protocol.Message{V: 1, Type: typ, Seq: seq, Body: body}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func (s *scriptedServer) request() *http.Request { s.mu.Lock(); defer s.mu.Unlock(); return s.last }

func TestHelloReturnsTheWelcomeAndTheSessionHeader(t *testing.T) {
	s, cl := newScripted2(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(agentapi.HeaderSession, "sess-9")
		_, _ = w.Write(frameBytes(t, "welcome", 1, protocol.Welcome{Version: 1, SessionID: "sess-9", HeartbeatMS: 1000, AgentTTLMS: 3000}))
	})
	var skewSeen bool
	cl.o.OnSkew = func(time.Duration) { skewSeen = true }
	m, sess, err := cl.hello(context.Background(), []byte("hello-frame\n"))
	if err != nil || sess != "sess-9" || m.Type != "welcome" || !skewSeen {
		t.Fatalf("hello = %+v %q %v skew=%v", m, sess, err, skewSeen)
	}
	rq := s.request()
	if rq.Method != "POST" || rq.URL.Path != "/v1/agent/hello" || rq.Header.Get(agentapi.HeaderSession) != "" || string(s.body) != "hello-frame\n" {
		t.Fatalf("request %s %s session %q body %q", rq.Method, rq.URL.Path, rq.Header.Get(agentapi.HeaderSession), s.body)
	}
}

func TestHelloReadsAnNDJSONRejectOnAnErrorStatusAndRefusesOtherBodies(t *testing.T) {
	_, cl := newScripted2(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(frameBytes(t, "reject", 1, protocol.Reject{Code: "E652", Reason: "version"}))
	})
	m, _, err := cl.hello(context.Background(), []byte("x\n"))
	if err != nil || m.Type != "reject" {
		t.Fatalf("reject = %+v %v", m, err)
	}
	_, cl2 := newScripted2(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"code":"E660","reason":"wrong_kind","message":"no"}`)
	})
	_, _, err = cl2.hello(context.Background(), []byte("x\n"))
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != 403 || he.Reason != "wrong_kind" {
		t.Fatalf("err = %v", err)
	}
	_, cl3 := newScripted2(t, func(w http.ResponseWriter, r *http.Request) { // 200 with garbage
		_, _ = io.WriteString(w, "not a frame\n")
	})
	if _, _, err = cl3.hello(context.Background(), []byte("x\n")); err == nil {
		t.Fatal("a garbage welcome was accepted")
	}
}

func TestHelloWithoutADateHeaderSkipsTheSkewCheck(t *testing.T) {
	_, cl := newScripted2(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header()["Date"] = nil
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write(frameBytes(t, "welcome", 1, protocol.Welcome{Version: 1, SessionID: "s"}))
	})
	called := false
	cl.o.OnSkew = func(time.Duration) { called = true }
	if _, _, err := cl.hello(context.Background(), []byte("x\n")); err != nil || called {
		t.Fatalf("err %v skew called %v", err, called)
	}
}

func TestLeasePollCarriesTheSessionAndWaitAndReadsStatuses(t *testing.T) {
	status := http.StatusNoContent
	s, cl := newScripted2(t, func(w http.ResponseWriter, r *http.Request) {
		switch status {
		case http.StatusOK:
			w.Header().Set("Content-Type", "application/x-ndjson")
			l := protoLease("l1", "a1", 4)
			var b bytes.Buffer
			enc, _ := protocol.NewResumedEncoder(&b, 4, 4)
			ep := l.Epoch
			if err := enc.Encode(protocol.Message{V: 1, Type: "lease", Seq: 5, LeaseID: l.LeaseID, Epoch: &ep, Body: l}); err != nil {
				t.Error(err)
			}
			_, _ = w.Write(b.Bytes())
		case http.StatusNoContent:
			w.WriteHeader(status)
		default:
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"code":"E653","reason":"no_session"}`)
		}
	})
	m, err := cl.lease(context.Background(), "sess-1")
	if m != nil || err != nil {
		t.Fatalf("204 = %v %v", m, err)
	}
	rq := s.request()
	if rq.Method != "GET" || rq.URL.Path != "/v1/agent/leases" || rq.Header.Get(agentapi.HeaderSession) != "sess-1" || rq.Header.Get(agentapi.HeaderWait) != "7" {
		t.Fatalf("request %s %s %v", rq.Method, rq.URL.Path, rq.Header)
	}
	status = http.StatusOK
	m, err = cl.lease(context.Background(), "sess-1")
	if err != nil || m == nil || m.Seq != 5 || m.Type != "lease" {
		t.Fatalf("200 = %+v %v", m, err)
	}
	status = http.StatusConflict
	if _, err = cl.lease(context.Background(), "sess-1"); err == nil || !err.(*HTTPError).noSession() {
		t.Fatalf("409 = %v", err)
	}
}

func TestStreamRequestCarriesClaimTokensAndContentType(t *testing.T) {
	status := http.StatusOK
	s, cl := newScripted2(t, func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.Header().Set(agentapi.HeaderLastSeq, "12")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"code":"E653","reason":"resume_seq"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	resp, err := cl.stream(context.Background(), "sess-2", 41, map[string]string{"l1": "tok1", "l2": "tok2"}, strings.NewReader("frames"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	rq := s.request()
	got := rq.Header.Values(agentapi.HeaderToken)
	if rq.Method != "POST" || rq.URL.Path != "/v1/agent/stream" || rq.Header.Get(agentapi.HeaderSession) != "sess-2" || rq.Header.Get(agentapi.HeaderLastSeq) != "41" ||
		rq.Header.Get("Content-Type") != "application/x-ndjson" || len(got) != 2 || string(s.body) != "frames" {
		t.Fatalf("request %s %s %v tokens %v body %q", rq.Method, rq.URL.Path, rq.Header, got, s.body)
	}
	if !((got[0] == "l1=tok1" && got[1] == "l2=tok2") || (got[0] == "l2=tok2" && got[1] == "l1=tok1")) {
		t.Fatalf("token values %v", got)
	}
	status = http.StatusConflict
	if _, err = cl.stream(context.Background(), "sess-2", 41, nil, strings.NewReader("")); err == nil || err.(*HTTPError).LastSeq != "12" {
		t.Fatalf("409 = %v", err)
	}
}

func TestControlStreamOpensWithTheSessionAndRefusesErrors(t *testing.T) {
	status := http.StatusOK
	s, cl := newScripted2(t, func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write(frameBytes(t, "drain", 3, protocol.Drain{Deadline: "2030-01-01T00:00:00Z"}))
	})
	fr, closer, err := cl.control(context.Background(), "sess-3")
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	m, err := fr.next()
	if err != nil || m.Type != "drain" || m.Seq != 3 {
		t.Fatalf("control frame %+v %v", m, err)
	}
	if rq := s.request(); rq.URL.Path != "/v1/agent/control" || rq.Header.Get(agentapi.HeaderSession) != "sess-3" {
		t.Fatalf("request %s %v", rq.URL.Path, rq.Header)
	}
	status = http.StatusInternalServerError
	if _, _, err = cl.control(context.Background(), "sess-3"); err == nil || err.(*HTTPError).Status != 500 {
		t.Fatalf("500 = %v", err)
	}
}

func TestTheClientDoesNotFollowRedirects(t *testing.T) {
	s, cl := newScripted2(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/agent/leases" {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
			return
		}
		t.Errorf("followed the redirect to %s", r.URL.Path)
	})
	_ = s
	if _, err := cl.lease(context.Background(), "s"); err == nil || err.(*HTTPError).Status != 302 {
		t.Fatalf("redirect = %v", err)
	}
}

func TestReadLineBoundsAFrameAtOneMiB(t *testing.T) {
	ok := strings.Repeat("a", protocol.MaxFrame-1) + "\n"
	line, err := readLine(bufio.NewReaderSize(strings.NewReader(ok), 4096))
	if err != nil || len(line) != protocol.MaxFrame {
		t.Fatalf("a frame of exactly the maximum: %d %v", len(line), err)
	}
	big := strings.Repeat("a", protocol.MaxFrame) + "\n"
	if _, err = readLine(bufio.NewReaderSize(strings.NewReader(big), 4096)); err == nil {
		t.Fatal("a frame over the maximum was read")
	}
	if _, err = readLine(bufio.NewReaderSize(strings.NewReader(""), 4096)); !errors.Is(err, io.EOF) {
		t.Fatalf("empty = %v", err)
	}
	if _, err = readLine(bufio.NewReaderSize(strings.NewReader("half"), 4096)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("unterminated = %v", err)
	}
}

func TestSkewIsServerMinusAgentClock(t *testing.T) {
	cl := newClient(Options{BaseURL: "https://x", Now: func() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }})
	resp := &http.Response{Header: http.Header{"Date": {"Tue, 01 Jan 2030 00:01:30 GMT"}}}
	if d, ok := cl.skew(resp); !ok || d != 90*time.Second {
		t.Fatalf("skew = %v %v", d, ok)
	}
	if _, ok := cl.skew(&http.Response{Header: http.Header{}}); ok {
		t.Fatal("a missing Date gave a skew")
	}
	_ = pki.ClockSkew
}
