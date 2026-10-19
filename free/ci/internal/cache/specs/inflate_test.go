package specs

import (
	"bytes"
	"compress/gzip"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/detect"
)

// filler makes the deflate encoder Huffman-code the block, so the value is not stored as plain bytes.
var filler = strings.Repeat("the quick brown fox jumps over the lazy dog 0123456789\n", 400)

var npmFacts = []detect.Fact{
	{Kind: "package_manager", Name: "npm", Path: "package.json", Confidence: detect.Certain},
	{Kind: "lockfile", Name: "npm", Path: "package-lock.json", SHA256: strings.Repeat("b2", 32), Confidence: detect.Certain},
}

func gzipMembers(t *testing.T, parts ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	for _, p := range parts {
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write([]byte(p)); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return buf.Bytes()
}

// npmSave runs an npm attempt that leaves the given files in _cacache and finalizes it.
func npmSave(t *testing.T, e *env, id string, files map[string][]byte) []string {
	t.Helper()
	e.running(id, ownerFacts)
	in := e.input(id, ownerFacts)
	in.Detected = npmFacts
	s := e.begin(in)
	for name, b := range files {
		putFile(t, filepath.Join(s.dir("cache"), "_cacache", name), string(b))
	}
	s.Finalize(e.ctx, e.finalizing(id))
	return resultsOf(e.uses(id))
}

func TestNpmSaveRefusesARegisteredValueInsideAGzipPayload(t *testing.T) {
	cases := map[string][]byte{
		"one member":    gzipMembers(t, filler+"module.exports = '"+fakeSecret+"'"+filler),
		"second member": gzipMembers(t, filler, "token="+fakeSecret+filler),
	}
	for name, payload := range cases {
		e := newEnv(t)
		if err := e.scan.Register([]byte(fakeSecret)); err != nil {
			t.Fatal(err)
		}
		got := strings.Join(npmSave(t, e, "n1", map[string][]byte{"content-v2/sha512/ab/cd": payload}), ",")
		if got != "npm:miss,npm:refused" || e.entryCount() != 0 || e.auditActions("cache.save.refused") != 1 {
			t.Errorf("%s: uses %s, entries %d, refusals %d", name, got, e.entryCount(), e.auditActions("cache.save.refused"))
		}
	}
}

func TestNpmSaveKeepsACleanOrCorruptGzipPayload(t *testing.T) {
	e := newEnv(t)
	if err := e.scan.Register([]byte(fakeSecret)); err != nil {
		t.Fatal(err)
	}
	clean := gzipMembers(t, strings.Repeat("clean tarball bytes ", 1000))
	corrupt := append(gzipMembers(t, "clean"), 0x1f, 0x8b, 0x08, 0x00, 0xff, 0xff)
	got := strings.Join(npmSave(t, e, "n2", map[string][]byte{"content-v2/sha512/aa/bb": clean, "content-v2/sha512/cc/dd": corrupt}), ",")
	if got != "npm:miss,npm:saved" || e.entryCount() != 1 {
		t.Fatalf("uses %s, entries %d", got, e.entryCount())
	}
}
