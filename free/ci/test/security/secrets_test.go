//go:build security

package security

// Purpose: secrets (I05, I06; TM-01, TM-16, TM-29, TM-30). An untrusted attempt never reaches the
// resolver whatever it asks for and on whatever node; a trusted attempt on an eligible node gets its
// value, which is registered with the redactor before it is handed out; the value never appears in
// the output stream in any registered encoding however the stream is chunked; a container receives
// names only on its argv; a hosted runner is never offered a secret class.
// Inputs:  a stub resolver that fails the test if an untrusted attempt reaches it; synthetic values.
// Outputs: pass or fail.
// Constraints: values are obviously synthetic; the Source, Check, Redactor and Admissible under test
//          are the production ones.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// secretValue is a synthetic secret with characters that encode differently.
const secretValue = "synthetic/Val+ue=secret&x 42"

var refForClass = map[model.SecretClass]string{
	"project": "project/KEY_A", "environment": "environment/stg/KEY_B", "team": "team/KEY_C",
	"release": "release/KEY_D", "deploy": "deploy/stg/KEY_E",
}

type stubResolver struct {
	probes, resolves int
	value            string
}

func (s *stubResolver) Probe(context.Context) error { s.probes++; return nil }
func (s *stubResolver) Resolve(_ context.Context, _ string, refs []secrets.Ref) ([]secrets.Value, error) {
	s.resolves++
	out := make([]secrets.Value, len(refs))
	for i, r := range refs {
		out[i] = secrets.Value{Ref: r, Bytes: []byte(s.value), Source: "age:ci.age"}
	}
	return out, nil
}

// secretSource builds the per-attempt secret source of a job of the given class on an operator node.
func secretSource(class model.TrustClass, mode secrets.Mode, eligible bool, res secrets.ValueResolver, reg secrets.Registrar) *secrets.Source {
	runner := trust.RunnerFacts{Ownership: "operator", PersonalOwnNode: mode == secrets.ModePersonalLocal, Dedicated: true, Isolation: "vm", UIDSeparation: true}
	return secrets.New(secrets.Config{Mode: mode, Attempt: "att-s", Resolver: res, Registry: reg, RedactorInstalled: func() bool { return true },
		NodeID: "n1", NodeEligible: eligible, AgeKeyPath: "/node/age.key",
		Facts: secrets.Facts{Job: trust.JobFacts{Trust: class, PersonalOwnRun: true}, Runner: runner}})
}

// TestUntrustedNeverReceivesSecret is I05 against the production Source: every secret class, on a
// personal node and on an eligible agent node, is refused with E677 secrets.untrusted before the
// resolver is probed.
func TestUntrustedNeverReceivesSecret(t *testing.T) {
	for class, ref := range refForClass {
		for _, mode := range []secrets.Mode{secrets.ModePersonalLocal, secrets.ModeAgentNode} {
			res := &stubResolver{value: secretValue}
			s := secretSource("untrusted", mode, true, res, redact.New())
			got, err := s.Resolve(context.Background(), []string{ref})
			var e *secrets.Error
			if got != nil || !errors.As(err, &e) || e.Code != "E677" || !strings.Contains(strings.Join(e.Reasons, ","), string(trust.SecretsUntrusted)) {
				t.Fatalf("%s via %s: untrusted attempt not refused: %v %v", class, mode, got, err)
			}
			if res.probes+res.resolves != 0 || s.Held() != 0 {
				t.Fatalf("%s via %s: the untrusted attempt reached the resolver (%d probes, %d resolves, %d held)", class, mode, res.probes, res.resolves, s.Held())
			}
		}
	}
	// Control: an internal attempt on the same node gets its value, so the refusals above are the trust rule.
	res := &stubResolver{value: secretValue}
	reg := redact.New()
	s := secretSource("internal", secrets.ModePersonalLocal, false, res, reg)
	got, err := s.Resolve(context.Background(), []string{refForClass["project"]})
	if err != nil || got["KEY_A"] != secretValue {
		t.Fatalf("control: an internal attempt on a personal node was refused: %v %v", got, err)
	}
	s.Zero()
	if s.Held() != 0 {
		t.Fatal("the secret buffers were not zeroed")
	}
}

// TestSecretNeverOnAHostedOrUntrustedRunner: the admission rules never offer a secret class to a hosted
// runner and name the reason for an untrusted job asking for one.
func TestSecretNeverOnAHostedOrUntrustedRunner(t *testing.T) {
	hosted := hostedRunner("hosted1").Decl
	hosted.SecretClasses = []model.SecretClass{"project", "environment", "team", "release", "deploy"}
	policy := repoPolicy(t, "public", "owner")
	for class := range refForClass {
		job := trust.JobFacts{Trust: "owner", SecretClasses: []model.SecretClass{class}, Network: "internet"}
		if got := trust.Admissible(job, hosted, policy); !hasTrustReason(got, trust.SecretsNotInPolicy) {
			t.Errorf("hosted runner offered the %s secret class: %v", class, got)
		}
		untrusted := trust.JobFacts{Trust: "untrusted", SecretClasses: []model.SecretClass{class}, Network: "restricted"}
		if got := trust.Admissible(untrusted, gvisorRunner("gv", 1).Decl, policy); !hasTrustReason(got, trust.SecretsUntrusted) {
			t.Errorf("untrusted %s job not refused: %v", class, got)
		}
	}
}

func hasTrustReason(rs []trust.Reason, want trust.Reason) bool {
	for _, r := range rs {
		if r == want {
			return true
		}
	}
	return false
}

// encodings returns the forms a secret takes when a tool prints it.
func encodings(v string) map[string]string {
	return map[string]string{
		"raw": v, "url": url.QueryEscape(v), "url-path": url.PathEscape(v), "b64": base64.StdEncoding.EncodeToString([]byte(v)),
		"b64url": base64.URLEncoding.EncodeToString([]byte(v)), "hex": hex.EncodeToString([]byte(v)),
	}
}

// TestSecretValueNeverInOutput is I06: a registered value is masked in every encoding, however the
// stream is split, and a too-short value is refused (E673) instead of being half-protected.
func TestSecretValueNeverInOutput(t *testing.T) {
	r := redact.New()
	if err := r.Register([]byte(secretValue)); err != nil {
		t.Fatal(err)
	}
	if err := r.Register([]byte("abc")); err == nil {
		t.Fatal("a 3-byte value was accepted for redaction")
	}
	for name, form := range encodings(secretValue) {
		line := "before " + form + " after\n"
		for chunk := 1; chunk <= len(line); chunk++ {
			var out bytes.Buffer
			w := r.Wrap(&out)
			for i := 0; i < len(line); i += chunk {
				end := min(i+chunk, len(line))
				if _, err := w.Write([]byte(line[i:end])); err != nil {
					t.Fatal(err)
				}
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			for encName, enc := range encodings(secretValue) {
				if strings.Contains(out.String(), enc) {
					t.Fatalf("form %s in chunks of %d leaked the %s encoding: %q", name, chunk, encName, out.String())
				}
			}
			if !strings.Contains(out.String(), redact.Placeholder) {
				t.Fatalf("form %s in chunks of %d: no placeholder in %q", name, chunk, out.String())
			}
		}
	}
	// Control: text that is not the secret is left alone.
	var out bytes.Buffer
	w := r.Wrap(&out)
	_, _ = w.Write([]byte("nothing to hide here\n"))
	_ = w.Close()
	if out.String() != "nothing to hide here\n" {
		t.Fatalf("redactor changed unrelated output: %q", out.String())
	}
}

// TestContainerGetsSecretNamesOnly (TM-30): the container argv carries `-e NAME` and never a value;
// the value goes through the CLI process environment only.
func TestContainerGetsSecretNamesOnly(t *testing.T) {
	values := map[string]string{"KEY_A": secretValue}
	args, err := secrets.ContainerArgs(values)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, " ") != "-e KEY_A" {
		t.Fatalf("container argv %q", args)
	}
	for _, a := range args {
		if strings.Contains(a, secretValue) || strings.Contains(a, "=") {
			t.Fatalf("a value or assignment reached the container argv: %q", a)
		}
	}
	if _, err = secrets.ContainerArgs(map[string]string{"bad name": "x"}); err == nil {
		t.Fatal("an invalid environment name was accepted")
	}
}
