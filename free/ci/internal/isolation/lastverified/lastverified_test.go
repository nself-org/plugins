package lastverified

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/drill"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type fakeSource struct {
	got map[string]store.Verification
	err error
}

func (f fakeSource) LastVerified(_ context.Context, job, prefix string, _ []string) (map[string]store.Verification, error) {
	if job != Job || prefix != PassedPrefix {
		return nil, errors.New("wrong query")
	}
	return f.got, f.err
}

func byLevel(rs []Record) map[Level]Record {
	m := map[Level]Record{}
	for _, r := range rs {
		m[r.Level] = r
	}
	return m
}

func TestReadVerifiedUnverifiedUnavailable(t *testing.T) {
	at := time.Date(2026, 10, 3, 3, 0, 0, 0, time.UTC)
	src := fakeSource{got: map[string]store.Verification{
		"isolation.container": {FinishedAt: at, Revision: "0123456789abcdef", Platform: "linux/amd64"},
		"isolation.vm":        {FinishedAt: at, Platform: "linux/amd64"}, // stale record from another host class: still unavailable on Linux
	}}
	recs, err := Read(context.Background(), src, "linux")
	if err != nil {
		t.Fatal(err)
	}
	m := byLevel(recs)
	if got := m[Container].Message(); got != "verified 2026-10-03 on linux/amd64 (revision 0123456)" {
		t.Errorf("container: %q", got)
	}
	if m[SandboxedContainer].Message() != "unverified since never" || m[Network].Status != Unverified || m[Hygiene].Status != Unverified {
		t.Errorf("a level with no pass reads unverified: %+v", recs)
	}
	for _, l := range []Level{VM, EphemeralVM} {
		if m[l].Status != Unavailable || m[l].Message() != "unavailable (no KVM host, D-0120)" {
			t.Errorf("%s on Linux: %+v", l, m[l])
		}
	}
}

func TestReadOnMacHasNoEphemeralVMAndVMFollowsEvidence(t *testing.T) {
	recs, _ := Read(context.Background(), fakeSource{got: map[string]store.Verification{"isolation.vm": {FinishedAt: time.Now(), Platform: "darwin/arm64"}}}, "darwin")
	m := byLevel(recs)
	if _, ok := m[EphemeralVM]; ok || len(recs) != 5 {
		t.Errorf("levels on darwin: %+v", recs)
	}
	if m[VM].Status != Verified || m[Container].Status != Unverified {
		t.Errorf("darwin: %+v", recs)
	}
}

func TestReadSourceFailureNeverVerifies(t *testing.T) {
	src := fakeSource{got: map[string]store.Verification{"isolation.container": {FinishedAt: time.Now()}}, err: errors.New("boom")}
	recs, err := Read(context.Background(), src, "linux")
	if err == nil {
		t.Fatal("the source error is returned")
	}
	for _, r := range recs {
		if r.Status == Verified {
			t.Errorf("verified on a failed read: %+v", r)
		}
	}
	nilRecs, nilErr := Read(context.Background(), nil, "linux")
	if nilErr != nil || byLevel(nilRecs)[Container].Status != Unverified {
		t.Errorf("no store: %+v %v", nilRecs, nilErr)
	}
}

// The drill writes the passed line and the store reads it back: this pins the two ends together.
func TestDrillExcerptFormatMatchesTheStoreReader(t *testing.T) {
	if drill.PassedPrefix != PassedPrefix {
		t.Fatalf("drill writes %q, the reader looks for %q", drill.PassedPrefix, PassedPrefix)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	doc := `{"schema":"ci.evidence/v1","revision":"0123456789abcdef0123456789abcdef01234567","repo_id":"local:0","run_id":"r","pipeline":{"id":"drills","name":"drills","kind":"drill"},"trigger":"manual","source_trust":"owner","policy_digest":"0","input_digests":{"config":"0","lockfiles":[],"generated":[]},"worktree_state":"clean","tracked_changes_digest":null,"binding":{"state":"bound","reason":null},"runner":{"id":"h","provider":"local","platform":"linux/amd64","isolation":"process"},"producer":{"kind":"self-reported","version":"x/1"},"timings":{"queued_at":null,"started_at":null,"finished_at":"2026-10-03T03:00:00Z","duration_ms":1},"tool_versions":{},"checks":[` +
		`{"id":"isolation.container","job_id":"i","kind":"drill","tool":"bash","tool_version":null,"required":true,"substantive":true,"result":"pass","findings":0,"excerpt":"ok","log_ref":null},` +
		`{"id":"isolation.network","job_id":"i","kind":"drill","tool":"bash","tool_version":null,"required":true,"substantive":true,"result":"fail","findings":1,"excerpt":"no","log_ref":null}],` +
		`"artifacts":[],"retries":[],"selection":{"mode":"full","reason":"selection.full_required","digest":"0"},"result":"fail","counts":{"pass":1,"fail":1,"skip":0,"error":0,"total":2},"freshness":{"state":"current","reason":null},"signature":{"kind":"coordinator-recorded"}}`
	ch, known := drill.Judge(doc+"\n", false)
	if !known || !strings.Contains(ch.Excerpt, "isolation.network") {
		t.Fatalf("judge: %+v", ch)
	}
	excerpt, _ := json.Marshal(ch.Excerpt)
	run := `{"source_trust":"owner","revision":"0123456789abcdef0123456789abcdef01234567","runner":{"platform":"linux/amd64"},"timings":{"finished_at":"2026-10-04T03:00:00Z"},"checks":[{"job_id":"` + Job + `","kind":"drill","excerpt":` + string(excerpt) + `}]}`
	if err = st.CreatePipeline(ctx, store.PipelineRow{ID: "p", Revision: "r", Project: "x", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	if err = st.RecordRunEvidence(ctx, "e", "p", "r", "i", []byte(run)); err != nil {
		t.Fatal(err)
	}
	recs, err := Read(ctx, st, "linux")
	if err != nil {
		t.Fatal(err)
	}
	m := byLevel(recs)
	if m[Container].Status != Verified || m[Container].HostClass != "linux/amd64" || m[Network].Status != Unverified {
		t.Fatalf("the failed nested check must not verify: %+v", recs)
	}
}
