package secrets

// Purpose: the only path to secret values: `nself config secrets resolve` (contract:cli.secret-resolve
// v1), reached through a Runner (the exec funnel in production, a fake `nself` in tests).
// Inputs:  an attempt id and parsed refs.
// Outputs: one Value per ref, in input order, each as a byte slice the caller must Zero.
// Constraints: availability is probed once per Resolver with `--help` (E675 when the command is
// absent); every call has a 30 s timeout; the output buffer is zeroed after parsing; stdout is never
// logged and no error text contains output, only the CLI's error code (the CLI's messages carry no
// value, but this package does not depend on that).

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// CallTimeout bounds one call to the cli.
const CallTimeout = 30 * time.Second

// maxOutput bounds what a call may print before it is refused.
const maxOutput = 4 << 20

// Call is one invocation of the cli.
type Call struct {
	Argv []string
	// Env is added to the runner's base environment (SECRETS_AGE_KEY_PATH on an agent node).
	Env     []string
	Timeout time.Duration
}

// Output is what a call printed. Bytes are stdout and stderr in arrival order.
type Output struct {
	Bytes    []byte
	ExitCode int
}

// Runner runs the cli. A non-nil error means the program could not be run at all.
type Runner interface {
	Run(ctx context.Context, c Call) (Output, error)
}

// Value is one resolved secret. Bytes is owned by the holder, who zeroes it.
type Value struct {
	Ref    Ref
	Bytes  []byte
	Source string
}

// ValueResolver is what a Source needs from the cli side (a Resolver, or a fake in tests).
type ValueResolver interface {
	Probe(ctx context.Context) error
	Resolve(ctx context.Context, attempt string, refs []Ref) ([]Value, error)
}

// Resolver implements ValueResolver over a Runner.
type Resolver struct {
	Runner Runner
	// Binary is the cli program; "" means "nself".
	Binary string
	// Project is the project root passed as --project (absolute).
	Project string
	// AgeKeyPath, when set, is exported as SECRETS_AGE_KEY_PATH: the node's own age identity.
	AgeKeyPath string
	// Env is added to every cli call. The signer sets NSELF_V15=1 here: the funnel strips the
	// caller's environment, and released cli 1.4.x has `config secrets resolve` only with it (D-1236).
	Env []string
	// Shared, when set, is shared by every Resolver of the process, so availability is asked once per
	// process (P7-CI D26); nil keeps the result inside this Resolver.
	Shared *ProbeState

	own ProbeState
}

// ProbeState remembers the outcome of the `--help` probe.
type ProbeState struct {
	once sync.Once
	err  error
}

func (r *Resolver) binary() string {
	if r.Binary == "" {
		return "nself"
	}
	return r.Binary
}

func (r *Resolver) env() []string {
	out := append([]string(nil), r.Env...)
	if r.AgeKeyPath != "" {
		out = append(out, "SECRETS_AGE_KEY_PATH="+r.AgeKeyPath)
	}
	return out
}

// Probe checks once that the cli has the resolve command. Absence is E675.
func (r *Resolver) Probe(ctx context.Context) error {
	st := r.Shared
	if st == nil {
		st = &r.own
	}
	st.once.Do(func() {
		// detached from the first caller's ctx: a cancelled first attempt must not poison every later one
		pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), CallTimeout)
		defer cancel()
		out, err := r.Runner.Run(pctx, Call{Argv: []string{r.binary(), "config", "secrets", "resolve", "--help"}, Env: r.env(), Timeout: CallTimeout})
		zero(out.Bytes)
		switch {
		case err != nil:
			st.err = errUnavailable("the nself cli could not be run to probe `config secrets resolve`")
		case out.ExitCode != 0:
			st.err = errUnavailable("this nself cli has no `config secrets resolve` (exit %d)", out.ExitCode)
		}
	})
	return st.err
}

// Resolve runs `nself config secrets resolve --project <root> --for-attempt <id> --json <refs>`.
func (r *Resolver) Resolve(ctx context.Context, attempt string, refs []Ref) ([]Value, error) {
	argv := []string{r.binary(), "config", "secrets", "resolve", "--project", r.Project, "--for-attempt", attempt, "--json"}
	for _, ref := range refs {
		argv = append(argv, ref.Raw)
	}
	out, err := r.Runner.Run(ctx, Call{Argv: argv, Env: r.env(), Timeout: CallTimeout})
	defer zero(out.Bytes)
	if err != nil {
		return nil, errResolve("the nself cli could not be run")
	}
	return parseEnvelope(out, attempt, refs)
}

type envelope struct {
	Data  *envelopeData `json:"data"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}
type envelopeData struct {
	Schema    string `json:"schema"`
	AttemptID string `json:"attempt_id"`
	Values    []struct {
		Ref     string          `json:"ref"`
		EnvName string          `json:"env_name"`
		Value   json.RawMessage `json:"value"`
		Source  string          `json:"source"`
	} `json:"values"`
}

// parseEnvelope validates the v1 document against the request and copies each value into its own buffer.
func parseEnvelope(out Output, attempt string, refs []Ref) ([]Value, error) {
	start := bytes.IndexByte(out.Bytes, '{')
	if start < 0 || len(out.Bytes) > maxOutput {
		return nil, errResolve("the resolver printed no v1 envelope (exit %d)", out.ExitCode)
	}
	var env envelope
	defer func() { // every raw value copy the decoder made is overwritten on every return path
		if env.Data != nil {
			for _, v := range env.Data.Values {
				zero(v.Value)
			}
		}
	}()
	if err := json.NewDecoder(bytes.NewReader(out.Bytes[start:])).Decode(&env); err != nil {
		return nil, errResolve("the resolver output was not a v1 envelope (exit %d)", out.ExitCode)
	}
	if env.Error != nil {
		e := newErr("E676", "the resolver refused the request (%s)", safeCode(env.Error.Code))
		e.Cause = safeCode(env.Error.Code)
		return nil, e
	}
	d := env.Data
	if out.ExitCode != 0 || d == nil || d.Schema != "cli.secret-resolve/v1" || d.AttemptID != attempt || len(d.Values) != len(refs) {
		return nil, errResolve("the resolver answer does not match the request (exit %d)", out.ExitCode)
	}
	vals := make([]Value, 0, len(refs))
	fail := func() ([]Value, error) {
		for _, v := range vals {
			zero(v.Bytes)
		}
		return nil, errResolve("the resolver answer does not match the request")
	}
	for i, v := range d.Values {
		var s string
		if v.Ref != refs[i].Raw || v.EnvName != refs[i].Name || json.Unmarshal(v.Value, &s) != nil || s == "" {
			zero(v.Value)
			return fail()
		}
		zero(v.Value)
		vals = append(vals, Value{Ref: refs[i], Bytes: []byte(s), Source: boundSource(v.Source)})
	}
	return vals, nil
}

// safeCode keeps an E-code and drops anything else the document put in the field.
func safeCode(c string) string {
	if len(c) >= 2 && len(c) <= 6 && c[0] == 'E' && strings.Trim(c[1:], "0123456789") == "" {
		return c
	}
	return "E???"
}

// boundSource keeps the `age:<file>` label short and printable (evidence shows it).
func boundSource(s string) string {
	if len(s) > 64 || strings.ContainsAny(s, "\r\n\x00") {
		return "age:?"
	}
	return s
}

// zero overwrites b.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
