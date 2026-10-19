package signing_test

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func verifier(t *testing.T, f *fixture, p sdk.Purpose) *sdk.Verifier {
	t.Helper()
	v, err := sdk.NewVerifier(p, []sdk.Key{{ID: sdk.KeyID(p, f.key.Public), Purpose: p, Public: f.key.Public}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSignsManifestAndStatement(t *testing.T) {
	f := newFixture(t)
	mp, sp := f.manifestPayload(nil), f.statementPayload(nil)
	req := request(item(manifest.PayloadType, mp), item(signing.PayloadStatement, sp))
	envs, err := f.handler().Handle(context.Background(), req, decodeAll(t, req))
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 2 {
		t.Fatalf("got %d envelopes", len(envs))
	}
	for i, want := range [][]byte{mp, sp} {
		typ, payload, ids, err := sdk.VerifyEnvelope(verifier(t, f, sdk.PurposeCIRelease), envs[i])
		if err != nil || typ != req.Items[i].PayloadType || string(payload) != string(want) || len(ids) != 1 || ids[0] != f.key.ID {
			t.Fatalf("envelope %d: %v %q %v", i, err, typ, ids)
		}
		for _, other := range []sdk.Purpose{sdk.PurposePlugins, sdk.PurposeAgent} {
			if _, _, _, err := sdk.VerifyEnvelope(verifier(t, f, other), envs[i]); err == nil {
				t.Fatalf("envelope %d verified under %s", i, other)
			}
		}
	}
	if f.res.attempts[0] != "att-rel-2" {
		t.Fatalf("the key was resolved for attempt %v", f.res.attempts)
	}
}

func TestStatementAloneUsesLastPassedReleaseAttempt(t *testing.T) {
	f := newFixture(t)
	req := request(item(signing.PayloadStatement, f.statementPayload(nil)))
	if _, err := f.handler().Handle(context.Background(), req, decodeAll(t, req)); err != nil {
		t.Fatal(err)
	}
	if f.res.attempts[0] != "att-rel-2" {
		t.Fatalf("resolved for %v, want the highest passed release attempt", f.res.attempts)
	}
	f.src.attempts = []store.AttemptView{{ID: "att-x", JobID: "job-test", N: 1, State: "passed"}}
	if _, err := f.handler().Handle(context.Background(), req, decodeAll(t, req)); signing.IsReason(err, signing.ReasonPayloadRejected) == false {
		t.Fatalf("a run without a passed release attempt signed: %v", err)
	}
}

func TestSignerRefuses(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		build  func(f *fixture) signing.Request
		tweak  func(f *fixture)
	}{
		{"artifact digest is not a record", signing.ReasonDigestNotCommitted, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) { m.Artifacts[0].SHA256 = strings.Repeat("c", 64) })))
		}, nil},
		{"artifact size differs", signing.ReasonDigestNotCommitted, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) { m.Artifacts[0].Size++ })))
		}, nil},
		{"artifact name differs", signing.ReasonDigestNotCommitted, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) {
				m.Artifacts[0].Name, m.Artifacts[0].Path = "other", "artifacts/other"
				m.Artifacts[0].SBOM.CycloneDX.Path, m.Artifacts[0].SBOM.SPDX.Path = "sbom/other.cdx.json", "sbom/other.spdx.json"
			})))
		}, nil},
		{"sbom digest is not a record", signing.ReasonDigestNotCommitted, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) { m.Artifacts[1].SBOM.SPDX.SHA256 = strings.Repeat("d", 64) })))
		}, nil},
		{"revision differs", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) { m.Revision = strings.Repeat("b", 40) })))
		}, nil},
		{"evidence digest differs", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) { m.Evidence.SHA256 = strings.Repeat("e", 64) })))
		}, nil},
		{"repo id differs", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) { m.RepoID = "repo-2" })))
		}, nil},
		{"manifest run id differs", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) { m.Release.RunID = "run-9" })))
		}, nil},
		{"attempt is not passed", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) { m.Release.AttemptID = "att-rel-1" })))
		}, nil},
		{"attempt is of a check job", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) {
				m.Release.AttemptID, m.Release.JobID = "att-test", "job-test"
			})))
		}, nil},
		{"attempt of another job", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) { m.Release.AttemptID = "att-test" })))
		}, nil},
		{"unknown attempt", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) { m.Release.AttemptID = "att-zz" })))
		}, nil},
		{"manifest is not v1", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, []byte(`{"schema":"x"}`)))
		}, nil},
		{"manifest names another key", signing.ReasonKeyIDMismatch, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) { m.Signing.KeyID = "ci-release-0123456789abcdef" })))
		}, nil},
		{"evidence did not pass", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(nil)))
		}, func(f *fixture) {
			f.setEvidence(`{"revision":"` + revision + `","repo_id":"repo-1","run_id":"run-1","result":"fail"}`)
		}},
		{"evidence names another run", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(nil)))
		}, func(f *fixture) {
			f.setEvidence(`{"revision":"` + revision + `","repo_id":"repo-1","run_id":"run-2","result":"pass"}`)
		}},
		{"evidence names another revision", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(nil)))
		}, func(f *fixture) {
			f.setEvidence(`{"revision":"` + strings.Repeat("b", 40) + `","repo_id":"repo-1","run_id":"run-1","result":"pass"}`)
		}},
		{"no run evidence", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(nil)))
		}, func(f *fixture) { f.src.evidence = f.src.evidence[:1] }},
		{"evidence does not parse", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(nil)))
		}, func(f *fixture) { f.setEvidence(`not json`) }},
		{"record not committed", signing.ReasonDigestNotCommitted, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(nil)))
		}, func(f *fixture) { f.mutateRecord("app", func(r *records.Record) { r.State = "staged" }) }},
		{"record of another run", signing.ReasonDigestNotCommitted, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(nil)))
		}, func(f *fixture) { f.mutateRecord("app", func(r *records.Record) { r.Run = "run-2" }) }},
		{"record of another revision", signing.ReasonDigestNotCommitted, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(nil)))
		}, func(f *fixture) {
			f.mutateRecord("app", func(r *records.Record) { r.Revision = strings.Repeat("b", 40) })
		}},
		{"record marked missing", signing.ReasonDigestNotCommitted, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(nil)))
		}, func(f *fixture) { f.mutateRecord("app", func(r *records.Record) { r.Missing = true }) }},
		{"blob gone", signing.ReasonDigestNotCommitted, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(nil)))
		}, func(f *fixture) { delete(f.blobs.data, sum(f.files["app"])) }},
		{"blob bytes changed", signing.ReasonDigestNotCommitted, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(nil)))
		}, func(f *fixture) { f.blobs.data[sum(f.files["app"])] = []byte("evil-bytes") }},
		{"blob size changed", signing.ReasonDigestNotCommitted, func(f *fixture) signing.Request {
			return request(item(manifest.PayloadType, f.manifestPayload(nil)))
		}, func(f *fixture) { f.blobs.data[sum(f.files["app"])] = []byte("app-bytes-and-more") }},
		{"statement subject unknown", signing.ReasonDigestNotCommitted, func(f *fixture) signing.Request {
			return request(item(signing.PayloadStatement, f.statementPayload(func(m map[string]any) {
				m["subject"] = []map[string]any{{"name": "x", "digest": map[string]string{"sha256": strings.Repeat("f", 64)}}}
			})))
		}, nil},
		{"statement wrong type", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(signing.PayloadStatement, f.statementPayload(func(m map[string]any) { m["_type"] = "x" })))
		}, nil},
		{"statement wrong predicate", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(signing.PayloadStatement, f.statementPayload(func(m map[string]any) { m["predicateType"] = "x" })))
		}, nil},
		{"statement without predicate", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(signing.PayloadStatement, f.statementPayload(func(m map[string]any) { delete(m, "predicate") })))
		}, nil},
		{"statement without subjects", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(signing.PayloadStatement, f.statementPayload(func(m map[string]any) { m["subject"] = []any{} })))
		}, nil},
		{"statement subject bad digest", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(signing.PayloadStatement, f.statementPayload(func(m map[string]any) {
				m["subject"] = []map[string]any{{"name": "x", "digest": map[string]string{"sha256": "XYZ"}}}
			})))
		}, nil},
		{"statement subject two digests", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(signing.PayloadStatement, f.statementPayload(func(m map[string]any) {
				m["subject"] = []map[string]any{{"name": "x", "digest": map[string]string{"sha256": sum(f.files["app"]), "sha512": "aa"}}}
			})))
		}, nil},
		{"statement subject no name", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(signing.PayloadStatement, f.statementPayload(func(m map[string]any) {
				m["subject"] = []map[string]any{{"name": "", "digest": map[string]string{"sha256": sum(f.files["app"])}}}
			})))
		}, nil},
		{"statement not json", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			return request(item(signing.PayloadStatement, []byte("nope")))
		}, nil},
		{"unknown run", signing.ReasonPayloadRejected, func(f *fixture) signing.Request {
			r := request(item(signing.PayloadStatement, f.statementPayload(nil)))
			r.RunID = "run-9"
			return r
		}, nil},
		{"run store unreadable", signing.ReasonSignerFailed, func(f *fixture) signing.Request {
			return request(item(signing.PayloadStatement, f.statementPayload(nil)))
		}, func(f *fixture) { f.src.err = context.DeadlineExceeded }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			req := c.build(f)
			if c.tweak != nil {
				c.tweak(f)
			}
			envs, err := f.handler().Handle(context.Background(), req, decodeAll(t, req))
			if err == nil || envs != nil {
				t.Fatalf("signed: %v %v", envs, err)
			}
			if got := reasonOf(t, err); got != c.reason {
				t.Fatalf("reason %s, want %s (%v)", got, c.reason, err)
			}
			if c.reason != signing.ReasonKeyIDMismatch && f.res.calls != 0 {
				t.Fatal("the key was resolved for a refused request")
			}
		})
	}
}

func TestSignerRefusesManifestsNamingDifferentKeysInEitherOrder(t *testing.T) {
	f := newFixture(t)
	right := item(manifest.PayloadType, f.manifestPayload(nil))
	wrong := item(manifest.PayloadType, f.manifestPayload(func(m *manifest.Manifest) { m.Signing.KeyID = "ci-release-0123456789abcdef" }))
	for _, items := range [][]signing.Item{{wrong, right}, {right, wrong}} {
		req := request(items...)
		if _, err := f.handler().Handle(context.Background(), req, decodeAll(t, req)); !signing.IsReason(err, signing.ReasonKeyIDMismatch) || f.res.calls != 0 {
			t.Fatalf("%v (%d resolves)", err, f.res.calls)
		}
	}
	req := request(right, right)
	if _, err := f.handler().Handle(context.Background(), req, decodeAll(t, req)); err != nil {
		t.Fatalf("two manifests naming the same key: %v", err)
	}
}

func TestStatementSubjectCountBoundary(t *testing.T) {
	f := newFixture(t)
	subjects := func(n int) func(map[string]any) {
		return func(m map[string]any) {
			list := make([]map[string]any, n)
			for i := range list {
				list[i] = map[string]any{"name": "app", "digest": map[string]string{"sha256": sum(f.files["app"])}}
			}
			m["subject"] = list
		}
	}
	at := request(item(signing.PayloadStatement, f.statementPayload(subjects(4096))))
	if _, err := f.handler().Handle(context.Background(), at, decodeAll(t, at)); err != nil {
		t.Fatalf("4096 subjects: %v", err)
	}
	over := request(item(signing.PayloadStatement, f.statementPayload(subjects(4097))))
	if _, err := f.handler().Handle(context.Background(), over, decodeAll(t, over)); !signing.IsReason(err, signing.ReasonPayloadRejected) {
		t.Fatalf("4097 subjects: %v", err)
	}
}

func TestReleaseAttemptChoice(t *testing.T) {
	f := newFixture(t)
	f.src.jobs = append(f.src.jobs, store.RunJob{ID: "job-rel-b", Name: "publish-b", Kind: "release"})
	f.src.attempts = []store.AttemptView{
		{ID: "a-3", JobID: "job-rel", N: 3, State: "failed"},
		{ID: "a-1", JobID: "job-rel", N: 1, State: "passed"},
		{ID: "a-2", JobID: "job-rel-b", N: 2, State: "passed"},
		{ID: "a-2b", JobID: "job-rel", N: 2, State: "passed"},
		{ID: "a-c", JobID: "job-test", N: 9, State: "passed"},
	}
	req := request(item(signing.PayloadStatement, f.statementPayload(nil)))
	if _, err := f.handler().Handle(context.Background(), req, decodeAll(t, req)); err != nil {
		t.Fatal(err)
	}
	if f.res.attempts[0] != "a-2" {
		t.Fatalf("resolved for %q: want the highest passed release attempt, first listed on a tie", f.res.attempts[0])
	}
}

func TestSignerRefusesNonTerminalRun(t *testing.T) {
	for _, state := range []string{"queued", "leased", "running", "finalizing", "lost", "retryable", "needs-operator"} {
		f := newFixture(t)
		f.src.attempts[0].State = state
		req := request(item(signing.PayloadStatement, f.statementPayload(nil)))
		if _, err := f.handler().Handle(context.Background(), req, decodeAll(t, req)); !signing.IsReason(err, signing.ReasonPayloadRejected) || f.res.calls != 0 {
			t.Fatalf("%s: %v", state, err)
		}
	}
	f := newFixture(t)
	f.src.attempts = nil
	if err := signing.CheckTerminal(context.Background(), f.src, runID); !signing.IsReason(err, signing.ReasonPayloadRejected) {
		t.Fatalf("a run with no attempt passed: %v", err)
	}
	f.src.attempts = []store.AttemptView{{ID: "a", State: "cancelled"}, {ID: "b", State: "failed"}, {ID: "c", State: "passed"}}
	if err := signing.CheckTerminal(context.Background(), f.src, runID); err != nil {
		t.Fatal(err)
	}
}

func TestSignerKeyFailures(t *testing.T) {
	f := newFixture(t)
	req := request(item(signing.PayloadStatement, f.statementPayload(nil)))
	delete(f.res.vals, signing.KeyRef)
	if _, err := f.handler().Handle(context.Background(), req, decodeAll(t, req)); !signing.IsReason(err, signing.ReasonKeyMissing) || !strings.Contains(err.Error(), "key init") {
		t.Fatalf("missing key: %v", err)
	}
}
