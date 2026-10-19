package flaky

// D-1008: a finishing run compares itself only with runs of its own revision
// (groupKey names the revision), so the history scan skips the others instead of
// decoding and verifying their evidence.

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/run"
)

func TestScanDocsReadsOnlyTheRevisionAsked(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	const other = "fedcba9876543210fedcba9876543210fedcba98"
	runJob(t, root, home, testSHA, "exit 0")
	runJob(t, root, home, other, "exit 0")
	runJob(t, root, home, testSHA, "exit 0")
	st, err := run.OpenStore(filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	all, err := scanDocs(ctx, st, 50, "")
	if err != nil || len(all) != 3 {
		t.Fatalf("a scan without a revision reads every run: %d docs, %v", len(all), err)
	}
	only, err := scanDocs(ctx, st, 50, testSHA)
	if err != nil {
		t.Fatal(err)
	}
	if len(only) != 2 {
		t.Fatalf("a scan for %s read %d docs, want the 2 of that revision", testSHA, len(only))
	}
	for _, d := range only {
		if d.Evidence.Revision != testSHA {
			t.Fatalf("doc of revision %s read for %s", d.Evidence.Revision, testSHA)
		}
	}
}
