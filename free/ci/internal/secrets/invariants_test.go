package secrets

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

var refForClass = map[model.SecretClass]string{
	"project": "project/KEY_A", "environment": "environment/stg/KEY_B", "team": "team/KEY_C",
	"release": "release/KEY_D", "deploy": "deploy/stg/KEY_E",
}

func refsOf(classes []model.SecretClass) []string {
	var out []string
	for _, c := range classes {
		if r, ok := refForClass[c]; ok {
			out = append(out, r)
		}
	}
	return out
}

// stubResolver counts calls and returns whatever fn says.
type stubResolver struct {
	probes, resolves int
	fn               func(refs []Ref) ([]Value, error)
}

func (s *stubResolver) Probe(context.Context) error { s.probes++; return nil }
func (s *stubResolver) Resolve(_ context.Context, _ string, refs []Ref) ([]Value, error) {
	s.resolves++
	return s.fn(refs)
}

func worldSource(w invariants.World, res ValueResolver, reg Registrar) *Source {
	mode, eligible := ModePersonalLocal, false
	if w.Ownership != "operator" {
		mode, eligible = ModeAgentNode, true
	}
	return New(Config{Mode: mode, Attempt: "att-w", Resolver: res, Registry: reg,
		RedactorInstalled: func() bool { return true }, NodeID: "n1", NodeEligible: eligible, AgeKeyPath: "/node/age.key",
		Facts: Facts{
			Job:    trust.JobFacts{Trust: w.Trust, PersonalOwnRun: true, Mode: trust.DecodeCoordinatorMode(w.Mode)},
			Runner: trust.RunnerFacts{Ownership: w.Ownership, PersonalOwnNode: w.Ownership == "operator", Dedicated: true, Isolation: w.Isolation, UIDSeparation: true},
		}})
}

func TestInvariant_I05(t *testing.T) {
	invariants.Check(t, func(w invariants.World) error {
		raw := refsOf(w.Secrets)
		if w.Trust != "untrusted" || len(raw) == 0 {
			return nil
		}
		res := &stubResolver{fn: func([]Ref) ([]Value, error) { return nil, fmt.Errorf("must not be called") }}
		s := worldSource(w, res, &recorder{})
		got, err := s.Resolve(context.Background(), raw)
		var e *Error
		if got != nil || CodeOf(err) != "E677" || !asErr(err, &e) || !has(e.Reasons, string(trust.SecretsUntrusted)) {
			return fmt.Errorf("untrusted attempt not refused: %v %v", got, err)
		}
		if res.probes+res.resolves != 0 || s.Held() != 0 {
			return fmt.Errorf("untrusted attempt reached the resolver")
		}
		refs, perr := ParseAll(raw)
		if perr != nil || !hasReason(Check(refs, s.cfg.Facts), trust.SecretsUntrusted) {
			return fmt.Errorf("Check did not report secrets.untrusted")
		}
		d := trust.Declaration{Accepts: []model.TrustClass{"owner", "internal", "collaborator"}, Isolation: w.Isolation, SecretClasses: w.Secrets, Ownership: w.Ownership,
			UIDSeparation: true, NoLANRoute: true, CoordinatorMode: trust.DecodeCoordinatorMode(w.Mode)}
		job := trust.JobFacts{Trust: w.Trust, SecretClasses: w.Secrets, Mode: trust.DecodeCoordinatorMode(w.Mode)}
		if !hasReason(trust.Admissible(job, d, trust.Effective{}), trust.SecretsUntrusted) {
			return fmt.Errorf("Admissible did not report secrets.untrusted")
		}
		return nil
	})
}

func hasReason(rs []trust.Reason, want trust.Reason) bool {
	for _, r := range rs {
		if r == want {
			return true
		}
	}
	return false
}

// TestInvariant_I06 is the store half: whatever the package exposes after an attempt (errors, evidence
// sources, audit events, lease refs) holds no value, and every buffer is zero after pass, fail, cancel
// and lost.
func TestInvariant_I06(t *testing.T) {
	n := 0
	invariants.Check(t, func(w invariants.World) error {
		n++
		val := fmt.Sprintf("fake-i06-%08x-value", n*2654435761)
		outcome := n % 4 // 0 pass, 1 fail (Register), 2 cancel, 3 lost
		var held [][]byte
		res := &stubResolver{fn: func(refs []Ref) ([]Value, error) {
			switch outcome {
			case 2:
				return nil, errResolve("the attempt was canceled")
			case 3:
				return nil, errResolve("the node was lost")
			}
			out := make([]Value, len(refs))
			for i, r := range refs {
				b := []byte(val)
				held = append(held, b)
				out[i] = Value{Ref: r, Bytes: b, Source: "age:ci.age"}
			}
			return out, nil
		}}
		reg := &recorder{}
		if outcome == 1 {
			reg.err = &Error{Code: "E673", Message: "value too short"}
		}
		w.Trust = "internal"
		w.Ownership = "operator"
		s := worldSource(w, res, reg)
		raw := []string{"project/KEY_A"}
		var seen []string
		got, err := s.Resolve(context.Background(), raw)
		if err != nil {
			seen = append(seen, err.Error())
		}
		for k, v := range s.Sources() {
			seen = append(seen, k, v)
		}
		refs, _ := s.Refs(raw)
		for _, r := range refs {
			seen = append(seen, r.Ref, string(r.Class))
		}
		if (outcome == 0) != (err == nil) {
			return fmt.Errorf("outcome %d gave err=%v", outcome, err)
		}
		if outcome == 0 && got["KEY_A"] != val {
			return fmt.Errorf("pass outcome did not deliver the value")
		}
		for _, s := range seen {
			if strings.Contains(s, val) {
				return fmt.Errorf("a value reached an exposed string")
			}
		}
		s.Zero()
		for _, b := range held {
			if len(bytes.Trim(b, "\x00")) != 0 {
				return fmt.Errorf("buffer not zeroed after outcome %d", outcome)
			}
		}
		if s.Held() != 0 {
			return fmt.Errorf("buffers still held")
		}
		return nil
	})
}

// TestStoreFileHoldsNoValue records everything the package exposes into a real SQLite store after a run
// with a planted value, then greps the database and its WAL.
func TestStoreFileHoldsNoValue(t *testing.T) {
	f := newFake(map[string]string{"project/FOO": planted, "release/ANDROID_KEY_PASSWORD": planted + "-2"})
	s, _ := newSrc(f, func(c *Config) {
		c.Audit = func(e Event) error { return nil }
		c.Legacy = &LegacyEnv{Getenv: func(string) string { return planted + "-legacy" }}
	})
	if _, err := s.Resolve(context.Background(), []string{"project/FOO", "release/ANDROID_KEY_PASSWORD"}); err != nil {
		t.Fatal(err)
	}
	_, badErr := s.Resolve(context.Background(), []string{"project/FOO", "bad ref"})
	dir := t.TempDir()
	path := filepath.Join(dir, "ci.db")
	st, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	refs, _ := s.Refs([]string{"project/FOO"})
	for k, v := range s.Sources() {
		if err = st.AppendAudit(ctx, "secrets", "resolved", k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err = st.AppendAudit(ctx, "secrets", "refused", fmt.Sprint(refs), badErr.Error()); err != nil {
		t.Fatal(err)
	}
	s.Zero()
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	read := 0
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		b, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			t.Fatal(rerr)
		}
		read++
		if bytes.Contains(b, []byte("planted-fake-value")) {
			t.Fatalf("a planted value is in %s", e.Name())
		}
	}
	if read == 0 {
		t.Fatal("no store files were read")
	}
}
