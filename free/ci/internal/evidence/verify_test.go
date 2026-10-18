package evidence

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

const sampleRev = "0123456789abcdef0123456789abcdef01234567"

func sampleInputs() model.InputDigests {
	return model.InputDigests{Config: strings.Repeat("c", 64), Lockfiles: []model.DigestRef{{Path: "go.sum", SHA256: strings.Repeat("1", 64)}}, Generated: []model.DigestRef{}}
}

// sampleEvidence is a valid, bound, passing document for sampleRev.
func sampleEvidence() model.Evidence {
	return model.Evidence{
		Schema: "ci.evidence/v1", Revision: sampleRev, RepoID: "local:" + strings.Repeat("r", 8), RunID: "r1",
		Pipeline: model.PipelineRef{ID: "default", Name: "default", Kind: "default"}, Trigger: "local", SourceTrust: "owner", PolicyDigest: "p",
		InputDigests: sampleInputs(), WorktreeState: "clean", Binding: model.Binding{State: "bound"},
		Runner:   model.RunnerRef{ID: "local", Provider: "local", Platform: "linux/amd64", Isolation: "process"},
		Producer: model.Producer{Kind: "coordinator-recorded", Version: "1"}, ToolVersions: map[string]string{},
		Checks: []model.Check{{ID: "unit", JobID: "unit", Kind: "test", Tool: "t", Required: true, Substantive: true, Result: "pass"}},
		Artifacts: []model.Artifact{}, Retries: []model.Retry{}, Selection: model.Selection{Mode: "full", Reason: "selection.no_base", Digest: "d"},
		Result: "pass", Counts: model.Counts{Pass: 1, Total: 1}, Freshness: model.Freshness{State: "current"}, Signature: model.Signature{Kind: "coordinator-recorded"},
	}
}

func docOf(t *testing.T, ev model.Evidence) []byte {
	t.Helper()
	b, err := json.Marshal(ev)
	ok(t, err)
	return b
}

func TestSampleIsValid(t *testing.T) {
	if _, err := attest.Verify(docOf(t, sampleEvidence())); err != nil {
		t.Fatalf("the fixture is not valid evidence: %v", err)
	}
}

func TestJudgeExitCodes(t *testing.T) {
	cur := sampleInputs()
	changed := sampleInputs()
	changed.Lockfiles = []model.DigestRef{{Path: "go.sum", SHA256: strings.Repeat("2", 64)}}
	unbound := func(ev *model.Evidence) {
		reason := model.BindingReason("dirty")
		ev.WorktreeState, ev.Binding = "dirty", model.Binding{State: "unbound", Reason: &reason}
	}
	for _, tc := range []struct {
		name    string
		mut     func(*model.Evidence)
		current *model.InputDigests
		want    int
		reason  attest.Reason
	}{
		{"current bound pass", func(*model.Evidence) {}, &cur, ExitOK, ""},
		{"failing result", func(e *model.Evidence) {
			e.Result, e.Checks[0].Result, e.Counts = "fail", "fail", model.Counts{Fail: 1, Total: 1}
		}, &cur, ExitFails, attest.ReasonNotPassing},
		{"stale lockfile", func(*model.Evidence) {}, &changed, ExitStale, attest.ReasonStale},
		{"inputs unknown", func(*model.Evidence) {}, nil, ExitStale, attest.ReasonStale},
		{"unbound", unbound, &cur, ExitUnbound, attest.ReasonUnbound},
		{"unbound beats stale", unbound, &changed, ExitUnbound, attest.ReasonUnbound},
		{"other revision", func(e *model.Evidence) { e.Revision = strings.Repeat("9", 40) }, &cur, ExitInvalid, attest.ReasonRevisionMismatch},
		{"invalid beats unbound", func(e *model.Evidence) { unbound(e); e.Revision = strings.Repeat("9", 40) }, &cur, ExitInvalid, attest.ReasonRevisionMismatch},
		{"counts lie", func(e *model.Evidence) { e.Counts.Pass = 7 }, &cur, ExitInvalid, attest.ReasonInvalid},
		{"adhoc never gates", func(e *model.Evidence) { e.Pipeline.Kind = "adhoc" }, &cur, ExitFails, attest.ReasonAdhoc},
		{"drill never gates", func(e *model.Evidence) { e.Pipeline.Kind = "drill" }, &cur, ExitFails, attest.ReasonAdhoc},
		{"stored freshness is ignored", func(e *model.Evidence) { e.Freshness = model.Freshness{State: "current"} }, &changed, ExitStale, attest.ReasonStale},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev := sampleEvidence()
			tc.mut(&ev)
			v := Judge(ev, sampleRev, tc.current)
			if v.Exit != tc.want {
				t.Fatalf("exit %d (%v), want %d", v.Exit, v.Reasons, tc.want)
			}
			if tc.reason != "" && !hasReason(v.Reasons, tc.reason) {
				t.Fatalf("reasons %v lack %s", v.Reasons, tc.reason)
			}
			if tc.want == ExitOK && (v.Report.Freshness.State != "current" || v.Report.Class != ClassDefault || v.Report.Quarantined) {
				t.Fatalf("report: %+v", v.Report)
			}
		})
	}
}

func hasReason(l []attest.Reason, r attest.Reason) bool {
	for _, x := range l {
		if x == r {
			return true
		}
	}
	return false
}

func TestClassOf(t *testing.T) {
	ev := sampleEvidence()
	if ClassOf(ev) != ClassDefault {
		t.Fatal("default")
	}
	ev.Pipeline.Kind = "named"
	if ClassOf(ev) != ClassNamed {
		t.Fatal("named")
	}
	ev.Checks = append(ev.Checks, model.Check{ID: "rel", JobID: "rel", Kind: "release", Result: "fail"})
	if ClassOf(ev) != ClassNamed {
		t.Fatal("a failing release job does not make a release")
	}
	ev.Checks[1].Result = "pass"
	if ClassOf(ev) != ClassRelease {
		t.Fatal("a passing release job makes a release")
	}
}

func TestVerifyDocument(t *testing.T) {
	cur := sampleInputs()
	good := docOf(t, sampleEvidence())
	if v, err := VerifyDocument(good, sampleRev, &cur, nil); err != nil || v.Exit != ExitOK || len(v.Warnings) != 1 {
		t.Fatalf("good document: %+v %v (an unknown origin must warn)", v, err)
	}
	for name, doc := range map[string][]byte{
		"empty": nil, "not json": []byte("{"), "array": []byte("[]"), "trailing": append(append([]byte{}, good...), []byte(" {}")...),
		"unknown field": []byte(strings.Replace(string(good), `"schema":`, `"extra":1,"schema":`, 1)),
		"duplicate key": []byte(strings.Replace(string(good), `"schema":`, `"schema":"x","schema":`, 1)),
		"bad enum":      []byte(strings.Replace(string(good), `"result":"pass","counts"`, `"result":"maybe","counts"`, 1)),
	} {
		v, err := VerifyDocument(doc, sampleRev, &cur, nil)
		if e, isErr := AsError(err); !isErr || e.Code != CodeRejected || e.Exit != ExitInvalid || v.Exit != ExitInvalid {
			t.Errorf("%s: %+v %v", name, v, err)
		}
	}
	stored := sampleEvidence()
	same := func(model.Evidence) *model.Evidence { return &stored }
	if v, err := VerifyDocument(good, sampleRev, &cur, same); err != nil || v.Exit != ExitOK || len(v.Warnings) != 0 {
		t.Fatalf("matching record: %+v %v", v, err)
	}
	edited := sampleEvidence()
	edited.InputDigests.Lockfiles[0].SHA256 = strings.Repeat("2", 64)
	if v, err := VerifyDocument(docOf(t, edited), sampleRev, &edited.InputDigests, same); err != nil || v.Exit != ExitInvalid {
		t.Fatalf("a document that differs from the recorded one must be invalid: %+v %v", v, err)
	}
}

func TestMissingInputsFailClosed(t *testing.T) {
	dir, sha := newRepo(t, map[string]string{"go.mod": "module x\n"})
	tree, err := TreeFS(t.Context(), nil, dir, sha)
	ok(t, err)
	ev := sampleEvidence()
	missing := MissingInputs(ev, tree)
	if len(missing) != 1 || missing[0] != "go.sum" {
		t.Fatalf("missing: %v", missing)
	}
	v := Judge(ev, sampleRev, &ev.InputDigests)
	v.MarkMissing(missing)
	if v.Exit != ExitInvalid || len(v.Warnings) != 1 {
		t.Fatalf("a missing input must be invalid: %+v", v)
	}
}

// FuzzVerifyFile: for any bytes, verification never panics; exit 0 only for a
// document that passes the strict decoder, names the revision, is bound and
// clean, is fresh against the inputs and passes; a decode failure is exit 11.
func FuzzVerifyFile(f *testing.F) {
	good := sampleEvidence()
	body, _ := json.Marshal(good)
	f.Add(body, sampleRev)
	f.Add([]byte(`{}`), sampleRev)
	f.Add([]byte(``), "")
	f.Add(body, "main")
	bad := good
	bad.Revision = strings.Repeat("9", 64)
	body2, _ := json.Marshal(bad)
	f.Add(body2, sampleRev)
	cur := sampleInputs()
	f.Fuzz(func(t *testing.T, doc []byte, want string) {
		v, err := VerifyDocument(doc, want, &cur, nil)
		if err != nil {
			if e, isErr := AsError(err); !isErr || e.Exit != ExitInvalid || v.Exit != ExitInvalid {
				t.Fatalf("decode failure must be exit 11: %v %+v", err, v)
			}
			return
		}
		if v.Exit != ExitOK {
			return
		}
		ev, derr := attest.Verify(doc)
		if derr != nil || ev.Revision != want || ev.Binding.State != "bound" || ev.WorktreeState != "clean" || ev.Result != "pass" ||
			attest.Fresh(ev, cur).State != "current" || ev.Pipeline.Kind == "adhoc" || ev.Pipeline.Kind == "drill" {
			t.Fatalf("exit 0 for a document that must not gate: %s (%v)", doc, derr)
		}
	})
}
