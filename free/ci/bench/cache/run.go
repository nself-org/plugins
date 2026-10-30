package cache

// Purpose: the cold/warm cycle. Per fixture and run index i: a cold attempt
// (miss, the job fills the cache dirs, Finalize saves), then a warm attempt on
// the same key (hit, the dirs are restored and compared with what was saved).
// Inputs:  Options.
// Outputs: []FixtureResult, one per fixture, with the raw samples.
// Constraints: only Begin (plan, key, lookup, unpack) and Finalize (pack, scan,
// put, commit) are timed; generating the tree stands in for the job and is not.
// A warm run whose tree differs from the saved one is an error, not a sample.

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Options configures Run.
type Options struct {
	Runs int
	Seed int64
	Dir  string // work directory, created and removed by the caller
}

// Sample is one cycle of one fixture.
type Sample struct {
	ColdRestoreMs, SaveMs, WarmRestoreMs float64
	IntegrityMs                          float64
	Hit                                  bool
	PackedBytes                          int64
}

// FixtureResult is every cycle of one fixture.
type FixtureResult struct {
	Fixture string
	Tree    Tree // the tree one cycle saves and restores
	Samples []Sample
}

func ms(d time.Duration) float64 { return float64(d.Nanoseconds()) / 1e6 }

// Run executes opts.Runs interleaved cold/warm cycles of every fixture.
func Run(ctx context.Context, opts Options) ([]FixtureResult, error) {
	env, err := NewEnv(opts.Dir)
	if err != nil {
		return nil, err
	}
	defer env.Close()
	var out []FixtureResult
	for _, fx := range Fixtures() {
		res, err := runFixtureIn(ctx, env, opts, fx)
		if err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, nil
}

// runFixture is Run for one fixture over a fresh engine in opts.Dir.
func runFixture(ctx context.Context, opts Options, fx Fixture) (FixtureResult, error) {
	env, err := NewEnv(opts.Dir)
	if err != nil {
		return FixtureResult{}, err
	}
	defer env.Close()
	return runFixtureIn(ctx, env, opts, fx)
}

func runFixtureIn(ctx context.Context, env *Env, opts Options, fx Fixture) (FixtureResult, error) {
	block := Block(opts.Seed)
	res := FixtureResult{Fixture: fx.Name}
	for i := 0; i < opts.Runs; i++ {
		s, tree, err := cycle(ctx, env, fx, opts, block, i)
		if err != nil {
			return res, fmt.Errorf("%s run %d: %w", fx.Name, i, err)
		}
		res.Tree, res.Samples = tree, append(res.Samples, s)
	}
	return res, nil
}

func (e *Env) begin(ctx context.Context, fx Fixture, id, lock string) (*specs.Session, time.Duration, error) {
	probe := func(_ context.Context, argv []string) (string, error) {
		if v, ok := fx.Probe[argv[0]]; ok {
			return v, nil
		}
		return "", fmt.Errorf("no probe output for %v", argv)
	}
	in := specs.BeginInput{Engine: e.Engine, Attempt: id, Epoch: 2, Facts: e.Facts, Root: filepath.Join(e.Dir, "cache-work", id),
		Detected: fx.Facts(lock), Platform: "linux/amd64", Isolation: "process", Probe: probe, Getenv: func(string) string { return "" },
		Warn: func(s string) { fmt.Fprintln(os.Stderr, "cachebench: warn:", s) }}
	t0 := time.Now()
	s, err := specs.Begin(ctx, in)
	return s, time.Since(t0), err
}

func cycle(ctx context.Context, env *Env, fx Fixture, opts Options, block []byte, i int) (Sample, Tree, error) {
	var smp Sample
	lock := LockSHA(fx.Name, opts.Seed, i)
	cold, warm := fmt.Sprintf("cold-%s-%d", fx.Name, i), fmt.Sprintf("warm-%s-%d", fx.Name, i)
	if err := env.Attempt(ctx, cold); err != nil {
		return smp, Tree{}, err
	}
	sess, d, err := env.begin(ctx, fx, cold, lock)
	if err != nil {
		return smp, Tree{}, err
	}
	smp.ColdRestoreMs = ms(d)
	root := filepath.Join(env.Dir, "cache-work", cold)
	for n, name := range fx.DirOrder { // the job: fills the cache dirs
		if err = WriteTree(root, filepath.Join(fx.Spec, name), fx.Dirs[name], opts.Seed+int64(100*i+n), block); err != nil {
			return smp, Tree{}, err
		}
	}
	want, err := Measure(root, fx.Spec)
	if err != nil {
		return smp, Tree{}, err
	}
	epoch, err := env.Finalizing(ctx, cold)
	if err != nil {
		return smp, Tree{}, err
	}
	t0 := time.Now()
	sess.Finalize(ctx, epoch)
	smp.SaveMs = ms(time.Since(t0))
	if !sess.Saved() {
		return smp, Tree{}, fmt.Errorf("cold attempt saved nothing")
	}
	if err = env.Store.Transition(ctx, cold, "finalizing", "passed", store.TransitionDetail{Epoch: epoch}); err != nil {
		return smp, Tree{}, err
	}
	sess.Cleanup()
	if err = os.RemoveAll(root); err != nil {
		return smp, Tree{}, err
	}
	if err = env.Attempt(ctx, warm); err != nil {
		return smp, Tree{}, err
	}
	sess, d, err = env.begin(ctx, fx, warm, lock)
	if err != nil {
		return smp, Tree{}, err
	}
	smp.WarmRestoreMs = ms(d)
	wroot := filepath.Join(env.Dir, "cache-work", warm)
	got, err := Measure(wroot, fx.Spec)
	if err != nil {
		return smp, Tree{}, err
	}
	if got != want {
		uses, _ := env.Entries.Usage(ctx, warm)
		return smp, Tree{}, fmt.Errorf("restored tree differs from the saved one: got %+v want %+v (uses %+v)", got, want, uses)
	}
	uses, err := env.Entries.Usage(ctx, warm)
	if err != nil {
		return smp, Tree{}, err
	}
	for _, u := range uses {
		smp.Hit = smp.Hit || u.Result == "hit"
		smp.PackedBytes += u.Bytes
	}
	if smp.IntegrityMs, err = env.integrity(ctx, uses); err != nil {
		return smp, Tree{}, err
	}
	sess.Cleanup()
	return smp, want, os.RemoveAll(wroot)
}

// integrity is the cost of one full SHA-256 pass over the entry's blobs, with
// the bytes already in memory: the re-hash a verified read adds to a restore.
func (e *Env) integrity(ctx context.Context, uses []entries.Use) (float64, error) {
	ns, err := entries.NamespaceOf(e.Facts)
	if err != nil {
		return 0, err
	}
	var bufs [][]byte
	for _, u := range uses {
		m, ok, err := e.Entries.Lookup(ctx, ns, u.Key, authz.ModeRWOwn)
		if err != nil || !ok {
			return 0, fmt.Errorf("integrity lookup of %s: ok=%v err=%v", u.Key, ok, err)
		}
		for _, b := range m.Blobs {
			var raw []byte
			err = e.Blobs.ReadVerified(cas.CacheRealm(string(ns.Canonical())), b.SHA256, func(r io.Reader) (err error) { raw, err = io.ReadAll(r); return })
			if err != nil {
				return 0, err
			}
			bufs = append(bufs, raw)
		}
	}
	t0 := time.Now()
	for _, b := range bufs {
		_ = sha256.Sum256(b)
	}
	return ms(time.Since(t0)), nil
}
