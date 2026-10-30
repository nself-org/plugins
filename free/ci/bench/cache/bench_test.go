package cache

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

// BenchmarkKeyDerivation derives the cache key of a Go lockfile fact set with
// the production entries.Key.
func BenchmarkKeyDerivation(b *testing.B) {
	k := entries.KeyInputs{SpecID: "go", SpecVersion: "1", Platform: "linux/amd64", Isolation: "process", Toolchain: []string{"go=go1.26.4"}, Lockfiles: []string{"go.sum=" + LockSHA("go", 1, 0)}}
	ns, err := entries.NamespaceOf(entries.Facts{Project: "/work/bench", Trust: "owner", Protection: "protected"})
	if err != nil {
		b.Fatal(err)
	}
	k.Namespace = ns
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := entries.Key(k); err != nil {
			b.Fatal(err)
		}
	}
}

func newCAS(b *testing.B) (*cas.Store, cas.Realm) {
	b.Helper()
	s, err := cas.New(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	return s, cas.CacheRealm("bench")
}

// payload is size bytes cut from the seeded block, stamped with n so that
// every n is a distinct blob.
func payload(size int, n int) []byte {
	blk, out := Block(7), make([]byte, size)
	for off := 0; off < size; off += len(blk) {
		copy(out[off:], blk)
	}
	copy(out, []byte{byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)})
	return out
}

// BenchmarkCASPut stores 16 MiB through cas.Put (stream, hash, atomic publish).
func BenchmarkCASPut(b *testing.B) {
	s, realm := newCAS(b)
	const size = 16 << 20
	b.SetBytes(size)
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		data := payload(size, i)
		b.StartTimer()
		if _, _, err := s.Put(context.Background(), realm, bytes.NewReader(data)); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCASReadVerified reads 16 MiB back through the re-hashing read: the
// per-16-MiB cost of integrity (scale by 64 for a GiB).
func BenchmarkCASReadVerified(b *testing.B) {
	s, realm := newCAS(b)
	digest, size, err := s.Put(context.Background(), realm, bytes.NewReader(payload(16<<20, 1)))
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(size)
	for i := 0; i < b.N; i++ {
		if err := s.ReadVerified(realm, digest, func(r io.Reader) error { _, err := io.Copy(io.Discard, r); return err }); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSaveRestore runs one real cold save and warm restore of every
// fixture per iteration; the sub-benchmarks report the engine's own ns/op.
func BenchmarkSaveRestore(b *testing.B) {
	for _, fx := range Fixtures() {
		b.Run(fx.Name, func(b *testing.B) {
			dir := b.TempDir()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				one := Options{Runs: 1, Seed: int64(i + 1), Dir: filepath.Join(dir, "r"+string(rune('a'+i%26)))}
				if err := os.MkdirAll(one.Dir, 0o755); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				if _, err := runFixture(context.Background(), one, fx); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestRunIsReal runs two cold/warm cycles of every fixture and checks what the
// numbers claim: a warm run hits, restores the saved tree byte for byte and has
// a positive restore time; and that the same seed builds the same tree.
func TestRunIsReal(t *testing.T) {
	res, err := Run(context.Background(), Options{Runs: 2, Seed: 5, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("fixtures=%d", len(res))
	}
	for _, fr := range res {
		if len(fr.Samples) != 2 || fr.Tree.Files == 0 || fr.Tree.Bytes == 0 {
			t.Fatalf("%s: %+v", fr.Fixture, fr)
		}
		for _, s := range fr.Samples {
			if !s.Hit || s.WarmRestoreMs <= 0 || s.SaveMs <= 0 || s.PackedBytes < fr.Tree.Bytes {
				t.Fatalf("%s: not a real cycle: %+v", fr.Fixture, s)
			}
		}
	}
	a, b := t.TempDir(), t.TempDir()
	fx := Fixtures()[2]
	for _, d := range []string{a, b} {
		if err := WriteTree(d, "x", fx.Dirs["git"], 9, Block(9)); err != nil {
			t.Fatal(err)
		}
	}
	ta, _ := Measure(a, "x")
	tb, _ := Measure(b, "x")
	if ta != tb || ta.Files != fx.Dirs["git"].Files {
		t.Fatalf("seeded tree is not deterministic: %+v vs %+v", ta, tb)
	}
}

func TestBuildReportAndSlowdown(t *testing.T) {
	fr := FixtureResult{Fixture: "go", Tree: Tree{Files: 1, Bytes: 1 << 20}}
	for i := 1; i <= 20; i++ {
		fr.Samples = append(fr.Samples, Sample{ColdRestoreMs: 1, SaveMs: float64(i), WarmRestoreMs: float64(2 * i), IntegrityMs: 1, Hit: true, PackedBytes: 1 << 20})
	}
	rep := BuildReport("abc", 1, []FixtureResult{fr}, nil)
	var found bool
	for _, m := range rep.Metrics {
		if m.Name == "restore_ms" {
			found = true
			if m.N != 20 || m.P95 != 38 {
				t.Fatalf("restore_ms %+v", m)
			}
		}
	}
	if !found || rep.Injected || rep.Fixtures[0].HitRate != 1 {
		t.Fatalf("report %+v", rep)
	}
	rep.Slowdown(2)
	for _, m := range rep.Metrics {
		if m.Name == "restore_ms" && m.P95 != 76 {
			t.Fatalf("slowdown %+v", m)
		}
	}
	if rep.Injected {
		t.Fatal("a slowdown fixture must stay injected=false")
	}
}
