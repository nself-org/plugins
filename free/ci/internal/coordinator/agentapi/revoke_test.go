package agentapi

// Purpose: TestRevokeClosesStreams: a revoke while an agent long-polls, streams or listens on the control
//          stream ends that request within 1 s, over the real listener, by both mechanisms (the deny
//          list through the request guard, and CloseNode, which is what the lifecycle session closer calls).
// Inputs:  the rig. Outputs: pass or fail.
// Constraints: the bound is 1 s plus a margin; no mocks of the guard or the registry.

import (
	"io"
	"net/http"
	"testing"
	"time"
)

func TestRevokeClosesStreams(t *testing.T) {
	routes := []struct {
		name, method, path string
		body               bool
	}{
		{"leases long-poll", http.MethodGet, "/v1/agent/leases", false},
		{"control stream", http.MethodGet, "/v1/agent/control", false},
		{"upstream stream", http.MethodPost, "/v1/agent/stream", true},
	}
	closers := []struct {
		name string
		do   func(r *rig, a *agent)
	}{
		{"deny list", func(r *rig, a *agent) {
			if _, err := r.ca.Revoke(t.Context(), "cli:test", a.id, "stolen"); err != nil {
				t.Fatal(err)
			}
		}},
		{"CloseNode", func(r *rig, a *agent) { r.srv.CloseNode(a.id) }},
	}
	for _, cl := range closers {
		for _, rt := range routes {
			t.Run(cl.name+"/"+rt.name, func(t *testing.T) {
				r := newRig(t, nil)
				a := r.newAgent("n-1")
				a.connect()
				var body io.Reader
				var pw *io.PipeWriter
				if rt.body {
					var pr *io.PipeReader
					pr, pw = io.Pipe()
					body = pr
					defer pw.Close()
				}
				rq := a.req(rt.method, rt.path, body, map[string]string{HeaderWait: "30"})
				type res struct {
					err error
					at  time.Time
				}
				done := make(chan res, 1)
				go func() {
					resp, err := a.cl.Do(rq)
					if err == nil {
						_, err = io.Copy(io.Discard, resp.Body) // the stream is over when the body ends
						resp.Body.Close()
					}
					done <- res{err, time.Now()}
				}()
				eventually(t, 2*time.Second, "the request to be live", func() bool { return len(r.srv.NodesConnected()) == 1 && liveRequests(r.srv, a.id) > 0 })
				start := time.Now()
				cl.do(r, a)
				select {
				case got := <-done:
					if d := got.at.Sub(start); d > time.Second {
						t.Fatalf("request ended %v after the revoke, want < 1 s (err %v)", d, got.err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("request still open 3 s after the revoke")
				}
			})
		}
	}
}

func liveRequests(s *Server, node string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ns := s.nodes[node]; ns != nil && ns.sess != nil {
		return len(ns.sess.cancels)
	}
	return 0
}
