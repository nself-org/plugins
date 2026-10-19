package publish_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// failingLedger fails the write of a published row.
type failingLedger struct{ publish.Ledger }

func (failingLedger) RecordRelease(_ context.Context, r store.ReleaseRow) error {
	if r.State == store.ReleasePublished {
		return errors.New("disk full at /private/path")
	}
	return nil
}

func TestPublishConcurrentCallsOnePublication(t *testing.T) {
	f := newFixture(t)
	var wg sync.WaitGroup
	results := make([]publish.Release, 4)
	errs := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = publish.Run(context.Background(), f.deps(), runID)
		}()
	}
	wg.Wait()
	for i := range results {
		if errs[i] != nil || results[i].State != publish.Published || results[i].ManifestSHA256 != results[0].ManifestSHA256 {
			t.Fatalf("call %d: %+v %v", i, results[i], errs[i])
		}
	}
	// the lock serialises them: the first publishes, the others find it intact and sign nothing
	if f.signer.signs != 2 {
		t.Fatalf("signer was asked %d times for one release (want 2: statement and manifest)", f.signer.signs)
	}
	if ents, _ := os.ReadDir(filepath.Join(f.home, "releases")); len(ents) != 2 { // the run directory and its lock file
		t.Fatalf("leftovers: %v", ents)
	}
}

func TestPublishRestoresTheDirectoryWhenTheLedgerFails(t *testing.T) {
	f := newFixture(t)
	first := mustRun(t, f)
	damaged := filepath.Join(first.Dir, "SHA256SUMS")
	if err := os.WriteFile(damaged, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := readDir(t, first.Dir)
	d := f.deps()
	d.Ledger = failingLedger{f.ledger}
	_, err := publish.Run(context.Background(), d, runID)
	if err == nil || strings.Contains(err.Error(), "/private/path") {
		t.Fatalf("error %v", err)
	}
	if !bytesEqualMaps(readDir(t, first.Dir), before) {
		t.Fatal("the directory that was replaced was not put back")
	}
	if ents, _ := os.ReadDir(filepath.Join(f.home, "releases")); len(ents) != 2 {
		t.Fatalf("leftovers: %v", ents)
	}
}

func TestPublishFailureTextHoldsNothingFromOutside(t *testing.T) {
	f := newFixture(t)
	key := f.res.vals[signing.KeyRef]
	pem, _ := base64.StdEncoding.DecodeString(string(key))
	var sinks []string
	grep := func(where, text string) {
		t.Helper()
		if strings.Contains(text, string(key)) || (len(pem) > 0 && strings.Contains(text, string(pem))) || strings.Contains(text, f.home) {
			t.Fatalf("%s holds the key or a private path: %q", where, text)
		}
		sinks = append(sinks, text)
	}
	// a failure: the stored bytes of an artifact changed
	f.blobs.data[sum(f.files["app"])] = []byte("tampered")
	_, err := publish.Run(context.Background(), f.deps(), runID)
	if err == nil {
		t.Fatal("tampered bytes published")
	}
	grep("error", err.Error())
	row, _, _ := f.ledger.ReleaseFor(context.Background(), runID)
	grep("ledger reason", row.Reason)
	// a success
	f.blobs.data[sum(f.files["app"])] = f.files["app"]
	rel := mustRun(t, f)
	shown := rel
	shown.Dir = "" // the release directory is the one path the command prints on purpose
	raw, _ := json.Marshal(shown)
	grep("release json", string(raw))
	for name, b := range readDir(t, rel.Dir) {
		if strings.Contains(string(b), string(key)) || (len(pem) > 0 && strings.Contains(string(b), string(pem))) {
			t.Fatalf("%s holds the key", name)
		}
	}
	if row, _, _ = f.ledger.ReleaseFor(context.Background(), runID); row.State != store.ReleasePublished {
		t.Fatalf("ledger %+v", row)
	}
	grep("ledger key id", row.KeyID+row.ManifestSHA256)
	if len(sinks) < 4 {
		t.Fatal("sinks were not collected")
	}
}

func TestPublishedReleaseIsNotReportedAsUnpublished(t *testing.T) {
	f := newFixture(t)
	key := f.res.vals[signing.KeyRef]
	first := mustRun(t, f)
	delete(f.res.vals, signing.KeyRef) // the signer cannot answer any more
	_, err := publish.Run(context.Background(), f.deps(), runID)
	if err == nil || strings.Contains(err.Error(), "NOT published") || !strings.Contains(err.Error(), "recorded as published") {
		t.Fatalf("error %v", err)
	}
	if row, _, _ := f.ledger.ReleaseFor(context.Background(), runID); row.State != store.ReleasePublished || row.ManifestSHA256 != first.ManifestSHA256 {
		t.Fatalf("the published row changed: %+v", row)
	}
	f.res.vals[signing.KeyRef] = key
	// a TEAM coordinator leaves a published release alone
	d := f.deps()
	d.Team = true
	rel, err := publish.Run(context.Background(), d, runID)
	if err != nil || rel.State != publish.Published {
		t.Fatalf("team on a published release: %+v %v", rel, err)
	}
}

func TestPublishUnknownRunHasNoRecovery(t *testing.T) {
	f := newFixture(t)
	_, err := publish.Run(context.Background(), f.deps(), "no-such-run")
	if err == nil || strings.Contains(err.Error(), "artifacts sign") {
		t.Fatalf("error %v", err)
	}
	for _, bad := range []string{"", "..", "a/b", "a\\b"} {
		if _, err = publish.Run(context.Background(), f.deps(), bad); err == nil {
			t.Fatalf("run id %q accepted", bad)
		}
		if _, err = publish.Export(context.Background(), f.deps(), bad, filepath.Join(t.TempDir(), "o"), t.TempDir()); err == nil {
			t.Fatalf("export of run id %q accepted", bad)
		}
	}
}

func TestExportRefusesADirectoryThatIsNotTheLedgersRelease(t *testing.T) {
	f := newFixture(t)
	rel := mustRun(t, f)
	if err := os.WriteFile(filepath.Join(rel.Dir, "release-manifest.dsse.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "o")
	if _, err := publish.Export(context.Background(), f.deps(), runID, out, t.TempDir()); err == nil || !strings.Contains(err.Error(), "ledger") {
		t.Fatalf("export: %v", err)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("a refused export created the destination")
	}
}

func TestStaleStagingIsSwept(t *testing.T) {
	f := newFixture(t)
	releases := filepath.Join(f.home, "releases")
	stale := filepath.Join(releases, ".stage-dead")
	fresh := filepath.Join(releases, ".stage-live")
	for _, d := range []string{stale, fresh} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-3 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	mustRun(t, f)
	if _, err := os.Stat(stale); err == nil {
		t.Fatal("a stale staging directory survived")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("a fresh staging directory (maybe another process's) was removed")
	}
}
