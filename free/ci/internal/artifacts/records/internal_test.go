package records

import (
	"bytes"
	"context"
	"errors"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

type needle string

func (n needle) Scan(b []byte) []authz.Match {
	var out []authz.Match
	for i := 0; ; {
		j := bytes.Index(b[i:], []byte(n))
		if j < 0 {
			return out
		}
		out = append(out, authz.Match{Offset: i + j, Len: len(n)})
		i += j + len(n)
	}
}

func drain(r io.Reader) ([]byte, error) { return io.ReadAll(r) }

func TestScanReaderWindowsAndOverlap(t *testing.T) {
	const secret = "SECRET-VALUE"
	pad := func(n int) string { return strings.Repeat("a", n) }
	cases := map[string]struct {
		data  string
		match bool
	}{
		"empty":                 {"", false},
		"small clean":           {"hello", false},
		"small match":           {"x" + secret + "y", true},
		"at the start":          {secret + pad(100), true},
		"straddles the window":  {pad(scanWindow-5) + secret + pad(50), true},
		"ends the first window": {pad(scanWindow-len(secret)) + secret, true},
		"starts the second":     {pad(scanWindow) + secret, true},
		"second window clean":   {pad(scanWindow) + "tail", false},
		"three windows":         {pad(2*scanWindow+10) + secret, true},
		"exact window":          {pad(scanWindow), false},
		"two exact windows":     {pad(2 * scanWindow), false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := newScanReader(strings.NewReader(c.data), needle(secret), "f", 1<<30)
			got, err := drain(r)
			if c.match {
				if !IsReason(err, ReasonSecretMatch) || len(got) >= len(c.data) {
					t.Fatalf("want a match, got %v after %d of %d bytes", err, len(got), len(c.data))
				}
				if strings.Contains(string(got), secret) {
					t.Fatal("the matched bytes were released")
				}
				return
			}
			if err != nil || string(got) != c.data {
				t.Fatalf("clean data altered: err %v, %d of %d bytes", err, len(got), len(c.data))
			}
		})
	}
	// bytes are released only after their window was scanned: the secret never appears even in released prefixes
	r := newScanReader(strings.NewReader(strings.Repeat("a", scanWindow)+secret), needle(secret), "f", 1<<30)
	buf := make([]byte, 10)
	n, err := r.Read(buf)
	if err != nil || n != 10 {
		t.Fatalf("first read %d %v", n, err)
	}
	// tiny reads still return every byte of a clean stream
	got, err := drain(iotest.OneByteReader(newScanReader(strings.NewReader("one byte at a time"), needle(secret), "f", 100)))
	if err != nil || string(got) != "one byte at a time" {
		t.Fatalf("%q %v", got, err)
	}
	// the cap is enforced while reading
	_, err = drain(newScanReader(strings.NewReader(strings.Repeat("b", 11)), needle(secret), "f", 10))
	if !IsReason(err, ReasonFileCap) {
		t.Fatalf("cap: %v", err)
	}
	if _, err = drain(newScanReader(strings.NewReader(strings.Repeat("b", 10)), needle(secret), "f", 10)); err != nil {
		t.Fatalf("exactly at the cap: %v", err)
	}
	// a read error and a nil scanner
	if _, err = drain(newScanReader(iotest.ErrReader(errors.New("boom")), needle(secret), "f", 10)); !IsReason(err, ReasonUnreadable) {
		t.Fatalf("read error: %v", err)
	}
	if _, err = drain(newScanReader(strings.NewReader("data"), nil, "f", 10)); !IsReason(err, ReasonSecretMatch) {
		t.Fatalf("nil scanner: %v", err)
	}
	if _, err = drain(newScanReader(strings.NewReader(""), nil, "f", 10)); err != nil {
		t.Fatalf("an empty file needs no scanner: %v", err)
	}
}

type fakeInfo struct {
	name string
	mode fs.FileMode
	size int64
}

func (f fakeInfo) Name() string       { return f.name }
func (f fakeInfo) Size() int64        { return f.size }
func (f fakeInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeInfo) Sys() any           { return nil }

func TestAdmitCaseFoldAndDuplicates(t *testing.T) {
	x := authz.NewExtractor(authz.Limits{})
	mk := func(rel string) found { return found{rel: rel, info: fakeInfo{name: rel, mode: 0o644, size: 3}} }
	if err := admit(x, nil, mk("dist/App.txt")); err != nil {
		t.Fatal(err)
	}
	var pe *authz.PathError
	if err := admit(x, nil, mk("dist/app.txt")); !errors.As(err, &pe) || pe.Reason != authz.CollisionFold {
		t.Fatalf("case-fold collision: %v", err)
	}
	if err := admit(x, nil, mk("dist/App.txt")); !errors.As(err, &pe) || pe.Reason != authz.CollisionDuplicate {
		t.Fatalf("duplicate: %v", err)
	}
	dev := found{rel: "dev0", info: fakeInfo{name: "dev0", mode: fs.ModeDevice | 0o644}}
	if err := admit(authz.NewExtractor(authz.Limits{}), nil, dev); !errors.As(err, &pe) || pe.Reason != authz.EntryDevice {
		t.Fatalf("device node: %v", err)
	}
	sock := found{rel: "s", info: fakeInfo{name: "s", mode: fs.ModeSocket | 0o644}}
	if err := admit(authz.NewExtractor(authz.Limits{}), nil, sock); !errors.As(err, &pe) || pe.Reason != authz.EntrySocket {
		t.Fatalf("socket: %v", err)
	}
	suid := found{rel: "su", info: fakeInfo{name: "su", mode: fs.ModeSetuid | 0o755}}
	if err := admit(authz.NewExtractor(authz.Limits{}), nil, suid); !errors.As(err, &pe) || pe.Reason != authz.EntrySetID {
		t.Fatalf("setuid: %v", err)
	}
}

func TestRecordNameAndPrefix(t *testing.T) {
	for _, c := range []struct {
		d       Decl
		rel     string
		literal bool
		want    string
	}{
		{Decl{Path: "out/app"}, "out/app", true, "out/app"},
		{Decl{Path: "out/app", Name: "app"}, "out/app", true, "app"},
		{Decl{Path: "out", Name: "rel"}, "out/a/b", false, "rel/a/b"},
		{Decl{Path: "out/*.tgz", Name: "pkgs"}, "out/x.tgz", false, "pkgs/x.tgz"},
		{Decl{Path: "*.log", Name: "logs"}, "a.log", false, "logs/a.log"},
		{Decl{Path: "**/x", Name: "n"}, "a/b/x", false, "n/a/b/x"},
		{Decl{Path: "a/*/c", Name: "n"}, "a/b/c", false, "n/b/c"},
		{Decl{Path: "*.log"}, "a.log", false, "a.log"},
	} {
		if got := recordName(c.d, c.rel, c.literal); got != c.want {
			t.Errorf("recordName(%+v, %q, %v) = %q, want %q", c.d, c.rel, c.literal, got, c.want)
		}
	}
	for p, want := range map[string]string{"a/b/c": "a/b/c", "a/*/c": "a", "*": "", "a/b?/c": "a", "x/y/[ab]": "x/y", "**/z": ""} {
		if got := literalPrefix(p); got != want {
			t.Errorf("literalPrefix(%q) = %q, want %q", p, got, want)
		}
	}
	for s, want := range map[string]bool{"plain": false, "a*": true, "a?": true, "[a]": true, "a/b": false} {
		if hasMeta(s) != want {
			t.Errorf("hasMeta(%q)", s)
		}
	}
}

func TestExpandPatterns(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a.txt", "b.log", "d1/x.txt", "d1/d2/y.txt", "d1/d2/z.log", "top/one"} {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	list := func(pattern string) string {
		t.Helper()
		got, err := expand(context.Background(), root.FS(), pattern, 100)
		if err != nil {
			t.Fatalf("%s: %v", pattern, err)
		}
		var rels []string
		for _, f := range got {
			rels = append(rels, f.rel)
		}
		return strings.Join(rels, " ")
	}
	for pattern, want := range map[string]string{
		"a.txt":       "a.txt",
		"*.txt":       "a.txt",
		"?.log":       "b.log",
		"[ab].*":      "a.txt b.log",
		"d1":          "d1/d2/y.txt d1/d2/z.log d1/x.txt",
		"d1/*.txt":    "d1/x.txt",
		"d1/**/*.log": "d1/d2/z.log",
		"**/y.txt":    "d1/d2/y.txt",
		"**/*.log":    "b.log d1/d2/z.log",
		"missing":     "",
		"d1/nope/*":   "",
		"*/one":       "top/one",
		"a.txt/x":     "",
	} {
		if got := list(pattern); got != want {
			t.Errorf("expand(%q) = %q, want %q", pattern, got, want)
		}
	}
	if _, err := expand(context.Background(), root.FS(), "[", 100); !IsReason(err, ReasonDeclaration) {
		t.Fatalf("bad pattern: %v", err)
	}
	if _, err := expand(context.Background(), root.FS(), "d1", 2); err == nil {
		t.Fatal("entry limit")
	}
	// a hostile pattern is bounded, not exponential
	hostile := strings.Repeat("**/", 40) + "y.txt"
	start := time.Now()
	got, err := expand(context.Background(), root.FS(), hostile, 100)
	if err != nil || len(got) != 1 || got[0].rel != "d1/d2/y.txt" || time.Since(start) > 5*time.Second {
		t.Fatalf("repeated **: %v %v after %v", got, err, time.Since(start))
	}
	if _, err = expand(context.Background(), root.FS(), strings.Repeat("a/", maxPatternParts)+"x", 100); !IsReason(err, ReasonDeclaration) {
		t.Fatalf("too many components: %v", err)
	}
}

func TestErrorsAndIDs(t *testing.T) {
	e := failf(ReasonMissing, "x %d", 1)
	if e.Error() != "E712 missing: x 1" || e.Code() != "E712" {
		t.Fatal(e)
	}
	if !errors.Is(e, &Error{ID: "E712"}) || !errors.Is(e, &Error{ID: "E712", Reason: ReasonMissing}) || errors.Is(e, &Error{ID: "E712", Reason: ReasonDigest}) || errors.Is(e, &Error{ID: "E713"}) {
		t.Fatal("Is")
	}
	if errors.Is(e, errors.New("x")) || IsReason(errors.New("x"), ReasonMissing) || IsReason(nil, ReasonMissing) {
		t.Fatal("Is on foreign errors")
	}
	if r := Refuse("epoch", "m"); r.Error() != "E711 epoch: m" {
		t.Fatal(r)
	}
	if Failf("config", "c").Error() != "E712 config: c" {
		t.Fatal("Failf")
	}
	first, again := RecordID("a1", "n"), RecordID("a1", "n")
	if first == RecordID("a2", "n") || first != again || RecordID("a", "bn") == RecordID("ab", "n") || !strings.HasPrefix(first, "art_") {
		t.Fatal("RecordID")
	}
	if k, ok := ParseKind(""); !ok || k != KindOther {
		t.Fatal("default kind")
	}
	if _, ok := ParseKind("nope"); ok {
		t.Fatal("unknown kind")
	}
	if k, ok := ParseKind("sbom"); !ok || k != KindSBOM {
		t.Fatal("sbom")
	}
}

func TestWorkdirOnlyFile(t *testing.T) {
	// linkCount reports the inode's names
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	if err := os.WriteFile(a, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Lstat(a)
	if linkCount(info) != 1 {
		t.Fatal("one name")
	}
	if err := os.Link(a, filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Lstat(a)
	if linkCount(info) != 2 {
		t.Fatal("two names")
	}
	if linkCount(fakeInfo{}) != 1 {
		t.Fatal("unknown")
	}
}

func TestStreamBudget(t *testing.T) {
	p := Policy{MaxFile: 100, MaxTotal: 1000}
	for _, c := range []struct {
		used, streamed int64
		want           int64
		reason         string
	}{
		{0, 0, 100, ReasonFileCap},
		{899, 0, 100, ReasonFileCap}, // room 101 > file cap 100
		{900, 0, 100, ReasonFileCap}, // room 100 == file cap: the file cap speaks
		{901, 0, 99, ReasonSizeCap},
		{500, 450, 50, ReasonSizeCap},
		{1000, 0, 0, ReasonSizeCap},
		{900, 200, 0, ReasonSizeCap}, // never negative
	} {
		got, reason := streamBudget(p, Usage{Total: c.used}, c.streamed)
		if got != c.want || reason != c.reason {
			t.Errorf("used %d streamed %d: %d %s, want %d %s", c.used, c.streamed, got, reason, c.want, c.reason)
		}
	}
}

type fakeLedger struct {
	Ledger
	got     PruneSpec
	pruneFn func(ctx context.Context) ([]Record, error)
	refs    int
}

func (f *fakeLedger) ArtifactPrune(ctx context.Context, s PruneSpec) ([]Record, error) {
	f.got = s
	if f.pruneFn != nil {
		return f.pruneFn(ctx)
	}
	return nil, nil
}
func (f *fakeLedger) ArtifactBlobRefs(context.Context) ([]BlobRef, error) { f.refs++; return nil, nil }

type fakeBlobs struct {
	Blobs
	swept int
}

func (f *fakeBlobs) Sweep(context.Context, string, func(cas.Realm, string) bool, time.Time) error {
	f.swept++
	return nil
}

func TestPruneDefaultsAndAutoPruneBudget(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	for _, c := range []struct {
		in, want PruneSpec
	}{
		{PruneSpec{Now: now, Release: true}, PruneSpec{Now: now, Release: true, OlderThan: DefaultReleaseAge}},
		{PruneSpec{Now: now, Release: true, OlderThan: time.Hour}, PruneSpec{Now: now, Release: true, OlderThan: time.Hour}},
		{PruneSpec{Now: now, Release: true, OlderThan: -time.Hour}, PruneSpec{Now: now, Release: true, OlderThan: DefaultReleaseAge}},
		{PruneSpec{Now: now}, PruneSpec{Now: now}},
		{PruneSpec{Now: now, OlderThan: time.Hour}, PruneSpec{Now: now, OlderThan: time.Hour}},
	} {
		l, b := &fakeLedger{}, &fakeBlobs{}
		if _, err := Prune(ctx, l, b, c.in); err != nil || l.got != c.want || b.swept != 1 {
			t.Fatalf("%+v: got %+v swept %d err %v", c.in, l.got, b.swept, err)
		}
	}
	l, b := &fakeLedger{}, &fakeBlobs{}
	if _, err := Prune(ctx, l, b, PruneSpec{DryRun: true}); err != nil || b.swept != 0 || l.got.Now.IsZero() {
		t.Fatalf("dry run sweeps nothing and stamps the clock: %+v swept %d %v", l.got, b.swept, err)
	}
	if AutoPruneBudget != 2*time.Second || DefaultReleaseAge != 365*24*time.Hour {
		t.Fatal("budget constants")
	}
	// a context that ended is not an error; a real error is
	dead, cancel := context.WithCancel(ctx)
	cancel()
	timeout := &fakeLedger{pruneFn: func(c context.Context) ([]Record, error) { return nil, c.Err() }}
	if err := AutoPrune(dead, timeout, &fakeBlobs{}, now); err != nil {
		t.Fatalf("expired budget: %v", err)
	}
	boom := &fakeLedger{pruneFn: func(context.Context) ([]Record, error) { return nil, errors.New("disk on fire") }}
	if err := AutoPrune(ctx, boom, &fakeBlobs{}, now); err == nil {
		t.Fatal("a real error must surface")
	}
	if err := AutoPrune(ctx, &fakeLedger{}, &fakeBlobs{}, now); err != nil {
		t.Fatal(err)
	}
}

func TestExpandBoundaries(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"d1/x.txt", "d1/d2/y.txt", "e1/y.txt", "e1/e2/e3/y.txt"} {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	ctx := context.Background()
	// every sibling is searched: ** does not stop at the first branch that matches
	got, err := expand(ctx, root.FS(), "**/y.txt", 100)
	if err != nil || len(got) != 3 {
		t.Fatalf("siblings: %v %v", got, err)
	}
	// exactly maxEntries files pass, one fewer allowed fails
	if _, err := expand(ctx, root.FS(), "e1", 2); err != nil {
		t.Fatalf("2 files in 2: %v", err)
	}
	if _, err := expand(ctx, root.FS(), "e1", 1); !IsReason(err, ReasonSizeCap) {
		t.Fatalf("2 files in 1: %v", err)
	}
	// 64 components are allowed, 65 are not
	if _, err := expand(ctx, root.FS(), strings.Repeat("a/", maxPatternParts-1)+"a", 100); err != nil {
		t.Fatalf("64 components: %v", err)
	}
	if _, err := expand(ctx, root.FS(), strings.Repeat("a/", maxPatternParts)+"a", 100); !IsReason(err, ReasonDeclaration) {
		t.Fatalf("65 components: %v", err)
	}
	// the visit budget: find what "**/y.txt" needs on this tree, then pin the boundary
	saved := maxVisits
	defer func() { maxVisits = saved }()
	need := 0
	for limit := 1; limit < 200; limit++ {
		maxVisits = limit
		if _, err := expand(ctx, root.FS(), "**/y.txt", 100); err == nil {
			need = limit
			break
		}
	}
	if need == 0 {
		t.Fatal("never fits")
	}
	maxVisits = need
	if _, err := expand(ctx, root.FS(), "**/y.txt", 100); err != nil {
		t.Fatalf("limit %d: %v", need, err)
	}
	maxVisits = need - 1
	if _, err := expand(ctx, root.FS(), "**/y.txt", 100); !IsReason(err, ReasonDeclaration) {
		t.Fatalf("limit %d: %v", need-1, err)
	}
	if need != 15 {
		t.Fatalf("the tree needs %d visits; update the pinned boundary only if the walk changed on purpose", need)
	}
	// a cancelled context stops the walk
	maxVisits = saved
	dead, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := expand(dead, root.FS(), "**/y.txt", 100); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}
