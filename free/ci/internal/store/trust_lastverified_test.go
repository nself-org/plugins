package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

const lvJob, lvPrefix = "drill-isolation-levels", "passed: "

// lvDoc is a run document whose drill check carries excerpt; finished may be "" (no timing).
func lvDoc(job, kind, excerpt, finished, platform string) []byte {
	timing := `null`
	if finished != "" {
		timing = `"` + finished + `"`
	}
	return []byte(fmt.Sprintf(`{"revision":"rev-%s","runner":{"platform":%q},"timings":{"finished_at":%s},"checks":[{"job_id":%q,"kind":%q,"excerpt":%q}]}`,
		finished, platform, timing, job, kind, excerpt))
}

func lvStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{Clock: func() time.Time { now = now.Add(time.Second); return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	seed(t, s, "1")
	return s, &now
}

func lvRecord(t *testing.T, s *Store, id string, body []byte) {
	t.Helper()
	if err := s.RecordRunEvidence(context.Background(), id, "p1", "rev", "i", body); err != nil {
		t.Fatal(err)
	}
}

func TestLastVerifiedNewestPassPerCheck(t *testing.T) {
	s, _ := lvStore(t)
	lvRecord(t, s, "e1", lvDoc(lvJob, "drill", "nested evidence: pass\npassed: isolation.container isolation.network", "2026-10-02T03:00:00Z", "linux/amd64"))
	lvRecord(t, s, "e2", lvDoc(lvJob, "drill", "nested evidence: fail\n  fail isolation.network: x\npassed: isolation.container", "2026-10-03T03:00:00Z", "linux/arm64"))
	got, err := s.LastVerified(context.Background(), lvJob, lvPrefix, []string{"isolation.container", "isolation.network", "isolation.vm"})
	if err != nil {
		t.Fatal(err)
	}
	c, n := got["isolation.container"], got["isolation.network"]
	if c.Revision != "rev-2026-10-03T03:00:00Z" || c.Platform != "linux/arm64" || !c.FinishedAt.Equal(time.Date(2026, 10, 3, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("container: %+v", c)
	}
	if n.Platform != "linux/amd64" || n.FinishedAt.Day() != 2 {
		t.Errorf("network keeps its own newest pass, not the later failed run: %+v", n)
	}
	if _, ok := got["isolation.vm"]; ok || len(got) != 2 {
		t.Errorf("a check that never passed is absent: %+v", got)
	}
}

func TestLastVerifiedNeverReportsWithoutAPassLine(t *testing.T) {
	s, _ := lvStore(t)
	for i, doc := range [][]byte{
		lvDoc(lvJob, "drill", "nested evidence: fail\n  fail isolation.container: broke", "2026-10-02T03:00:00Z", "linux/amd64"),    // failed, not listed as passed
		lvDoc("other-job", "drill", "passed: isolation.container", "2026-10-03T03:00:00Z", "linux/amd64"),                           // another job's check
		lvDoc(lvJob, "test", "passed: isolation.container", "2026-10-04T03:00:00Z", "linux/amd64"),                                  // not a drill check
		lvDoc(lvJob, "drill", "xpassed: isolation.container\n  passed: isolation.container", "2026-10-05T03:00:00Z", "linux/amd64"), // prefix must start the line
		lvDoc(lvJob, "drill", "passed: isolation.container-extra isolation.container.x", "2026-10-06T03:00:00Z", "linux/amd64"),     // exact ids only
		[]byte(`not json drill-isolation-levels`), // undecodable
		[]byte(`{"checks":[{"job_id":"drill-isolation-levels","kind":"drill","excerpt":"passed: isolation.container"}],"timings":{"finished_at":7}}`), // wrong type for a time
	} {
		lvRecord(t, s, fmt.Sprintf("e%d", i), doc)
	}
	got, err := s.LastVerified(context.Background(), lvJob, lvPrefix, []string{"isolation.container"})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v, err %v; want nothing verified", got, err)
	}
}

func TestLastVerifiedIgnoresAttemptEvidenceAndFallsBackToStoreTime(t *testing.T) {
	s, _ := lvStore(t)
	ctx := context.Background()
	if err := s.RecordEvidence(ctx, "att", "p1", "1", "rev", "i", lvDoc(lvJob, "drill", "passed: isolation.container", "2026-10-09T03:00:00Z", "linux/amd64")); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LastVerified(ctx, lvJob, lvPrefix, []string{"isolation.container"}); len(got) != 0 {
		t.Fatalf("attempt-level evidence must not count: %+v", got)
	}
	lvRecord(t, s, "e1", lvDoc(lvJob, "drill", "passed: isolation.container", "", "linux/amd64"))
	got, err := s.LastVerified(ctx, lvJob, lvPrefix, []string{"isolation.container"})
	if err != nil || got["isolation.container"].FinishedAt.IsZero() || got["isolation.container"].FinishedAt.Year() != 2026 {
		t.Fatalf("a document with no finish time uses the store time: %+v, %v", got, err)
	}
}
