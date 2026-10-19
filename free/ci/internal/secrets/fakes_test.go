package secrets

// Shared test doubles: a fake `nself` Runner that speaks contract:cli.secret-resolve v1, and a
// recording redaction registry. Every value used here is an obviously fake synthetic string.

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
)

const planted = "planted-fake-value-7b3e91c4"

type fakeNself struct {
	mu     sync.Mutex
	calls  []Call
	hasCmd bool
	store  map[string]string // ref -> value
	errAll string            // answer every resolve with this CLI error code
}

func newFake(store map[string]string) *fakeNself { return &fakeNself{hasCmd: true, store: store} }

func (f *fakeNself) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

func (f *fakeNself) resolveCalls() int {
	n := 0
	for _, c := range f.Calls() {
		if !strings.Contains(strings.Join(c.Argv, " "), "--help") {
			n++
		}
	}
	return n
}

func errEnvelope(code string) []byte {
	b, _ := json.Marshal(map[string]any{"schema_version": "1", "command": "config secrets resolve", "error": map[string]string{"code": code, "message": "fixed text"}})
	return b
}

func (f *fakeNself) Run(_ context.Context, c Call) (Output, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Argv: append([]string(nil), c.Argv...), Env: append([]string(nil), c.Env...), Timeout: c.Timeout})
	if len(c.Argv) > 0 && c.Argv[len(c.Argv)-1] == "--help" {
		if f.hasCmd {
			return Output{Bytes: []byte("usage: resolve <ref>...\n")}, nil
		}
		return Output{Bytes: []byte("unknown command\n"), ExitCode: 1}, nil
	}
	if f.errAll != "" {
		return Output{Bytes: errEnvelope(f.errAll), ExitCode: 1}, nil
	}
	var attempt string
	var refs []string
	for i, a := range c.Argv {
		if a == "--for-attempt" {
			attempt = c.Argv[i+1]
		}
		if a == "--json" {
			refs = c.Argv[i+1:]
		}
	}
	type val struct {
		Ref       string  `json:"ref"`
		EnvName   string  `json:"env_name"`
		Value     string  `json:"value"`
		Source    string  `json:"source"`
		ExpiresAt *string `json:"expires_at"`
	}
	var vals []val
	for _, r := range refs {
		v, ok := f.store[r]
		if !ok {
			return Output{Bytes: errEnvelope("E541"), ExitCode: 1}, nil
		}
		parsed, _ := Parse(r)
		vals = append(vals, val{Ref: r, EnvName: parsed.Name, Value: v, Source: "age:ci.age"})
	}
	b, _ := json.Marshal(map[string]any{"schema_version": "1", "command": "config secrets resolve",
		"data": map[string]any{"schema": "cli.secret-resolve/v1", "attempt_id": attempt, "values": vals}})
	return Output{Bytes: b}, nil
}

// recorder is a Registrar that remembers what it was given and can refuse.
type recorder struct {
	mu     sync.Mutex
	values []string
	err    error
}

func (r *recorder) Register(v []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	if len(v) < 6 {
		return &Error{Code: "E673", Message: "value is shorter than the 6-byte minimum"}
	}
	r.values = append(r.values, string(v))
	return nil
}

func (r *recorder) count() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.values) }
