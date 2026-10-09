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
	"regexp"
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
	req.HeadersWithoutAuth = redactedHeaders(req.HeadersWithoutAuth)
	resp.Headers = redactedHeaders(resp.Headers)
	req.Path = redactCredential(req.Path)
	req.Query = redactCredential(req.Query)
	resp.Body = redactBody(resp.Body)
	b, err := json.MarshalIndent(Exchange{req, resp}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".json"), append(b, '\n'), 0600)
}

var credentialPattern = regexp.MustCompile(`(?i)(?:bearer[[:space:]]+[a-z0-9._~+/-]+=*|[a-z0-9]+[-_](?:secret|token|key|password|credential)[-_][a-z0-9._-]+|(?:ghp|gho|ghu|ghs|ghr|glpat)_[a-z0-9_-]{12,}|[a-z0-9_-]{12,}\.[a-z0-9_-]{12,}\.[a-z0-9_-]{12,})`)

func redactCredential(value string) string {
	return credentialPattern.ReplaceAllString(value, "REDACTED")
}

func redactKnownSecrets(value string, secrets [][]byte) (string, bool) {
	leaked := false
	for _, secret := range secrets {
		if len(secret) > 0 && strings.Contains(value, string(secret)) {
			value = strings.ReplaceAll(value, string(secret), "REDACTED")
			leaked = true
		}
	}
	return value, leaked
}

func redactedHeaders(headers http.Header) http.Header {
	out := headers.Clone()
	for key, values := range out {
		if sensitive(key) {
			out[key] = []string{"REDACTED"}
			continue
		}
		for i, value := range values {
			values[i] = redactCredential(value)
		}
	}
	return out
}

func redactBody(body string) string {
	var value any
	if json.Unmarshal([]byte(body), &value) == nil {
		redactJSON(&value)
		if encoded, err := json.Marshal(value); err == nil {
			return string(encoded)
		}
	}
	return redactCredential(body)
}

func redactJSON(value *any) {
	switch v := (*value).(type) {
	case map[string]any:
		for key, child := range v {
			if sensitive(key) {
				v[key] = "REDACTED"
				continue
			}
			redactJSON(&child)
			v[key] = child
		}
	case []any:
		for i := range v {
			redactJSON(&v[i])
		}
	case string:
		*value = redactCredential(v)
	}
}
func sensitive(key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(k, "auth") || strings.Contains(k, "token") || strings.Contains(k, "key") || strings.Contains(k, "secret")
}
