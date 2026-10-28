package enroll

// Purpose: the enrollment handler and limiter with a fake Redeemer (the real stack is exercised by
// TestEnroll in internal/pairing): error mapping, what the Redeemer receives, the per-source limit, the
// bounded source table and the listener's Close.
// Inputs:  httptest requests. Outputs: pass/fail. Constraints: an injected clock, no sleeps over 200 ms.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRedeemer struct {
	mu   sync.Mutex
	got  []Request
	srcs []string
	resp Response
	err  error
}

func (f *fakeRedeemer) Hello() Hello {
	return Hello{Version: Version, CoordinatorID: "c1", ServerTime: 42}
}

func (f *fakeRedeemer) Redeem(_ context.Context, q Request, src string) (Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got, f.srcs = append(f.got, q), append(f.srcs, src)
	return f.resp, f.err
}

type codedErr struct{ code, reason, msg string }

func (e codedErr) Error() string     { return e.code + ": " + e.msg + " (" + e.reason + ")" } // the format of pairing.Error and pki.Error
func (e codedErr) ErrCode() string   { return e.code }
func (e codedErr) ErrReason() string { return e.reason }

type tick struct {
	mu sync.Mutex
	t  time.Time
}

func (c *tick) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *tick) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func send(h http.Handler, method, path, remote, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func enrollBody(code string) string {
	b, _ := json.Marshal(Request{Phase: PhaseEnroll, Version: Version, ClientTime: 1, Code: code, Name: "n1", CSR: []byte{1}, NodeKey: []byte{2}})
	return string(b)
}

func TestEnrollHandlerMapsErrors(t *testing.T) {
	const code = "ABCDEFGHJK"
	for _, tc := range []struct {
		name   string
		err    error
		status int
		body   ErrorBody
		hides  []string
	}{
		{"E659 reason", codedErr{"E659", "denied", "not approved"}, http.StatusForbidden, ErrorBody{Code: "E659", Reason: "denied"}, nil},
		{"E660 skew is a bad request", codedErr{"E660", "clock_skew", "clocks differ"}, http.StatusBadRequest, ErrorBody{Code: "E660", Reason: "clock_skew"}, nil},
		{"E660 other is forbidden", codedErr{"E660", "bad_csr", "csr"}, http.StatusForbidden, ErrorBody{Code: "E660", Reason: "bad_csr"}, nil},
		{"a plain error never reaches the client", errors.New("sqlite: /home/admin/.nself/ci/state.db is locked, code " + code), http.StatusInternalServerError,
			ErrorBody{Code: "E659", Reason: "internal"}, []string{"sqlite", "/home/admin", code}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := Handler(&fakeRedeemer{err: tc.err}, HandlerOptions{Now: (&tick{t: time.Unix(1e6, 0)}).now})
			rec := send(h, http.MethodPost, Route, "192.0.2.5:4000", enrollBody(code))
			var got ErrorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != tc.status || got.Code != tc.body.Code || got.Reason != tc.body.Reason {
				t.Fatalf("%d %s (%v)", rec.Code, rec.Body.String(), err)
			}
			if ce, ok := tc.err.(codedErr); ok && got.Message != ce.msg {
				t.Fatalf("message %q repeats the code or the reason, want %q", got.Message, ce.msg)
			}
			for _, secret := range tc.hides {
				if strings.Contains(rec.Body.String(), secret) {
					t.Fatalf("the reply holds %q: %s", secret, rec.Body.String())
				}
			}
			if rec.Header().Get("Content-Type") != "application/json" || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("headers %v", rec.Header())
			}
		})
	}
}

func TestEnrollHandlerPassesTheRequest(t *testing.T) {
	f := &fakeRedeemer{resp: Response{NodeID: "n1", Cert: []byte{9}, NotAfter: 77}}
	h := Handler(f, HandlerOptions{Now: (&tick{t: time.Unix(1e6, 0)}).now})
	rec := send(h, http.MethodPost, Route, "[2001:db8::1]:5555", enrollBody("ABCDEFGHJK"))
	var got Response
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.NodeID != "n1" || got.NotAfter != 77 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if len(f.got) != 1 || f.srcs[0] != "2001:db8::1" || f.got[0].Code != "ABCDEFGHJK" || f.got[0].Name != "n1" || !bytes.Equal(f.got[0].CSR, []byte{1}) {
		t.Fatalf("redeemer saw %+v from %v", f.got, f.srcs)
	}
	hello := send(h, http.MethodPost, Route, "192.0.2.9:1", `{"phase":"hello","version":1,"client_time":5}`)
	if hello.Code != 200 || !strings.Contains(hello.Body.String(), `"coordinator_id":"c1"`) || len(f.got) != 1 {
		t.Fatalf("hello %d %s redeemer calls %d", hello.Code, hello.Body.String(), len(f.got))
	}
	odd := send(h, http.MethodPost, Route, "no-port-here", `{"phase":"hello","version":1}`)
	if odd.Code != 200 {
		t.Fatalf("a RemoteAddr without a port must still be served: %d", odd.Code)
	}
}

func TestEnrollHandlerRejectsBadRequests(t *testing.T) {
	f := &fakeRedeemer{}
	h := Handler(f, HandlerOptions{Now: (&tick{t: time.Unix(1e6, 0)}).now})
	n := 0
	do := func(method, path, body string) *httptest.ResponseRecorder {
		n++
		return send(h, method, path, "192.0.2."+strconv.Itoa(n)+":1", body) // one source per request: no limiter noise
	}
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodHead} {
		if rec := do(m, Route, ""); rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodPost {
			t.Errorf("%s: %d", m, rec.Code)
		}
	}
	for _, p := range []string{"/", "/v1", "/v1/enrol", "/v1/enroll/x", "/%2e%2e/v1/enroll"} {
		if rec := do(http.MethodPost, p, enrollBody("ABCDEFGHJK")); rec.Code == http.StatusOK || rec.Code == http.StatusForbidden {
			t.Errorf("POST %s reached the route: %d", p, rec.Code)
		}
	}
	over := `{"phase":"hello","version":1,"code":"` + strings.Repeat("A", MaxBody) + `"}`
	if rec := do(http.MethodPost, Route, over); rec.Code != http.StatusBadRequest && rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: %d", rec.Code)
	}
	for _, body := range []string{`{"phase":"enroll","version":1,"code":"ABCDEFGHJK","x":1}`, `{"phase":"enroll","version":3}`, `{"phase":"?","version":1}`, `{}`, ``, `nul`} {
		rec := do(http.MethodPost, Route, body)
		if rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), "ABCDEFGHJK") {
			t.Errorf("%q: %d %s", body, rec.Code, rec.Body.String())
		}
	}
	if len(f.got) != 0 {
		t.Fatalf("the redeemer saw %d bad requests", len(f.got))
	}
}

func TestEnrollLimiter(t *testing.T) {
	c := &tick{t: time.Unix(1e6, 0)}
	f := &fakeRedeemer{}
	h := Handler(f, HandlerOptions{Now: c.now})
	hello := `{"phase":"hello","version":1}`
	if send(h, http.MethodPost, Route, "192.0.2.1:1000", hello).Code != 200 {
		t.Fatal("first request")
	}
	if rec := send(h, http.MethodPost, Route, "192.0.2.1:2000", hello); rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("same host, other port: %d", rec.Code)
	}
	if send(h, http.MethodPost, Route, "192.0.2.2:1000", hello).Code != 200 {
		t.Fatal("another source must not be limited")
	}
	c.add(900 * time.Millisecond)
	if send(h, http.MethodPost, Route, "192.0.2.1:1000", hello).Code != http.StatusTooManyRequests {
		t.Fatal("0.9 s")
	}
	c.add(100 * time.Millisecond) // 1.0 s after the allowed one: the refused attempts did not extend the block
	if send(h, http.MethodPost, Route, "192.0.2.1:1000", hello).Code != 200 {
		t.Fatal("exactly one interval later must pass")
	}
	// A limited request never reaches the Redeemer, whatever it carries.
	c.add(10 * time.Millisecond)
	send(h, http.MethodPost, Route, "192.0.2.1:1000", enrollBody("ABCDEFGHJK"))
	if len(f.got) != 0 {
		t.Fatal("a rate-limited enroll reached the redeemer")
	}
}

func TestEnrollLimiterTableIsBounded(t *testing.T) {
	l := NewLimiter()
	t0 := time.Unix(1e6, 0)
	for i := 0; i < maxSources+10; i++ {
		l.Allow("src-"+strconv.Itoa(i), t0)
	}
	if len(l.last) != maxSources {
		t.Fatalf("table %d, want it capped at %d", len(l.last), maxSources)
	}
	if l.Allow("one-more", t0) {
		t.Fatal("a new source was admitted while the table is full of fresh entries")
	}
	if !l.Allow("src-0", t0.Add(2*Interval)) {
		t.Fatal("a known source was refused after its interval")
	}
	if !l.Allow("fresh", t0.Add(2*time.Minute)) {
		t.Fatal("a new source after a quiet minute")
	}
	if len(l.last) != 1 {
		t.Fatalf("stale entries were not dropped: %d left", len(l.last))
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				l.Allow("s"+strconv.Itoa(j%20), t0.Add(time.Duration(j)*time.Second))
			}
		}()
	}
	wg.Wait()
}

func TestEnrollListenCloseIsIdempotent(t *testing.T) {
	cfg := testServerTLS(t)
	srv, err := Listen("127.0.0.1:0", cfg, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
	srv.Close()
	select {
	case <-srv.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done never closed")
	}
	if _, err = Listen("256.0.0.1:99999", cfg, http.NotFoundHandler()); err == nil {
		t.Fatal("a bad address was accepted")
	}
}

// An idle keep-alive connection must not outlive the listener's limits (D-1258).
func TestEnrollListenerSetsIdleLimits(t *testing.T) {
	srv, err := Listen("127.0.0.1:0", testServerTLS(t), http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	if srv.http.ReadHeaderTimeout <= 0 || srv.http.IdleTimeout <= 0 {
		t.Fatalf("ReadHeaderTimeout %v, IdleTimeout %v: both must be set", srv.http.ReadHeaderTimeout, srv.http.IdleTimeout)
	}
}
