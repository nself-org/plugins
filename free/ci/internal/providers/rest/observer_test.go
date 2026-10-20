package rest

// Purpose: the process-wide observer (P7-HOST-11): it sees every client's outcome, also one with an
//          observer of its own, and a request its caller cancelled is not reported.
// Inputs:  a TLS fake server and clients with and without SetObserver.
// Outputs: pass or fail.
// Constraints: no live network; the default observer is restored in t.Cleanup.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestDefaultObserverSeesEveryClient(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{}")) }))
	t.Cleanup(srv.Close)
	var mu sync.Mutex
	var seen []string
	SetDefaultObserver(func(o Outcome) { mu.Lock(); seen = append(seen, "default:"+o.Provider); mu.Unlock() })
	t.Cleanup(func() { SetDefaultObserver(nil) })
	mk := func(provider string) *Client {
		return &Client{BaseURL: srv.URL, AllowPrivateBase: true, Provider: provider, TLSConfig: srv.Client().Transport.(*http.Transport).TLSClientConfig}
	}
	own := mk("own")
	own.SetObserver(func(o Outcome) { mu.Lock(); seen = append(seen, "own:"+o.Provider); mu.Unlock() })
	for _, c := range []*Client{mk("plain"), own} {
		if _, err := c.Do(context.Background(), "GET", "/x", nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"default:plain", "own:own", "default:own"}
	if len(seen) != len(want) {
		t.Fatalf("seen %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("seen %v, want %v", seen, want)
		}
	}
}

func TestCancelledRequestIsNotReported(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	t.Cleanup(srv.Close)
	var mu sync.Mutex
	n := 0
	SetDefaultObserver(func(Outcome) { mu.Lock(); n++; mu.Unlock() })
	t.Cleanup(func() { SetDefaultObserver(nil) })
	c := &Client{BaseURL: srv.URL, AllowPrivateBase: true, Provider: "p", TLSConfig: srv.Client().Transport.(*http.Transport).TLSClientConfig}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	if _, err := c.Do(ctx, "GET", "/x", nil, nil, nil); err == nil {
		t.Fatal("the cancelled request must fail")
	}
	mu.Lock()
	defer mu.Unlock()
	if n != 0 {
		t.Fatalf("a cancelled request reached the observer %d times: it says nothing about the provider", n)
	}
}
