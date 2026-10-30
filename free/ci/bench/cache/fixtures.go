package cache

// Purpose: the Go, pnpm and Rust fixtures: seeded cache trees of the size a
// mid-size project fills (module and build cache, package store, registry and
// git checkouts), and the detector facts that select the cache spec.
// Inputs:  a fixture, a seed and a destination root.
// Outputs: Tree (file count, bytes, content digest) written under the spec dirs.
// Constraints: a tree is a pure function of (fixture, seed, dir); content is
// slices of one seeded block with the file index stamped in, so generating
// costs almost nothing and no two files share a digest.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/detect"
)

// TreeSpec is the shape of one spec dir.
type TreeSpec struct {
	Files int   // number of files
	Bytes int64 // target total bytes (files are drawn around Bytes/Files)
}

// Fixture is one repository shape.
type Fixture struct {
	Name     string
	Spec     string // cache spec id
	Facts    func(lockSHA string) []detect.Fact
	Probe    map[string]string // tool name -> probe output
	Dirs     map[string]TreeSpec
	DirOrder []string
}

func certain(kind, name, path, sha string) detect.Fact {
	return detect.Fact{Kind: kind, Name: name, Path: path, SHA256: sha, Confidence: detect.Certain}
}

// Fixtures lists the three fixtures in report order.
func Fixtures() []Fixture {
	return []Fixture{
		{Name: "go", Spec: "go", DirOrder: []string{"mod", "build"},
			Dirs:  map[string]TreeSpec{"mod": {1200, 24 << 20}, "build": {600, 24 << 20}},
			Probe: map[string]string{"go": "go version go1.26.4 linux/amd64\n"},
			Facts: func(l string) []detect.Fact {
				return []detect.Fact{certain("stack", "go", "go.mod", ""), certain("lockfile", "go", "go.sum", l)}
			}},
		{Name: "pnpm", Spec: "pnpm", DirOrder: []string{"store"},
			Dirs:  map[string]TreeSpec{"store": {3000, 40 << 20}},
			Probe: map[string]string{"node": "v22.11.0\n", "pnpm": "9.15.0\n"},
			Facts: func(l string) []detect.Fact {
				return []detect.Fact{certain("package_manager", "pnpm", "pnpm-lock.yaml", ""), certain("lockfile", "pnpm", "pnpm-lock.yaml", l)}
			}},
		{Name: "rust", Spec: "cargo", DirOrder: []string{"git", "registry"},
			Dirs:  map[string]TreeSpec{"git": {300, 6 << 20}, "registry": {1500, 28 << 20}},
			Probe: map[string]string{"cargo": "cargo 1.85.0\n", "rustc": "rustc 1.85.0\n"},
			Facts: func(l string) []detect.Fact {
				return []detect.Fact{certain("stack", "rust", "Cargo.toml", ""), certain("lockfile", "cargo", "Cargo.lock", l)}
			}},
	}
}

// Tree summarizes a written tree.
type Tree struct {
	Files  int
	Bytes  int64
	Digest string // sha256 over sorted relative paths, sizes and contents
}

const blockSize = 1 << 20

// Block returns the seeded pseudo-random block every file is cut from.
func Block(seed int64) []byte {
	b := make([]byte, blockSize)
	rand.New(rand.NewSource(seed)).Read(b)
	return b
}

// WriteTree writes the files of ts under root/sub: sizes are drawn from a
// log-uniform spread around the mean so there are many small files and a few
// large ones, as in a real module or package store.
func WriteTree(root, sub string, ts TreeSpec, seed int64, block []byte) error {
	rng := rand.New(rand.NewSource(seed))
	mean := float64(ts.Bytes) / float64(ts.Files)
	for i := 0; i < ts.Files; i++ {
		size := int(mean * (0.05 + rng.ExpFloat64()*0.95))
		size = min(max(size, 64), blockSize-16)
		off := rng.Intn(blockSize - size - 8)
		name := filepath.Join(root, sub, fmt.Sprintf("d%02d", i%37), fmt.Sprintf("f%05d.dat", i))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return err
		}
		buf := make([]byte, 8+size)
		binary.BigEndian.PutUint64(buf, uint64(seed)^uint64(i)*0x9e3779b97f4a7c15)
		copy(buf[8:], block[off:off+size])
		if err := os.WriteFile(name, buf, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Measure walks root/sub and returns its Tree.
func Measure(root, sub string) (Tree, error) {
	var names []string
	base := filepath.Join(root, sub)
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			names = append(names, p)
		}
		return err
	})
	if err != nil {
		return Tree{}, err
	}
	sort.Strings(names)
	h, t := sha256.New(), Tree{}
	for _, n := range names {
		raw, err := os.ReadFile(n)
		if err != nil {
			return Tree{}, err
		}
		rel, _ := filepath.Rel(base, n)
		fmt.Fprintf(h, "%s\x00%d\x00", rel, len(raw))
		h.Write(raw)
		t.Files++
		t.Bytes += int64(len(raw))
	}
	t.Digest = hex.EncodeToString(h.Sum(nil))
	return t, nil
}

// LockSHA is the lockfile digest of run i: a new one per cold run, so each cold
// run misses, and the same one for the warm run that follows it.
func LockSHA(fixture string, seed int64, i int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("lock/%s/%d/%d", fixture, seed, i)))
	return hex.EncodeToString(sum[:])
}
