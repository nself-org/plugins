package rest

// Purpose: tests of Client.DoRequest (P7-HOST-17 lead ruling: the poll reconcilers read lists with
// If-None-Match so an unchanged list costs no primary quota, which needs the request header and the
// response ETag).
// Inputs:  a local TLS server and the synthetic values below.
// Outputs: pass or fail.
// Constraints: no real network; the values carry no secret.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestDoRequestConditional proves the ETag is returned, If-None-Match is sent only when asked for, a 304
// to a conditional request is a success with nothing decoded, and a 304 to an unconditional request is
// still an error.
func TestDoRequestConditional(t *testing.T) {
	var gotMatch string
	status := http.StatusOK
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMatch = r.Header.Get("If-None-Match")
		w.Header().Set("ETag", `W/"abc"`)
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"n":7}`))
		}
	}))
	defer s.Close()
	c := testClient(s)
	var out struct{ N int }
	resp, err := c.DoRequest(context.Background(), Request{Method: "GET", Path: "/list"}, &out)
	if err != nil || resp.ETag != `W/"abc"` || out.N != 7 || gotMatch != "" {
		t.Fatalf("plain request: resp=%+v err=%v out=%+v match=%q", resp, err, out, gotMatch)
	}
	status = http.StatusNotModified
	out.N = 0
	resp, err = c.DoRequest(context.Background(), Request{Method: "GET", Path: "/list", IfNoneMatch: `W/"abc"`}, &out)
	if err != nil || resp.Status != 304 || out.N != 0 || gotMatch != `W/"abc"` {
		t.Fatalf("conditional request: resp=%+v err=%v out=%+v match=%q", resp, err, out, gotMatch)
	}
	if _, err = c.DoRequest(context.Background(), Request{Method: "GET", Path: "/list"}, &out); err == nil {
		t.Fatal("a 304 to an unconditional request must stay an error")
	}
}
