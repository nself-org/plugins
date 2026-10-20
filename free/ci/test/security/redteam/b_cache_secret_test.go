//go:build security && unix

package redteam

// Purpose: red team attacks on secrets smuggled into a cache entry (TM-16, store half): the real
// specs.Engine packs a directory and scans it; the attack hides a registered value in raw, encoded,
// compressed and window-straddling forms.
// Inputs:  a temp cache root with one file per case; the real redact.Redactor with fakeSecret.
// Outputs: pass or fail; a pass means the secret did not reach a committed entry.
// Constraints: local only; no spec in this test sets InflateScan, like every ecosystem but npm.

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

var planSpec = specs.Spec{ID: "go", Version: "1", Dirs: []specs.Dir{{Name: "mod"}}}

// saveWith writes content as the only file of the cache root and runs a whole attempt's Save.
// It returns the Save error.
func saveWith(t *testing.T, id string, files map[string][]byte) error {
	t.Helper()
	e := newCacheEnv(t)
	red := redact.New()
	if err := red.Register([]byte(fakeSecret)); err != nil {
		t.Fatal(err)
	}
	key := e.key(factsMain, id)
	e.start(id, factsMain)
	e.finish(id, factsMain, []string{key}, true)
	root := t.TempDir()
	for name, b := range files {
		p := filepath.Join(root, "mod", name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = base.Close() }()
	eng := &specs.Engine{Entries: e.es, Blobs: e.blobs, Scanner: red, Backend: e.st}
	_, err = eng.Save(e.ctx, specs.SaveRequest{Base: base, Attempt: id, Epoch: 3, Namespace: e.ns(factsMain), Key: key, Spec: planSpec, Dirs: specs.Dirs{"mod": "mod"}})
	if err == nil && !e.lookup(factsMain, key, authz.ModeRWOwn) {
		t.Fatalf("save reported success but nothing is committed")
	}
	return err
}

func gzBytes(s string) []byte {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func zipBytes(s string) []byte {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, _ := w.Create("inner.txt")
	_, _ = f.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func TestB_CacheEntry_SecretForms(t *testing.T) {
	v := fakeSecret
	// forms the raw tar scan can see: all must be refused
	for name, content := range map[string]string{
		"raw":        "x" + v + "y",
		"base64":     base64.StdEncoding.EncodeToString([]byte(v)),
		"base64-off": "pad" + base64.StdEncoding.EncodeToString([]byte("zz"+v)),
		"hex":        hex.EncodeToString([]byte(v)),
	} {
		err := saveWith(t, "s-"+name, map[string][]byte{"f.txt": []byte(content)})
		if !errors.Is(err, specs.ErrSecretMatch) && err == nil {
			t.Errorf("%s: a cache entry holding the secret was committed", name)
		}
	}
	// the control: a clean file saves
	if err := saveWith(t, "s-clean", map[string][]byte{"f.txt": []byte("nothing")}); err != nil {
		t.Fatalf("clean save failed: %v", err)
	}
}

// filler makes deflate use Huffman codes, so the secret is no longer a byte-aligned literal.
var filler = strings.Repeat("lorem ipsum dolor sit amet, consectetur adipiscing elit\n", 400)

func TestB_CacheEntry_SecretInsideCompressedFile(t *testing.T) {
	for name, content := range map[string][]byte{
		"gzip": gzBytes(filler + "token=" + fakeSecret + filler),
		"zip":  zipBytes(filler + "token=" + fakeSecret + filler),
	} {
		err := saveWith(t, "c-"+name, map[string][]byte{"dep." + name: content})
		t.Logf("%s: save result: %.100v", name, err)
		if err == nil {
			if !openFinding(t, "B08") {
				continue
			}
			t.Errorf("%s: a compressed file holding the secret was committed (spec has no InflateScan)", name)
		}
	}
}

func TestB_CacheEntry_SecretAcrossScanWindows(t *testing.T) {
	const window = 1 << 20
	for _, off := range []int{window - len(fakeSecret)/2, window - 1, window + 1, 2*window - 3, 4096 - 5, 512} {
		pad := bytes.Repeat([]byte("a"), off)
		content := append(append(pad, fakeSecret...), bytes.Repeat([]byte("b"), 100)...)
		if err := saveWith(t, "w-"+itoa(off), map[string][]byte{"big.bin": content}); err == nil {
			t.Errorf("offset %d: a secret straddling a scan window was committed", off)
		}
	}
}
