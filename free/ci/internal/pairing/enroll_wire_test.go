package pairing

// Purpose: the HTTP face of TestEnroll: the real enrollment handler behind a real TLS listener, probed
// with a raw client (route and method rules, body cap, strict JSON, rate limit, no echo of the code or
// the Authorization header in any reply or log line) and a graceful close while a redemption is in flight.
// Inputs:  a rig. Outputs: pass/fail. Constraints: the rate-limit clock is the rig clock.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
)

type reply struct {
	status int
	header http.Header
	body   string
}

func (r *rig) do(method, url, body string, hdr map[string]string) reply {
	r.tb.Helper()
	r.clock.Advance(2 * time.Second) // outside the 1 s per-source window of the previous request
	return r.doNoAdvance(method, url, body, hdr)
}

func (r *rig) doNoAdvance(method, url, body string, hdr map[string]string) reply {
	r.tb.Helper()
	tr := &http.Transport{TLSClientConfig: r.clientCfg(nil), DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		r.tb.Fatal(err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Transport: tr, Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		r.tb.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return reply{resp.StatusCode, resp.Header, string(raw)}
}

func jsonBody(t testing.TB, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func enrollHandlerStack(t *testing.T) {
	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	r := newRig(t, nil)
	ap := approving("policy:h", DefaultTrust())
	off := r.offer(OfferOptions{Approver: ap})
	real := r.serveReal()
	base := real.URL
	root := strings.TrimSuffix(base, enroll.Route)
	hello := jsonBody(t, enroll.Request{Phase: enroll.PhaseHello, Version: enroll.Version, ClientTime: r.clock.Now().Unix()})

	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodHead} {
		if got := r.do(m, base, "", nil); got.status != http.StatusMethodNotAllowed || got.header.Get("Allow") != "POST" {
			t.Fatalf("%s: %d %v", m, got.status, got.header)
		}
	}
	for _, p := range []string{"/", "/v1", "/v1/enroll/", "/v1/enroll/x", "/v2/enroll", "/admin"} {
		if got := r.do(http.MethodPost, root+p, hello, nil); got.status != http.StatusNotFound {
			t.Fatalf("POST %s: %d", p, got.status)
		}
	}
	if got := r.do(http.MethodPost, base, hello, nil); got.status != http.StatusOK || !strings.Contains(got.body, `"coordinator_id":"local"`) {
		t.Fatalf("hello: %d %s", got.status, got.body)
	}
	if real.Cnt.hellos.Load() != 1 {
		t.Fatalf("hellos %d", real.Cnt.hellos.Load())
	}

	t.Run("16 KiB body cap", func(t *testing.T) {
		big := `{"phase":"hello","version":1,"client_time":1,"code":"` + strings.Repeat("Q", enroll.MaxBody+1000) + `"}`
		got := r.do(http.MethodPost, base, big, nil)
		if got.status != http.StatusRequestEntityTooLarge && got.status != http.StatusBadRequest {
			t.Fatalf("status %d", got.status)
		}
		if strings.Contains(got.body, "QQQQ") {
			t.Fatal("the reply echoes the body")
		}
		fits := `{"phase":"hello","version":1,"client_time":1,"code":"` + strings.Repeat("Q", 8000) + `"}`
		if got = r.do(http.MethodPost, base, fits, nil); got.status != http.StatusOK {
			t.Fatalf("an 8 KiB body must pass: %d", got.status)
		}
	})

	t.Run("strict JSON", func(t *testing.T) {
		for _, body := range []string{
			`{"phase":"hello","version":1,"client_time":1,"surprise":true}`,
			`{"phase":"hello","version":2,"client_time":1}`,
			`{"phase":"nope","version":1,"client_time":1}`,
			`{"phase":"hello","version":1,"client_time":"x"}`,
			`not json`, ``, `[]`,
		} {
			got := r.do(http.MethodPost, base, body, nil)
			if got.status != http.StatusBadRequest || !strings.Contains(got.body, `"code":"E659"`) {
				t.Fatalf("%q: %d %s", body, got.status, got.body)
			}
		}
	})

	t.Run("one request per second per source", func(t *testing.T) {
		r.clock.Advance(2 * time.Second)
		if got := r.doNoAdvance(http.MethodPost, base, hello, nil); got.status != http.StatusOK {
			t.Fatalf("first: %d", got.status)
		}
		got := r.doNoAdvance(http.MethodPost, base, hello, nil)
		if got.status != http.StatusTooManyRequests || got.header.Get("Retry-After") != "1" {
			t.Fatalf("second inside the second: %d %v", got.status, got.header)
		}
		r.clock.Advance(500 * time.Millisecond)
		if got = r.doNoAdvance(http.MethodPost, base, hello, nil); got.status != http.StatusTooManyRequests {
			t.Fatalf("after 0.5 s: %d", got.status)
		}
		r.clock.Advance(time.Second)
		if got = r.doNoAdvance(http.MethodPost, base, hello, nil); got.status != http.StatusOK {
			t.Fatalf("after the interval: %d", got.status)
		}
	})

	t.Run("the code and the Authorization header never appear in a reply or a log line", func(t *testing.T) {
		m := newNodeMat(t)
		const token, wrong = "Bearer synthetic-token-0123456789", "ZZZZZZZZZZ"
		for _, c := range []string{wrong, off.Code} {
			req := r.request(c, "node-h", m)
			got := r.do(http.MethodPost, base, jsonBody(t, req), map[string]string{"Authorization": token})
			bad := got.status != http.StatusOK
			if c == wrong && !strings.Contains(got.body, `"reason":"invalid"`) || c == off.Code && bad {
				t.Fatalf("redeem with %q: %d %s", c[:1]+"...", got.status, got.body)
			}
			for _, secret := range []string{c, strings.ToLower(c), "synthetic-token", token} {
				if strings.Contains(got.body, secret) {
					t.Fatalf("the reply holds a secret: %s", got.body)
				}
				if strings.Contains(logs.String(), secret) {
					t.Fatalf("the log holds a secret: %s", logs.String())
				}
			}
			for k, v := range got.header {
				if strings.Contains(strings.Join(v, ","), "synthetic-token") || strings.EqualFold(k, "Authorization") {
					t.Fatalf("header %s echoes the token", k)
				}
			}
		}
		// A body that fails to decode must not echo its code field either.
		got := r.do(http.MethodPost, base, `{"phase":"enroll","version":1,"code":"ZZZZZZZZZY","bogus":1}`, nil)
		if got.status != http.StatusBadRequest || strings.Contains(got.body, "ZZZZZZZZZY") || strings.Contains(logs.String(), "ZZZZZZZZZY") {
			t.Fatalf("undecodable body: %d %s", got.status, got.body)
		}
	})

	t.Run("graceful close lets an in-flight redemption finish", enrollGracefulClose)
}

// blockingApprover holds the decision until release is closed.
type blockingApprover struct {
	entered, release chan struct{}
	once             sync.Once
	dec              Decision
}

func (b *blockingApprover) Kind() string { return "policy:block" }

func (b *blockingApprover) Approve(ctx context.Context, _ ApprovalRequest) (Decision, error) {
	b.once.Do(func() { close(b.entered) })
	select {
	case <-b.release:
		return b.dec, nil
	case <-ctx.Done():
		return Decision{}, ctx.Err()
	}
}

func enrollGracefulClose(t *testing.T) {
	r := newRig(t, nil)
	ap := &blockingApprover{entered: make(chan struct{}), release: make(chan struct{}), dec: Decision{Approve: true, Reason: "late yes", Trust: DefaultTrust()}}
	off := r.offer(OfferOptions{Approver: ap})
	real := r.serveReal()
	addr := real.Srv.Addr().String()
	done := make(chan error, 1)
	go func() {
		_, err := r.join(real.URL, off.Code, off.RootFingerprint, "node-g")
		done <- err
	}()
	select {
	case <-ap.entered:
	case <-time.After(60 * time.Second):
		t.Fatal("the redemption never reached the approver")
	}
	real.Srv.Close()
	eventually(t, time.Second, "the listener stops accepting", func() bool { return !isOpen(addr) })
	select {
	case err := <-done:
		t.Fatalf("the in-flight redemption ended before the approver answered: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(ap.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the in-flight redemption failed after Close: %v", err)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("the in-flight redemption never finished")
	}
	select {
	case <-real.Srv.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the server did not stop")
	}
}
