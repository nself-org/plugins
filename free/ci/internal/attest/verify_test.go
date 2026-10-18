package attest_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func rejection(t *testing.T, err error) *attest.Rejection {
	t.Helper()
	var r *attest.Rejection
	if !errors.As(err, &r) {
		t.Fatalf("want *Rejection, got %v", err)
	}
	return r
}

func TestVerify_AcceptsAGoodDocument(t *testing.T) {
	attest.RegisterRemoteVerifier(nil)
	ev, err := attest.Verify(marshal(t, goodEvidence()))
	if err != nil || ev.Revision != goodRevision || ev.Checks[0].ID != "vet" {
		t.Fatalf("%v %+v", err, ev)
	}
}

func TestVerify_RejectsMalformedDocuments(t *testing.T) {
	good := string(marshal(t, goodEvidence()))
	var generic map[string]any
	_ = json.Unmarshal([]byte(good), &generic)
	withField := func(k string, v any) string {
		m := map[string]any{}
		for key, val := range generic {
			m[key] = val
		}
		m[k] = v
		b, _ := json.Marshal(m)
		return string(b)
	}
	rows := map[string]string{
		"empty":          "",
		"not json":       "not json",
		"array":          "[]",
		"trailing data":  good + "{}",
		"unknown field":  withField("extra", 1),
		"wrong schema":   withField("schema", "ci.evidence/v2"),
		"uppercase rev":  withField("revision", strings.ToUpper(goodRevision)),
		"short rev":      withField("revision", "abc"),
		"bad trust":      withField("source_trust", "root"),
		"bad result":     withField("result", "great"),
		"null checks":    withField("checks", nil),
		"missing policy": strings.Replace(good, `"policy_digest":"policy-1",`, "", 1),
	}
	for name, doc := range rows {
		if _, err := attest.Verify([]byte(doc)); err == nil {
			t.Errorf("%s accepted", name)
		} else {
			rejection(t, err)
		}
	}
}

func TestVerify_TooLarge(t *testing.T) {
	big := make([]byte, attest.MaxDocument+1)
	if _, err := attest.Verify(big); err == nil {
		t.Fatal("oversized document accepted")
	}
}

func TestVerify_ConsistencyReasons(t *testing.T) {
	attest.RegisterRemoteVerifier(nil)
	rows := map[string]func(*model.Evidence){
		"dirty but bound":   func(e *model.Evidence) { e.WorktreeState = "dirty" },
		"bound with change": func(e *model.Evidence) { e.TrackedChangesDigest = strp("d") },
		"counts":            func(e *model.Evidence) { e.Counts.Total = 9 },
	}
	for name, bend := range rows {
		ev := goodEvidence()
		bend(&ev)
		if _, err := attest.Verify(marshal(t, ev)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestVerify_SignedRemote(t *testing.T) {
	t.Cleanup(func() { attest.RegisterRemoteVerifier(nil) })
	doc := marshal(t, signedRemote(goodEvidence()))
	attest.RegisterRemoteVerifier(nil)
	_, err := attest.Verify(doc)
	if r := rejection(t, err); !sameReasons(r.Reasons, []attest.Reason{attest.ReasonSignatureUnverifiable}) {
		t.Fatalf("no verifier: %v", r.Reasons)
	}
	fv := &fakeVerifier{}
	attest.RegisterRemoteVerifier(fv)
	if _, err = attest.Verify(doc); err != nil || fv.calls != 1 {
		t.Fatalf("verified: %v calls %d", err, fv.calls)
	}
	attest.RegisterRemoteVerifier(&fakeVerifier{err: errors.New("no")})
	if _, err = attest.Verify(doc); err == nil {
		t.Fatal("refusing verifier accepted")
	}
}

func TestVerify_ConsultsTheDocumentNotItsStoredFreshness(t *testing.T) {
	ev := goodEvidence()
	ev.Freshness = model.Freshness{State: "stale", Reason: strp("old")}
	if _, err := attest.Verify(marshal(t, ev)); err != nil {
		t.Fatalf("stored freshness should not matter to Verify: %v", err)
	}
}

func TestRejectionError(t *testing.T) {
	if attest.Reject(nil, "x") != nil {
		t.Fatal("no reasons must be nil")
	}
	err := attest.Reject([]attest.Reason{attest.ReasonStale, attest.ReasonAdhoc}, "why")
	want := "E685: evidence rejected: evidence.stale, evidence.adhoc (why)"
	if err.Error() != want {
		t.Fatalf("got %q", err.Error())
	}
}

func TestConsistent(t *testing.T) {
	if got := attest.Consistent(goodEvidence()); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	ev := goodEvidence()
	ev.Schema = "x"
	if got := attest.Consistent(ev); !hasReason(got, attest.ReasonInvalid) {
		t.Fatalf("%v", got)
	}
}

func TestCodeRegistered(t *testing.T) {
	if problems := model.CodeProblems(); len(problems) != 0 {
		t.Fatalf("%v", problems)
	}
}
