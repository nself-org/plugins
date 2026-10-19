package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

var relDigest = strings.Repeat("ab", 32)

func relRow(state string) ReleaseRow {
	r := ReleaseRow{RunID: "run-1", Project: "proj", Revision: strings.Repeat("a", 40), State: state}
	if state == ReleasePublished {
		r.ManifestSHA256, r.KeyID = relDigest, "ci-release-0123456789abcdef"
	}
	return r
}

func TestCacheReleasesMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	var version int
	for i := 0; i < 2; i++ {
		s, err := Open(path, Options{})
		if err != nil {
			t.Fatal(err)
		}
		var tables, logged, v int
		must := func(err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
		}
		must(s.readers.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='cache_release'").Scan(&tables))
		must(s.readers.QueryRow("SELECT count(*) FROM migration_log WHERE set_name='cache-releases' AND filename='0001_releases.sql'").Scan(&logged))
		must(s.readers.QueryRow("SELECT schema_version FROM schema_meta").Scan(&v))
		if tables != 1 || logged != 1 {
			t.Fatalf("pass %d: table %d, logged %d", i, tables, logged)
		}
		if i == 1 && v != version {
			t.Fatalf("reopen changed schema_version %d -> %d", version, v)
		}
		version = v
		_ = s.Close()
	}
}

func TestCacheReleasesStates(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if _, ok, err := s.ReleaseFor(ctx, "run-1"); ok || err != nil {
		t.Fatalf("a missing release read as %v %v", ok, err)
	}
	// failed -> awaiting -> failed -> published: any move before publication
	failed := relRow(ReleaseFailed)
	failed.Reason = "E717 key_missing: no release key"
	for _, r := range []ReleaseRow{failed, relRow(ReleaseAwaiting), failed, relRow(ReleasePublished)} {
		if err := s.RecordRelease(ctx, r); err != nil {
			t.Fatalf("%s: %v", r.State, err)
		}
	}
	got, ok, err := s.ReleaseFor(ctx, "run-1")
	if err != nil || !ok || got.State != ReleasePublished || got.ManifestSHA256 != relDigest || got.CreatedAt == 0 || got.UpdatedAt < got.CreatedAt {
		t.Fatalf("published row %+v %v %v", got, ok, err)
	}
	// recovery is idempotent: the same publication again changes nothing
	if err := s.RecordRelease(ctx, relRow(ReleasePublished)); err != nil {
		t.Fatalf("repeat: %v", err)
	}
	again, _, _ := s.ReleaseFor(ctx, "run-1")
	if again != got {
		t.Fatalf("a repeat changed the row: %+v -> %+v", got, again)
	}
	// a published row never moves, not even by SQL
	other := relRow(ReleasePublished)
	other.ManifestSHA256 = strings.Repeat("cd", 32)
	for _, r := range []ReleaseRow{other, failed, relRow(ReleaseAwaiting)} {
		if err := s.RecordRelease(ctx, r); err == nil {
			t.Fatalf("a published release accepted %+v", r)
		}
	}
	raw := s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE cache_release SET state='failed' WHERE run_id='run-1'")
		return err
	})
	del := s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM cache_release WHERE run_id='run-1'")
		return err
	})
	if del == nil {
		t.Fatal("the trigger let a published row be deleted")
	}
	if raw == nil {
		t.Fatal("the trigger let a published row change")
	}
	// shape checks
	bad := []ReleaseRow{
		{RunID: "", Project: "p", Revision: "r", State: ReleaseFailed},
		{RunID: "r2", Project: "p", Revision: "r", State: "done"},
		{RunID: "r2", Project: "p", Revision: "r", State: ReleasePublished},
		{RunID: "r2", Project: "p", Revision: "r", State: ReleaseFailed, KeyID: "k"},
		{RunID: "r2", Project: "p", Revision: "r", State: ReleaseFailed, Reason: "line\nbreak"},
	}
	for i, r := range bad {
		if err := s.RecordRelease(ctx, r); err == nil {
			t.Fatalf("bad row %d accepted", i)
		}
	}
	// another project's row id is refused
	moved := relRow(ReleaseFailed)
	moved.RunID, moved.Project = "run-2", "proj"
	if err := s.RecordRelease(ctx, moved); err != nil {
		t.Fatal(err)
	}
	moved.Project = "elsewhere"
	if err := s.RecordRelease(ctx, moved); err == nil {
		t.Fatal("a record moved to another project")
	}
	// an over-long reason is cut, not refused
	long := relRow(ReleaseFailed)
	long.RunID, long.Reason = "run-3", strings.Repeat("é", 600)
	if err := s.RecordRelease(ctx, long); err != nil {
		t.Fatal(err)
	}
	if r, _, _ := s.ReleaseFor(ctx, "run-3"); len(r.Reason) == 0 || len(r.Reason) > maxReleaseReason {
		t.Fatalf("reason length %d", len(r.Reason))
	}
}
