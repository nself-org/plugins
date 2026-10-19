package coordinator

// Purpose: tests of the Ingress registry (P7-NODE-10, HOST delta 5, review R1).
// Inputs:  fake Ingress values, request bodies up to 26 MiB.
// Outputs: pass or fail.
// Constraints: the registry is process-global, so each test registers a uniquely named fake and removes
//          it again with unregister.

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type fakeIngress struct {
	name    string
	secrets bool
	accept  atomic.Int32
	deliver Delivery
	err     error
}

func (f *fakeIngress) Name() string               { return f.name }
func (f *fakeIngress) Match(r *http.Request) bool { return r.Header.Get("X-Fake-Provider") == f.name }
func (f *fakeIngress) HasSecrets() bool           { return f.secrets }
func (f *fakeIngress) Accept(context.Context, *http.Request, []byte) (Delivery, error) {
	f.accept.Add(1)
	return f.deliver, f.err
}

func register(t *testing.T, f *fakeIngress) {
	t.Helper()
	if err := RegisterIngress(f); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ingresses.Lock()
		defer ingresses.Unlock()
		for i, have := range ingresses.list {
			if have == Ingress(f) {
				ingresses.list = append(ingresses.list[:i], ingresses.list[i+1:]...)
				return
			}
		}
	})
}

func fakePost(r *rig, provider string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Fake-Provider", provider)
	w := httptest.NewRecorder()
	r.c.Handler(true).ServeHTTP(w, req)
	return w
}

func TestIngressReceivesOnlyMatchingRequests(t *testing.T) {
	f := &fakeIngress{name: "zz-fake-a", deliver: Delivery{Source: "fake", DeliveryID: "f-1", Project: "o/r", Revision: testSHA}}
	register(t, f)
	r := newRig(t, nil)
	r.post("/webhook", "push", "g1", pushBody(otherSHA)) // a GitHub request: no match
	if f.accept.Load() != 0 {
		t.Fatal("Accept saw a request it did not match")
	}
	if w := fakePost(r, "zz-fake-a", []byte(`{}`)); w.Code != 202 || f.accept.Load() != 1 {
		t.Fatalf("matched request = %d %s accept=%d", w.Code, w.Body, f.accept.Load())
	}
}

func TestIngressMatchedRequestSkipsSharedSecret(t *testing.T) {
	f := &fakeIngress{name: "zz-fake-b", deliver: Delivery{Source: "fake", DeliveryID: "f-2", Project: "o/r", Revision: testSHA}}
	register(t, f)
	r := newRig(t, nil) // the v1.4 secret is set, and the request carries no signature
	if w := fakePost(r, "zz-fake-b", []byte(`{}`)); w.Code != 202 {
		t.Fatalf("a matched request was checked against the shared secret: %d %s", w.Code, w.Body)
	}
}

func TestIngressBodyCapIs413BeforeAccept(t *testing.T) {
	f := &fakeIngress{name: "zz-fake-c"}
	register(t, f)
	r := newRig(t, nil)
	if w := fakePost(r, "zz-fake-c", make([]byte, 26<<20)); w.Code != 413 || f.accept.Load() != 0 {
		t.Fatalf("26 MiB body = %d, accept=%d; want 413 and no Accept", w.Code, f.accept.Load())
	}
}

func TestIngressAcceptErrorIs403AndStoresNothing(t *testing.T) {
	f := &fakeIngress{name: "zz-fake-d", err: errors.New("bad signature with secret-value-123")}
	register(t, f)
	r := newRig(t, nil)
	w := fakePost(r, "zz-fake-d", []byte(`{}`))
	if w.Code != 403 || queued(t, r.st) != 0 {
		t.Fatalf("Accept error = %d, queued=%d", w.Code, queued(t, r.st))
	}
	if bytes.Contains(w.Body.Bytes(), []byte("secret-value-123")) || bytes.Contains([]byte(r.logs.String()), []byte("secret-value-123")) {
		t.Fatal("the Accept error text reached the answer or the log")
	}
}

func TestIngressSecretsLetServeStartWithoutSecret(t *testing.T) {
	register(t, &fakeIngress{name: "zz-fake-e", secrets: true})
	r := newRig(t, func(c *Config) { c.Secret = "" })
	defer r.start(t)() // per-repo secrets, no --secret: start succeeds
	if err := RegisterIngress(&fakeIngress{name: "zz-fake-e"}); ReasonOf(err) != ReasonDuplicateName {
		t.Fatalf("a repeated ingress name = %v", err)
	}
}
