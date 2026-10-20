//go:build cachesec

package cachesec

// Purpose: TM-16 cases on the local carrier: a registered secret value smuggled into a cache entry
// at save time (raw, base64, and inside a file name) is refused by specs.Engine.Save with E711
// secret_match, an audit row (cache.save.refused) and no entry and no blob stored.
// Inputs: a real store, cas and specs.Engine with a redact.Redactor holding one fake value.
// Outputs: registered cases C16-CAC-01..03.
// Constraints: production packages only; the value is the obviously fake SYNTHETIC_TOKEN below. The
// positive control saves an honest directory of the same attempt first. Landed counts the entry
// bytes added by the attack; the case also fails when the blob store gained a file (the scan aborts
// the stream before a blob is visible).

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

const g1Fake = "SYNTHETIC_TOKEN_aaaa1111bbbb2222"

// g1BlobFiles counts regular files in the blob store under home.
func g1BlobFiles(t *testing.T, home string) (n int) {
	t.Helper()
	_ = filepath.WalkDir(filepath.Join(home, "blobs"), func(p string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			n++
		}
		return nil
	})
	return n
}

// g1Smuggle saves an honest dir (control) and then a dir built by put, with the fake value registered.
func g1Smuggle(t *testing.T, put func(dir string)) Observed {
	t.Helper()
	w := newWorld(t)
	rd := redact.New()
	if err := rd.Register([]byte(g1Fake)); err != nil {
		t.Fatal(err)
	}
	eng := &specs.Engine{Entries: w.Es, Blobs: w.Blobs, Scanner: rd, Backend: w.St}
	root := t.TempDir()
	for _, d := range []string{"ok", "bad"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "ok", "f.txt"), []byte("honest module bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	put(filepath.Join(root, "bad"))
	base, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = base.Close() }()

	ns := w.NS(g1PR1)
	kOK, kBad := w.Key(g1PR1, "ok"), w.Key(g1PR1, "bad")
	fin := w.Attempt("s1", g1PR1, []string{kOK, kBad}, true)
	req := func(key, dir string) specs.SaveRequest {
		return specs.SaveRequest{Base: base, Attempt: "s1", Epoch: fin, Namespace: ns, Key: key, Spec: specs.Spec{ID: "go"}, Dirs: specs.Dirs{"mod": dir}}
	}
	if _, err := eng.Save(w.ctx, req(kOK, "ok")); err != nil { // positive control
		t.Fatalf("control: the honest save was refused: %v", err)
	}
	baseBytes, baseBlobs := w.EntryBytes(), g1BlobFiles(t, w.Home)

	_, err = eng.Save(w.ctx, req(kBad, "bad"))
	got := refusal(t, err, g1ReasonE711(t))
	got.Audit = w.AuditOf("cache.save.refused")
	rows := w.Audit("cache.save.refused")
	if len(rows) != 1 || !strings.Contains(rows[0].Detail, "reason=secret_match") || strings.Contains(rows[0].Detail, g1Fake) {
		t.Fatalf("audit rows %+v: want one secret_match row that does not carry the value", rows)
	}
	if n := g1BlobFiles(t, w.Home); n != baseBlobs {
		t.Fatalf("the blob store holds %d files, had %d: a blob of the refused save was left", n, baseBlobs)
	}
	if w.g1Visible(kBad) != 0 {
		t.Fatal("the refused entry is visible")
	}
	got.Landed = w.EntryBytes() - baseBytes
	return got
}

func init() {
	register("C16-CAC-01", func(t *testing.T) Observed {
		return g1Smuggle(t, func(dir string) {
			g1Write(t, filepath.Join(dir, "token.txt"), "key="+g1Fake+"\n")
		})
	})
	register("C16-CAC-02", func(t *testing.T) Observed {
		return g1Smuggle(t, func(dir string) {
			g1Write(t, filepath.Join(dir, "blob.b64"), base64.StdEncoding.EncodeToString([]byte(g1Fake)))
		})
	})
	register("C16-CAC-03", func(t *testing.T) Observed { // the value as a file name (a tar header, not file content)
		return g1Smuggle(t, func(dir string) {
			g1Write(t, filepath.Join(dir, g1Fake+".txt"), "innocent bytes")
		})
	})
}

func g1Write(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
