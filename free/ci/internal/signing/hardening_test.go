package signing_test

// Purpose: tests for the review round of P7-CACHE-07: strict JSON, subject
// names, project binding, re-checks after key resolution, the record row cap,
// key management locking and recovery, and the coordinator's shape checks.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestStrictJSONRefusals(t *testing.T) {
	st := func(s string) error {
		f := newFixture(t)
		req := request(item(signing.PayloadStatement, []byte(s)))
		_, err := f.handler().Handle(context.Background(), req, decodeAll(t, req))
		return err
	}
	good := newFixture(t).statementPayload(nil)
	// sanity: the unmodified statement passes
	if err := st(string(good)); err != nil {
		t.Fatal(err)
	}
	app := sum([]byte("app-bytes"))
	subj := `[{"name":"app","digest":{"sha256":"` + app + `"}}]`
	base := func(extra string) string {
		return `{"_type":"https://in-toto.io/Statement/v1","predicateType":"` + manifest.ProvenancePredicate + `","predicate":{}` + extra + `}`
	}
	bad := map[string]string{
		"duplicate subject":       base(`,"subject":` + subj + `,"subject":` + subj),
		"case-variant subject":    base(`,"subject":` + subj + `,"Subject":[{"name":"x","digest":{"sha256":"` + strings.Repeat("f", 64) + `"}}]`),
		"unicode-folded key":      base(`,"subject":` + subj + `,"ſubject":[]`),
		"escaped duplicate key":   base(`,"subject":` + subj + `,"subject":[]`),
		"duplicate nested digest": base(`,"subject":[{"name":"app","digest":{"sha256":"` + app + `","sha256":"` + app + `"}}]`),
		"duplicate predicate key": `{"_type":"https://in-toto.io/Statement/v1","predicateType":"` + manifest.ProvenancePredicate + `","predicate":{"a":1,"A":2},"subject":` + subj + `}`,
		"trailing document":       base(`,"subject":`+subj) + ` {}`,
		"trailing bracket":        base(`,"subject":`+subj) + ` ]`,
		"trailing brace":          base(`,"subject":`+subj) + `}`,
		"invalid utf-8":           base(`,"subject":`+subj) + "\xff",
		"too deep":                base(`,"subject":` + subj + `,"x":` + strings.Repeat("[", 40) + strings.Repeat("]", 40)),
		"array at top":            `[1]`,
		"scalar at top":           `7`,
		"truncated":               base(`,"subject":` + subj)[:60],
	}
	for name, s := range bad {
		if err := st(s); !signing.IsReason(err, signing.ReasonPayloadRejected) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// the same key in sibling objects and arrays is fine
	ok := base(`,"subject":[{"name":"app","digest":{"sha256":"` + app + `"}},{"name":"app","digest":{"sha256":"` + app + `"}}],"extra":[{"a":1},{"a":2}]`)
	if err := st(ok); err != nil {
		t.Fatalf("sibling objects with equal keys: %v", err)
	}
}

func TestDecodeRequestStrictness(t *testing.T) {
	typ := signing.PayloadStatement
	for name, in := range map[string]string{
		"duplicate run_id":    `{"run_id":"a","run_id":"b","items":[{"payload_type":"` + typ + `","payload_b64":"eA=="}]}`,
		"case-variant key":    `{"run_id":"a","Run_ID":"b","items":[{"payload_type":"` + typ + `","payload_b64":"eA=="}]}`,
		"trailing bracket":    `{"run_id":"a","items":[{"payload_type":"` + typ + `","payload_b64":"eA=="}]}]`,
		"trailing brace":      `{"run_id":"a","items":[{"payload_type":"` + typ + `","payload_b64":"eA=="}]}}`,
		"duplicate item keys": `{"run_id":"a","items":[{"payload_type":"` + typ + `","payload_b64":"eA==","payload_b64":"eA=="}]}`,
	} {
		if err := decode(in); !signing.IsReason(err, signing.ReasonPayloadRejected) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestStatementSubjectNames(t *testing.T) {
	f := newFixture(t)
	handle := func(name, digest string) error {
		req := request(item(signing.PayloadStatement, f.statementPayload(func(m map[string]any) {
			m["subject"] = []map[string]any{{"name": name, "digest": map[string]string{"sha256": digest}}}
		})))
		_, err := f.handler().Handle(context.Background(), req, decodeAll(t, req))
		return err
	}
	app, ev := sum(f.files["app"]), sum(f.evRaw)
	for _, c := range []struct {
		name, digest string
		ok           bool
	}{
		{"app", app, true},
		{"artifacts/app", app, true},
		{"never-built", app, false},
		{"image", app, false},
		{"artifacts/never-built", app, false},
		{"artifacts/artifacts/app", app, false},
		{"evidence.json", ev, true},
		{"evidence", ev, false},
		{"app", ev, false},
		{"evidence.json", app, false},
	} {
		err := handle(c.name, c.digest)
		if c.ok && err != nil || !c.ok && !signing.IsReason(err, signing.ReasonDigestNotCommitted) {
			t.Errorf("%s %s...: %v", c.name, c.digest[:6], err)
		}
	}
}

func TestSBOMDigestMustBeAnSBOMRecord(t *testing.T) {
	f := newFixture(t)
	f.mutateRecord("app.cdx", func(r *records.Record) { r.Kind = records.KindBinary })
	req := request(item(manifest.PayloadType, f.manifestPayload(nil)))
	if _, err := f.handler().Handle(context.Background(), req, decodeAll(t, req)); !signing.IsReason(err, signing.ReasonDigestNotCommitted) {
		t.Fatalf("an sbom digest backed by a binary record: %v", err)
	}
}

func TestRunMustBelongToTheServedProject(t *testing.T) {
	f := newFixture(t)
	req := request(item(signing.PayloadStatement, f.statementPayload(nil)))
	h := f.handler()
	h.Project = "/another/project"
	if _, err := h.Handle(context.Background(), req, decodeAll(t, req)); !signing.IsReason(err, signing.ReasonPayloadRejected) || f.res.calls != 0 {
		t.Fatalf("a run of another project: %v (%d resolves)", err, f.res.calls)
	}
	h.Project = project + "/" // an equal path with a trailing slash is the same project
	if _, err := h.Handle(context.Background(), req, decodeAll(t, req)); err != nil {
		t.Fatalf("%v", err)
	}
	h.Project = ""
	if _, err := h.Handle(context.Background(), req, decodeAll(t, req)); err != nil {
		t.Fatalf("no project bound: %v", err)
	}
}

func TestRunChangesWhileTheKeyIsResolved(t *testing.T) {
	mutations := map[string]func(f *fixture){
		"a new running attempt": func(f *fixture) {
			f.src.attempts = append(f.src.attempts, store.AttemptView{ID: "att-new", JobID: "job-rel", N: 3, State: "running"})
		},
		"a record marked missing": func(f *fixture) { f.mutateRecord("app", func(r *records.Record) { r.Missing = true }) },
		"blob bytes replaced":     func(f *fixture) { f.blobs.data[sum(f.files["app"])] = []byte("evil-bytes") },
		"the release attempt choice changes": func(f *fixture) {
			f.src.attempts = append(f.src.attempts, store.AttemptView{ID: "att-rel-9", JobID: "job-rel", N: 9, State: "passed"})
		},
	}
	for name, mutate := range mutations {
		f := newFixture(t)
		f.res.onResolve = func() { mutate(f) }
		req := request(item(signing.PayloadStatement, f.statementPayload(nil)), item(manifest.PayloadType, f.manifestPayload(nil)))
		if strings.HasSuffix(name, "choice changes") {
			req = request(item(signing.PayloadStatement, f.statementPayload(nil))) // no manifest names the attempt
		}
		envs, err := f.handler().Handle(context.Background(), req, decodeAll(t, req))
		if err == nil || envs != nil {
			t.Errorf("%s: signed over a run that changed during key resolution: %v", name, err)
		}
	}
}

func TestRecordListingAtTheStoreCapIsRefused(t *testing.T) {
	f := newFixture(t)
	f.src.extra = 1000
	req := request(item(manifest.PayloadType, f.manifestPayload(nil)))
	if _, err := f.handler().Handle(context.Background(), req, decodeAll(t, req)); !signing.IsReason(err, signing.ReasonSignerFailed) {
		t.Fatalf("a capped listing: %v", err)
	}
	f.src.extra = 995
	if _, err := f.handler().Handle(context.Background(), req, decodeAll(t, req)); err != nil {
		t.Fatalf("a listing just under the cap: %v", err)
	}
}

func TestInitRepairsAStaleStoredID(t *testing.T) {
	res := &fakeRes{vals: map[string][]byte{signing.KeyIDRef: []byte("ci-release-0123456789abcdef")}}
	st := &fakeStore{res: res}
	info, err := signing.Init(context.Background(), res, st)
	if err != nil || len(st.sets) != 2 || st.sets[1] != signing.KeyIDName || string(res.vals[signing.KeyIDRef]) != info.KeyID {
		t.Fatalf("%+v %v %v", info, st.sets, err)
	}
	if _, err := signing.Public(context.Background(), res, "x"); err != nil {
		t.Fatalf("the key does not load after init: %v", err)
	}
}

func TestRotateRecoversFromAPartialFailure(t *testing.T) {
	old, value, _ := signing.Generate()
	res := &fakeRes{vals: map[string][]byte{signing.KeyRef: value, signing.KeyIDRef: []byte(old.ID)}}
	st := &fakeStore{res: res, setErrOn: signing.KeyIDName}
	_, err := signing.Rotate(context.Background(), res, st)
	if !signing.IsReason(err, signing.ReasonKeyIDMismatch) {
		t.Fatalf("%v", err)
	}
	// the new id comes first in the message, whole
	newKey, _ := signing.LoadKeyIgnoringStoredID(context.Background(), res)
	if !strings.Contains(err.Error(), "new key "+newKey) {
		t.Fatalf("the new key id is not in the message: %v", err)
	}
	// the key now fails to load: stored id is stale
	if _, err := signing.Public(context.Background(), res, "x"); !signing.IsReason(err, signing.ReasonKeyIDMismatch) {
		t.Fatalf("%v", err)
	}
	// rotating again repairs it
	st.setErrOn = ""
	if _, err := signing.Rotate(context.Background(), res, st); err != nil {
		t.Fatalf("rotate does not repair a stale stored id: %v", err)
	}
	if _, err := signing.Public(context.Background(), res, "x"); err != nil {
		t.Fatalf("still broken after the repairing rotation: %v", err)
	}
}

func TestConcurrentInitsDoNotBothSucceed(t *testing.T) {
	res := &fakeRes{vals: map[string][]byte{}}
	st := &fakeStore{res: res}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = signing.Init(context.Background(), res, st)
		}()
	}
	wg.Wait()
	ok := 0
	for _, e := range errs {
		if e == nil {
			ok++
		} else if !signing.IsReason(e, signing.ReasonSecretExists) {
			t.Fatalf("%v", e)
		}
	}
	if ok != 1 || len(st.sets) != 1 || st.locks != 2 {
		t.Fatalf("%d initializers succeeded, %d sets, %d locks", ok, len(st.sets), st.locks)
	}
}

func TestKeyManagementNeedsItsLock(t *testing.T) {
	res := &fakeRes{vals: map[string][]byte{}}
	st := &fakeStore{res: res, lockErr: errors.New("busy")}
	if _, err := signing.Init(context.Background(), res, st); err == nil || len(st.sets) != 0 {
		t.Fatalf("init without the lock: %v", err)
	}
	if _, err := signing.Rotate(context.Background(), res, st); err == nil || len(st.sets) != 0 {
		t.Fatalf("rotate without the lock: %v", err)
	}
}

func TestClientShapeChecks(t *testing.T) {
	f := newFixture(t)
	items := []signing.Item{item(signing.PayloadStatement, []byte("x"))}
	answer := func(sig sdk.EnvelopeSig) *fakeSigner {
		raw, _ := json.Marshal(signing.Response{Envelopes: []sdk.Envelope{{PayloadType: items[0].PayloadType, Payload: items[0].PayloadB64, Signatures: []sdk.EnvelopeSig{sig}}}})
		return &fakeSigner{ans: ansOf(string(raw))}
	}
	if _, err := clientFor(f, answer(fakeSig)).Sign(context.Background(), runID, items); err != nil {
		t.Fatal(err)
	}
	for name, sig := range map[string]sdk.EnvelopeSig{
		"empty key id and signature": {KeyID: "", Sig: ""},
		"empty signature":            {KeyID: fakeSig.KeyID, Sig: ""},
		"short signature":            {KeyID: fakeSig.KeyID, Sig: "AAAA"},
		"another purpose":            {KeyID: "plugins-0123456789abcdef", Sig: fakeSig.Sig},
		"short key id":               {KeyID: "ci-release-0123", Sig: fakeSig.Sig},
		"uppercase key id":           {KeyID: "ci-release-0123456789ABCDEF", Sig: fakeSig.Sig},
	} {
		if _, err := clientFor(f, answer(sig)).Sign(context.Background(), runID, items); !signing.IsReason(err, signing.ReasonSignerFailed) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := (&signing.Client{Runner: &fakeSigner{}}).Sign(context.Background(), runID, items); !signing.IsReason(err, signing.ReasonSignerFailed) {
		t.Fatalf("a client without a run store: %v", err)
	}
	if _, err := (&signing.Client{Src: f.src}).Sign(context.Background(), runID, items); !signing.IsReason(err, signing.ReasonSignerFailed) {
		t.Fatalf("a client without a runner: %v", err)
	}
}

func TestClientRelaysALongRefusalWhole(t *testing.T) {
	f := newFixture(t)
	msg := strings.Repeat("m", 200) + "END"
	out := `{"error":{"code":"E717","reason":"key_id_mismatch","message":"` + msg + `"}}`
	_, err := clientFor(f, &fakeSigner{ans: answer{out: out, code: 2}}).PublicKey(context.Background())
	if !signing.IsReason(err, signing.ReasonKeyIDMismatch) || !strings.Contains(err.Error(), "mmmEND") {
		t.Fatalf("%v", err)
	}
	long := `{"error":{"code":"E717","reason":"key_id_mismatch","message":"` + strings.Repeat("m", 600) + `"}}`
	_, err = clientFor(f, &fakeSigner{ans: answer{out: long, code: 2}}).PublicKey(context.Background())
	if err == nil || strings.Count(err.Error(), "m") > 520 {
		t.Fatalf("a relayed message was not bounded: %d", len(err.Error()))
	}
}

func ansOf(s string) answer { return answer{out: s} }

var _ = time.Second
