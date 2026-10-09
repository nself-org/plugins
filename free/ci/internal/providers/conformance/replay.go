package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Request struct {
	Method             string      `json:"method"`
	Path               string      `json:"path"`
	Query              string      `json:"query"`
	HeadersWithoutAuth http.Header `json:"headers_without_auth"`
	BodySHA256         string      `json:"body_sha256"`
}
type Response struct {
	Status  int         `json:"status"`
	Headers http.Header `json:"headers"`
	Body    string      `json:"body"`
}
type Exchange struct {
	Request  Request  `json:"request"`
	Response Response `json:"response"`
}

// Replay serves ordered, recorded exchanges and retains the requests for inspection.
type Replay struct {
	*httptest.Server
	mu        sync.Mutex
	exchanges []Exchange
	requests  []*http.Request
	next      int
	problems  []string
}

func NewReplay(fixtures string) (*Replay, error) {
	r := &Replay{}
	if fixtures != "" {
		files, err := filepath.Glob(filepath.Join(fixtures, "*.json"))
		if err != nil {
			return nil, err
		}
		sort.Strings(files)
		for _, name := range files {
			b, err := os.ReadFile(name)
			if err != nil {
				return nil, err
			}
			var x Exchange
			if err := json.Unmarshal(b, &x); err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			r.exchanges = append(r.exchanges, x)
		}
	}
	r.Server = httptest.NewServer(http.HandlerFunc(r.serve))
	return r, nil
}
func (r *Replay) serve(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req.Clone(req.Context()))
	if r.next >= len(r.exchanges) {
		r.problems = append(r.problems, "unexpected request")
		http.Error(w, "unexpected request", http.StatusBadRequest)
		return
	}
	x := r.exchanges[r.next]
	r.next++
	if req.Method != x.Request.Method || req.URL.Path != x.Request.Path || req.URL.RawQuery != x.Request.Query {
		r.problems = append(r.problems, "request mismatch")
	}
	b, _ := io.ReadAll(req.Body)
	sum := sha256.Sum256(b)
	if x.Request.BodySHA256 != "" && hex.EncodeToString(sum[:]) != x.Request.BodySHA256 {
		r.problems = append(r.problems, "body mismatch")
	}
	for key, want := range x.Request.HeadersWithoutAuth {
		if !sensitive(key) && req.Header.Get(key) != strings.Join(want, ", ") {
			r.problems = append(r.problems, "header mismatch: "+key)
		}
	}
	for key, values := range x.Response.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	if x.Response.Status == 0 {
		x.Response.Status = 200
	}
	w.WriteHeader(x.Response.Status)
	_, _ = io.WriteString(w, x.Response.Body)
}
func (r *Replay) Requests() []*http.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*http.Request(nil), r.requests...)
}
func (r *Replay) Problems() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]string(nil), r.problems...)
	if r.next != len(r.exchanges) {
		out = append(out, "unused exchanges")
	}
	return out
}

// Record saves one exchange with credential-bearing headers redacted.
func Record(dir, name string, req Request, resp Response) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for key := range req.HeadersWithoutAuth {
		if sensitive(key) {
			req.HeadersWithoutAuth[key] = []string{"REDACTED"}
		}
	}
	for key := range resp.Headers {
		if sensitive(key) {
			resp.Headers[key] = []string{"REDACTED"}
		}
	}
	b, err := json.MarshalIndent(Exchange{req, resp}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".json"), append(b, '\n'), 0600)
}
func sensitive(key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(k, "auth") || strings.Contains(k, "token") || strings.Contains(k, "key") || strings.Contains(k, "secret")
}
