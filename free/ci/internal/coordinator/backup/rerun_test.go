package backup

// Purpose: the re-run guarantees of restore (D-1307, P7-NODE-20 follow-up): a re-run of a half-finished
//          restore prints the rollback line of the FIRST run (the original files), and a failure of the
//          database image before anything was replaced leaves no marker.
// Inputs:  the shared rig. Outputs: pass or fail; no network.
// Constraints: synthetic keys only; the post-swap failure is a read-only pki dir, as in round3_test.go.

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestRerunPrintsTheFirstRunsRollbackLine(t *testing.T) {
	ctx := context.Background()
	r, arch := seeded(t)
	_ = r.st.Close()
	pkiDir := filepath.Join(r.dir, "pki")
	if err := os.Chmod(pkiDir, 0o500); err != nil { // the KEK install after the swap must fail
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(pkiDir, 0o700) }()
	o := r.opts()
	first, err := Restore(ctx, o, bytes.NewReader(arch), r.kek)
	if err == nil {
		t.Skip("the platform let the restore write into a read-only directory")
	}
	if !strings.Contains(first.Rollback, "state.db.pre-restore-") {
		t.Fatalf("run 1 rollback line %q does not name the saved database", first.Rollback)
	}
	if err = os.Chmod(pkiDir, 0o700); err != nil {
		t.Fatal(err)
	}
	r.clk.t = r.clk.t.Add(time.Minute)
	second, err := Restore(ctx, o, bytes.NewReader(arch), r.kek)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if second.Rollback != first.Rollback {
		t.Fatalf("the re-run printed %q; the original files are at %q", second.Rollback, first.Rollback)
	}
	copies, _ := filepath.Glob(filepath.Join(r.dir, "state.db.pre-restore-*"))
	if len(copies) != 1 {
		t.Fatalf("a re-run saved the half-restored database too: %v", copies)
	}
}

func TestRefusedImageBeforeTheSwapLeavesNoMarker(t *testing.T) {
	ctx := context.Background()
	r, arch := seeded(t)
	_ = r.st.Close()
	m, es, err := Read(bytes.NewReader(arch), r.kek)
	if err != nil {
		t.Fatal(err)
	}
	for i := range es {
		if es[i].Name == nameState {
			es[i].Data = []byte("this is not a database image")
		}
	}
	var bad bytes.Buffer
	if err = Write(&bad, r.kek, m, es); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(r.dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Restore(ctx, r.opts(), &bad, r.kek); err == nil {
		t.Fatal("an invalid database image was meant to be refused")
	}
	if Pending(r.dir) {
		t.Fatal("the image was refused before the rename; nothing changed, so no restore.pending may stay")
	}
	after, err := os.ReadFile(filepath.Join(r.dir, "state.db"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("state.db changed although the image was refused (%v)", err)
	}
}

func TestStoreRestoreTypesItsPreRenameFailures(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	_, err = st.Restore(context.Background(), strings.NewReader("garbage"))
	if !errors.Is(err, store.ErrNotReplaced) {
		t.Fatalf("err %v: a refused image must be store.ErrNotReplaced", err)
	}
}
