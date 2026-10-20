//go:build cachesec

package cachesec

// Purpose: TM-18 cases on the signer carrier and the verifier: a release manifest re-signed with a key of
// another purpose (plugins, agent) is refused by release verification, a signer request for a digest the run
// never committed is refused by the signer before the key is resolved, and a signer asked while an attempt of
// the run is still running refuses (no signer process serves a run a job can still change).
// Inputs: newG4Release, newG4SignerFx. Outputs: registered cases C18-SIG-01..04.
// Constraints: production packages only. Neither verify.Release nor signing.Handler writes an audit row (the
//          threat model's TM-18 row names none), so the cases assert the audit table stays empty and report "none".

import (
	"context"
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"

	sdk "github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// g4None asserts that nothing was audited and returns the sentinel the matrix names.
func g4None(w *world) string {
	for _, r := range g4AuditRows(w, "") {
		if r.Action != "node.register" { // the fixture's own setup row
			w.t.Fatalf("an audit row was written: %s", r.Action)
		}
	}
	return "none"
}

// g4Resigned publishes a release whose manifest is signed by a key of another purpose and asserts the honest one verifies.
func g4Resigned(t *testing.T, purpose sdk.Purpose) Observed {
	w := newWorld(t)
	honest := newG4Release(t, "run-9", nil)
	if res := honest.Verify(); !res.Verified {
		t.Fatalf("control: the honest release does not verify: %s %s", res.Reason, res.Message)
	}
	foreign := ed25519.NewKeyFromSeed([]byte(strings.Repeat("f", 32)))
	bad := newG4Release(t, "run-9", func(r *g4Release) {
		r.manSign = sdk.NewEd25519Signer(purpose, foreign)
		r.rewriteK = true // the manifest also names the foreign key, as an attacker's re-signed copy would
	})
	res := bad.Verify()
	if res.Verified {
		t.Fatalf("a manifest signed with a %s key verified", purpose)
	}
	return Observed{Outcome: Refused, Code: res.Code, Reason: res.Reason, Audit: g4None(w), Landed: 0}
}

func g4SignerReason(err error) string {
	var e *signing.Error
	if !errors.As(err, &e) {
		return ""
	}
	return e.Reason
}

func init() {
	register("C18-SIG-01", func(t *testing.T) Observed { return g4Resigned(t, sdk.PurposePlugins) })
	register("C18-SIG-02", func(t *testing.T) Observed { return g4Resigned(t, sdk.PurposeAgent) })

	register("C18-SIG-03", func(t *testing.T) Observed {
		w := newWorld(t)
		f := newG4SignerFx(t)
		req, payloads := f.request(f.payload(nil))
		envs, err := f.handler().Handle(context.Background(), req, payloads) // control: the committed digest is signed
		if err != nil || len(envs) != 1 {
			t.Fatalf("control: honest request refused: %v", err)
		}
		before := f.res.calls
		req, payloads = f.request(f.payload(func(m *manifest.Manifest) { m.Artifacts[0].SHA256 = strings.Repeat("c", 64) })) // a digest no record holds
		out, err := f.handler().Handle(context.Background(), req, payloads)
		got := refusal(t, err, g4SignerReason)
		if f.res.calls != before {
			t.Fatalf("the release key was resolved for a refused request (%d -> %d)", before, f.res.calls)
		}
		got.Audit, got.Landed = g4None(w), int64(len(out))
		return got
	})

	register("C18-SIG-04", func(t *testing.T) Observed {
		w := newWorld(t)
		f := newG4SignerFx(t)
		req, payloads := f.request(f.payload(nil))
		if _, err := f.handler().Handle(context.Background(), req, payloads); err != nil { // control: every attempt final
			t.Fatalf("control: %v", err)
		}
		before := f.res.calls
		f.src.attempts = append(f.src.attempts, store.AttemptView{ID: "att-test-2", JobID: "job-test", N: 2, State: "running"}) // a job of the run is still running
		if err := signing.CheckTerminal(context.Background(), f.src, g4SigRun); err == nil {
			t.Fatal("the coordinator's pre-start check admitted a run with a running attempt")
		}
		out, err := f.handler().Handle(context.Background(), req, payloads)
		got := refusal(t, err, g4SignerReason)
		if f.res.calls != before {
			t.Fatalf("the release key was resolved while a job of the run was running (%d -> %d)", before, f.res.calls)
		}
		got.Audit, got.Landed = g4None(w), int64(len(out))
		return got
	})
}

// g4Registrar is the redaction registry stand-in of a Source: it only counts registrations.
type g4Registrar struct{ n int }

func (r *g4Registrar) Register([]byte) error { r.n++; return nil }

func init() {
	// C18-SIG-05: a job reaching for release secrets. The job's secret layer (secrets.Source) is the only way a job gets a
	// value; the signing key ref is refused at parse (E674) and an untrusted job asking for any release-class ref is
	// refused E677 before the resolver is called.
	register("C18-SIG-05", func(t *testing.T) Observed {
		w := newWorld(t)
		f := newG4SignerFx(t) // its resolver holds the release key and counts calls
		f.res.vals["project/BUILD_TOKEN"] = []byte("SYNTHETIC_TOKEN_aaaa1111")
		source := func(jobTrust model.TrustClass) *secrets.Source {
			return secrets.New(secrets.Config{Mode: secrets.ModePersonalLocal, Attempt: "att-job", Resolver: f.res, Registry: &g4Registrar{},
				RedactorInstalled: func() bool { return true },
				Facts: secrets.Facts{Job: trust.JobFacts{Trust: jobTrust, Mode: trust.Personal, PersonalOwnRun: true},
					Runner: trust.RunnerFacts{Ownership: "operator", PersonalOwnNode: true, Dedicated: true, Isolation: "process", UIDSeparation: true}}})
		}
		ok := source("internal")
		got, err := ok.Resolve(context.Background(), []string{"project/BUILD_TOKEN"}) // control: the same wiring serves an ordinary ref
		if err != nil || len(got) != 1 {
			t.Fatalf("control: an ordinary ref was refused: %v", err)
		}
		ok.Zero()
		before := f.res.calls
		// the signing key itself cannot even be named by a job: the reference parser refuses its NAME (E674).
		if _, err = source("internal").Resolve(context.Background(), []string{signing.KeyRef}); secrets.CodeOf(err) != "E674" {
			t.Fatalf("the release signing key ref was not refused at parse: %v", err)
		}
		vals, err := source("untrusted").Resolve(context.Background(), []string{"release/PUBLISH_TOKEN"})
		var se *secrets.Error
		if !errors.As(err, &se) || se.Code != "E677" || len(se.Reasons) == 0 {
			t.Fatalf("want E677 with reasons, got %v", err)
		}
		if f.res.calls != before {
			t.Fatalf("the resolver was called for a refused job (%d -> %d)", before, f.res.calls)
		}
		// the production reason key is "secrets.untrusted"; the matrix keys are bare words, so the namespace is dropped.
		return Observed{Outcome: Refused, Code: se.Code, Reason: strings.TrimPrefix(se.Reasons[0], "secrets."), Audit: g4None(w), Landed: int64(len(vals))}
	})
}
