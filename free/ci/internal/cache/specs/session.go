package specs

// Purpose: one attempt's cache life (Epic CACHE D5): Begin plans the keys,
// restores what the access rules allow and returns the environment and mounts;
// Finalize saves, only for a passing attempt that is finalizing; Cleanup
// removes the attempt's cache directories.
// Inputs:  BeginInput (coordinator-attested facts, narrowed detector facts, the
// job's environment, a tool probe) and the Engine.
// Outputs: Session with Setup, Finalize, Cleanup; usage rows per spec.
// Constraints: authz.CacheAccess decides the mode of every namespace read:
// rw-own for the attempt's own namespace, ro-snapshot for a protected namespace
// of a strictly higher class, none otherwise. A container job only ever gets
// read-only mounts (the hardened container profile refuses a writable host
// mount, E611), so a container job restores but cannot populate a cache; a
// process job cannot hold a mount read-only, so a snapshot restore is denied
// for it. A restored own entry is never saved again. Failures of the cache
// itself never fail the job: they become usage rows and warnings.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Mount is a host path made visible to the job at Target.
type Mount struct {
	Host, Target string
	ReadOnly     bool
}

// BeginInput is everything Begin needs.
type BeginInput struct {
	Engine    *Engine
	Attempt   string
	Epoch     int64
	Facts     entries.Facts
	Root      string // the attempt's cache root, absolute; created on demand
	Detected  []detect.Fact
	Scope     []string
	ExtraKeys []string
	Container bool
	Platform  string
	Isolation string
	Image     string
	JobEnv    map[string]string
	Environ   []string
	Probe     func(ctx context.Context, argv []string) (string, error)
	Home      string
	Getenv    func(string) string
	Warn      func(string)
}

// Item is one spec of the attempt.
type Item struct {
	Spec   Spec
	Mat    Material
	Key    string
	Root   string
	Dirs   Dirs
	Access authz.Mode // the authz mode of the namespace read: rw-own, ro-snapshot, or none
	Hit    bool
	NS     authz.Namespace
	// Bound is true when the job gets this spec's dirs (environment and mounts);
	// ReadOnly says the mounts are read-only.
	Bound, ReadOnly bool
}

// Session is one attempt's cache state.
type Session struct {
	in    BeginInput
	own   authz.Namespace
	items []*Item
}

// Begin plans, restores and returns the session; the error is a failure of the
// plan itself (the caller then runs the job without a cache).
func Begin(ctx context.Context, in BeginInput) (*Session, error) {
	s := &Session{in: in}
	list := For(in.Detected, in.Container)
	if len(list) == 0 {
		return s, nil
	}
	own, nsErr := entries.NamespaceOf(in.Facts)
	if nsErr != nil {
		for _, sp := range list {
			s.use(ctx, sp.ID, "", "denied", 0, 0)
		}
		return s, nil
	}
	s.own = own
	var keys []string
	for _, sp := range list {
		it, ok := s.plan(ctx, sp)
		if !ok {
			continue
		}
		s.items = append(s.items, it)
		keys = append(keys, it.Key)
	}
	if len(s.items) == 0 {
		return s, nil
	}
	if err := in.Engine.Entries.RecordPlan(ctx, in.Attempt, in.Epoch, in.Facts, keys); err != nil {
		s.items = nil
		return s, err
	}
	for _, it := range s.items {
		s.restore(ctx, it)
	}
	return s, nil
}

// plan resolves one spec's key material and directories, or records why not.
func (s *Session) plan(ctx context.Context, sp Spec) (*Item, bool) {
	in := s.in
	root := filepath.Join(in.Root, sp.ID)
	bound := []string{sp.RootEnv}
	it := &Item{Spec: sp, Root: root, Dirs: Dirs{}, Access: authz.ModeNone}
	for _, d := range sp.Dirs {
		bound = append(bound, d.Env)
		it.Dirs[d.Name] = filepath.Join(root, d.Name)
	}
	for _, name := range bound {
		if _, set := in.JobEnv[name]; set && name != "" { // the job pinned its own location: not ours to override
			s.use(ctx, sp.ID, "", "disabled", 0, 0)
			return nil, false
		}
	}
	if strings.ContainsAny(root, ",\r\n\x00") || !filepath.IsAbs(root) {
		s.use(ctx, sp.ID, "", "disabled", 0, 0)
		return nil, false
	}
	var err error
	it.Mat, err = Resolve(ctx, Input{Spec: sp, Platform: in.Platform, Isolation: in.Isolation, Image: in.Image, Facts: in.Detected,
		Scope: in.Scope, ExtraKeys: in.ExtraKeys, Environ: in.Environ, Probe: in.Probe})
	if err == nil {
		it.Key, err = it.Mat.Key(s.own)
	}
	if err != nil {
		s.use(ctx, sp.ID, "", "disabled", 0, 0)
		return nil, false
	}
	return it, true
}

// snapshotSources lists the namespaces s may read read-only.
func (s *Session) snapshotSources() []authz.Namespace {
	job := authz.Job{Namespace: s.own, State: authz.StateRunning, Capability: authz.Capability{Attempt: s.in.Attempt, Epoch: s.in.Epoch}}
	var out []authz.Namespace
	for _, class := range []model.TrustClass{"internal", "owner"} {
		ns := authz.Namespace{Project: s.own.Project, Trust: class, Protection: authz.Protected}
		if authz.CacheAccess(job, ns) == authz.ModeROSnapshot {
			out = append(out, ns)
		}
	}
	return out
}

// restore fills it.Dirs from the first namespace that has the entry.
func (s *Session) restore(ctx context.Context, it *Item) {
	start := time.Now()
	for _, d := range it.Dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			s.warn("cache dir for spec " + it.Spec.ID + " could not be created")
			it.Access = authz.ModeNone
			s.use(ctx, it.Spec.ID, it.Key, "disabled", 0, 0)
			return
		}
	}
	s.seed(it)
	job := authz.Job{Namespace: s.own, State: authz.StateRunning, Capability: authz.Capability{Attempt: s.in.Attempt, Epoch: s.in.Epoch}}
	it.Access = authz.CacheAccess(job, s.own)
	for _, ns := range append([]authz.Namespace{s.own}, s.snapshotSources()...) {
		access, key := authz.CacheAccess(job, ns), it.Key
		if !ns.Equal(s.own) {
			key, _ = it.Mat.Key(ns)
		}
		hit, n, err := s.in.Engine.Restore(ctx, ns, key, access, it.Spec, it.Dirs)
		ms := time.Since(start).Milliseconds()
		if err != nil {
			s.warn(fmt.Sprintf("cache restore of spec %s refused: %s", it.Spec.ID, refusalKey(errors.Unwrap(err))))
			s.use(ctx, it.Spec.ID, key, "refused", 0, ms)
			s.reset(it)
			continue
		}
		if !hit {
			continue
		}
		if access == authz.ModeROSnapshot && !s.in.Container {
			s.reset(it)
			it.Access = authz.ModeNone
			s.use(ctx, it.Spec.ID, key, "denied", 0, ms)
			return
		}
		it.Hit, it.NS, it.Access, it.Bound, it.ReadOnly = true, ns, access, true, access == authz.ModeROSnapshot || s.in.Container
		result := "hit"
		if it.ReadOnly {
			result = "readonly"
		}
		s.use(ctx, it.Spec.ID, key, result, n, ms)
		return
	}
	it.Bound = !s.in.Container // a container miss has no writable mount to fill
	s.use(ctx, it.Spec.ID, it.Key, "miss", 0, time.Since(start).Milliseconds())
}

// reset empties and recreates the dirs of it.
func (s *Session) reset(it *Item) {
	for _, d := range it.Dirs {
		_ = RemoveTree(d)
		_ = os.MkdirAll(d, 0o700)
	}
}

// Setup returns the variables and mounts of every spec that has access.
func (s *Session) Setup() (map[string]string, []Mount) {
	env, mounts := map[string]string{}, []Mount(nil)
	for _, it := range s.items {
		if !it.Bound {
			continue
		}
		ro := it.ReadOnly
		if it.Spec.RootEnv != "" {
			env[it.Spec.RootEnv] = it.Root
			mounts = append(mounts, Mount{Host: it.Root, Target: it.Root, ReadOnly: ro})
			continue
		}
		for _, d := range it.Spec.Dirs {
			if d.Env != "" {
				env[d.Env] = it.Dirs[d.Name]
			}
			mounts = append(mounts, Mount{Host: it.Dirs[d.Name], Target: it.Dirs[d.Name], ReadOnly: ro})
		}
	}
	return env, mounts
}

// seed copies the user's tool-home config files into it.Root (cargo's
// config.toml) because the bound variable hides the user's own directory.
func (s *Session) seed(it *Item) {
	sd := it.Spec.Seed
	if sd.HomeEnv == "" {
		return
	}
	home := s.in.Getenv(sd.HomeEnv)
	if home == "" && s.in.Home != "" {
		home = filepath.Join(s.in.Home, sd.HomeDir)
	}
	for _, name := range sd.Names {
		src := filepath.Join(home, name)
		if info, err := os.Lstat(src); err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			continue
		}
		if f, err := os.Open(src); err == nil {
			if out, err := os.OpenFile(filepath.Join(it.Root, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err == nil {
				_, _ = io.Copy(out, f)
				_ = out.Close()
			}
			_ = f.Close()
		}
	}
}

// Cleanup removes the attempt's cache directories.
func (s *Session) Cleanup() { _ = RemoveTree(s.in.Root) }

func (s *Session) warn(msg string) {
	if s.in.Warn != nil {
		s.in.Warn(msg)
	}
}

func (s *Session) use(ctx context.Context, spec, key, result string, bytes, ms int64) {
	if err := s.in.Engine.Entries.RecordUse(ctx, entries.Use{Attempt: s.in.Attempt, Spec: spec, Key: key, Result: result, Bytes: bytes, MS: ms}); err != nil {
		s.warn("cache usage row for spec " + spec + " was not recorded")
	}
}
