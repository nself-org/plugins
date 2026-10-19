package run

// Purpose: wire the per-ecosystem cache specs into the run (P7-CACHE-02, Epic
// D5). Before restores what authz.CacheAccess allows and returns the cache
// environment and mounts; the save happens in the finalize seam, because
// entries.Commit needs the attempt `finalizing` with a recorded pass and the
// CacheHook.After call comes after the attempt left that state (lead ruling in
// the CACHE Epic Invalidations); After only removes the attempt's directories.
// Inputs:  AttemptInfo (job, store, home, launch epoch), the pipeline row
// (project root, source trust), detector facts of the workspace, the process
// environment, NSELF_CI_CACHE_DISABLE (operator scope).
// Outputs: CacheSetup, entries committed in finalizing, usage rows.
// Constraints: a failed or cancelled attempt saves nothing; cache directories
// live under $NSELF_CI_HOME/cache-work/<attempt> (the executor owns
// work/<attempt>); the per-job `cache:` config can only tighten (disable, add
// keys, narrow paths) and an operator disable cannot be undone by the
// revision; cache failures never fail the job, only an out-of-workspace
// `cache.paths` does (E711 path_outside).

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

var _ authz.ValueScanner = (*redact.Redactor)(nil)

// CacheAttestation is what the pipeline row cannot hold: whether the ref is
// protected and, for an untrusted source, its head repo and PR. The values come
// from trust policy, never from the job.
type CacheAttestation struct{ Protection, HeadRepo, PR string }

// CacheAttest supplies the attestation; nil means unprotected with no head repo
// or PR, so an untrusted source gets no cache at all (E684 namespace rule).
var CacheAttest func(ctx context.Context, a AttemptInfo, p store.PipelineRow) (CacheAttestation, error)

// cacheEntry is a live session; one whose attempt never reached After (the job
// failed to prepare after Before) is evicted once it is staleAfter old.
type cacheEntry struct {
	s  *specs.Session
	at time.Time
}

type cacheHook struct {
	sessions sync.Map // attempt id -> cacheEntry
	scanner  func() authz.ValueScanner
	environ  func() []string
	home     func() string
	probe    func(ctx context.Context, argv []string) (string, error)
	warn     func(string)
}

var cacheOn = &cacheHook{
	scanner: func() authz.ValueScanner { return Redactions },
	environ: os.Environ,
	home:    func() string { h, _ := os.UserHomeDir(); return h },
	probe: func(ctx context.Context, argv []string) (string, error) {
		out, code, err := report.Capture(ctx, argv...)
		if err == nil && code != 0 {
			err = fmt.Errorf("exit %d", code)
		}
		return out, err
	},
	warn: func(w string) { fmt.Fprintln(os.Stderr, "nself-ci: warning: "+w) },
}

func init() {
	RegisterCache(cacheOn)
	RegisterAttemptFinalize(cacheOn)
}

// staleAfter is how old an abandoned cache-work directory must be to be removed.
const staleAfter = 24 * time.Hour

// Before restores the cache of a.
func (h *cacheHook) Before(ctx context.Context, a AttemptInfo) (CacheSetup, error) {
	var none CacheSetup
	env := mergeEnv(h.environ(), a.Job.Env)
	if a.Store == nil || a.Epoch == 0 || off(env["NSELF_CI_CACHE_DISABLE"]) || a.Job.Cache != nil && a.Job.Cache.Disable {
		return none, nil
	}
	var scope, keys []string
	if c := a.Job.Cache; c != nil {
		var err error
		if scope, err = specs.CheckPaths(c.Paths); err != nil {
			return none, err
		}
		keys = c.Keys
	}
	p, ok, err := a.Store.PipelineByID(ctx, a.RunID)
	if err != nil || !ok || !filepath.IsAbs(p.Project) {
		return none, nil
	}
	if topLevelDisabled(p.Project) {
		return none, nil
	}
	workdir, err := workdirOf(p.Project, a.Job.Workdir)
	if err != nil {
		return none, nil
	}
	detected, err := detect.DefaultRegistry().Detect(os.DirFS(workdir), func(string) bool { return true })
	if err != nil {
		return none, nil
	}
	detected = specs.Narrow(detected, scope)
	att := CacheAttestation{Protection: string(authz.Unprotected)}
	if CacheAttest != nil {
		if att, err = CacheAttest(ctx, a, p); err != nil {
			h.warn("cache attestation failed: " + firstLineOf(err.Error()))
			return none, nil
		}
	}
	return h.begin(ctx, a, p, workdir, detected, scope, keys, att, env), nil
}

func (h *cacheHook) begin(ctx context.Context, a AttemptInfo, p store.PipelineRow, workdir string, detected []detect.Fact, scope, keys []string, att CacheAttestation, env map[string]string) CacheSetup {
	home := records.StoreDir(a.Home)
	blobs, err := cas.New(home)
	if err != nil {
		return CacheSetup{}
	}
	container := a.Job.Isolation == "container" || a.Job.Container != nil
	priv, _ := specs.PrivateDeps(specs.PrivInput{Dirs: []string{p.Project, workdir}, Home: h.home(), Environ: flatEnv(env)})
	facts := cacheFacts(a, p, att, priv)
	es := entries.New(a.Store, blobs, entries.Options{Home: home, Project: func(string) authz.ProjectFacts { return authz.ProjectFacts{NoPrivateDeps: !priv} }})
	in := specs.BeginInput{Engine: &specs.Engine{Entries: es, Blobs: blobs, Scanner: h.scanner(), Backend: a.Store}, Attempt: a.AttemptID, Epoch: a.Epoch,
		Facts: facts, Root: filepath.Join(home, "cache-work", a.AttemptID), Detected: detected, Scope: scope, ExtraKeys: keys, Container: container,
		Platform: runtime.GOOS + "/" + runtime.GOARCH, Isolation: "process", JobEnv: a.Job.Env, Environ: flatEnv(env), Probe: h.probe, Home: h.home(), Getenv: func(k string) string { return env[k] }, Warn: h.warn}
	if container {
		in.Platform, in.Isolation, in.Image = "linux/"+runtime.GOARCH, "container", h.imageDigest(ctx, a)
	}
	sweepStale(filepath.Join(home, "cache-work"), time.Now())
	s, err := specs.Begin(ctx, in)
	if err != nil {
		h.warn("cache plan refused: " + firstLineOf(err.Error()))
		s.Cleanup()
		return CacheSetup{}
	}
	h.evict(time.Now())
	h.sessions.Store(a.AttemptID, cacheEntry{s, time.Now()})
	envOut, mounts := s.Setup()
	out := CacheSetup{Env: envOut}
	for _, m := range mounts {
		out.Mounts = append(out.Mounts, Mount(m))
	}
	return out
}

// cacheFacts are the coordinator-attested facts the store records at RecordPlan
// (and compares with the pipeline row). SecretsDeclared is true when the job
// spec names any `secrets:` ref, resolved or not: conservative, and no edge to
// secret resolution.
func cacheFacts(a AttemptInfo, p store.PipelineRow, att CacheAttestation, privateDeps bool) entries.Facts {
	return entries.Facts{Project: p.Project, Trust: p.SourceTrust, Protection: att.Protection, HeadRepo: att.HeadRepo, PR: att.PR,
		SecretsDeclared: len(a.Job.Secrets) > 0, PrivateDeps: privateDeps}
}

// AttemptFinalize saves while the attempt is finalizing at epoch.
func (h *cacheHook) AttemptFinalize(ctx context.Context, a AttemptInfo, epoch int64) error {
	if e, ok := h.sessions.Load(a.AttemptID); ok {
		e.(cacheEntry).s.Finalize(ctx, epoch)
	}
	return nil
}

// Wants lets the run skip the finalize read when nothing can be saved.
func (h *cacheHook) Wants(a AttemptInfo) bool {
	e, ok := h.sessions.Load(a.AttemptID)
	return ok && e.(cacheEntry).s.Wants()
}

// After removes the attempt's cache directories; it saves nothing.
func (h *cacheHook) After(_ context.Context, a AttemptInfo, _ Outcome) error {
	if e, ok := h.sessions.LoadAndDelete(a.AttemptID); ok {
		e.(cacheEntry).s.Cleanup()
	}
	return nil
}

// evict cleans up sessions whose attempt never reached After.
func (h *cacheHook) evict(now time.Time) {
	h.sessions.Range(func(k, v any) bool {
		if e := v.(cacheEntry); now.Sub(e.at) > staleAfter {
			h.sessions.Delete(k)
			e.s.Cleanup()
		}
		return true
	})
}

// imageDigest is the pinned digest of the job's container image, "" if unknown.
func (h *cacheHook) imageDigest(ctx context.Context, a AttemptInfo) string {
	img := ""
	if a.Job.Container != nil {
		img = a.Job.Container.Image
	}
	if img == "" && Registered.Image != nil {
		img, _ = Registered.Image.Image(ctx, a)
	}
	if _, d, ok := strings.Cut(img, "@"); ok && strings.HasPrefix(d, "sha256:") {
		return d
	}
	return ""
}

// topLevelDisabled reads the top-level `cache: {disable: true}` of the default
// config (<root>/.nself/ci.yaml). The revision can only tighten, so an
// unreadable or absent file means "not disabled here"; a config passed with
// --config from elsewhere is not seen (debt D-cache-config-path).
func topLevelDisabled(root string) bool {
	l, err := config.Load(filepath.Join(root, ".nself", "ci.yaml"))
	return err == nil && l.Config.Cache != nil && l.Config.Cache.Disable
}

func off(v string) bool { return v == "1" || strings.EqualFold(v, "true") }

// workdirOf resolves the job's workdir below the project root (relative, inside).
func workdirOf(root, workdir string) (string, error) {
	if workdir == "" || workdir == "." {
		return root, nil
	}
	if !fs.ValidPath(workdir) {
		return "", fs.ErrInvalid
	}
	return filepath.Join(root, filepath.FromSlash(workdir)), nil
}

// mergeEnv overlays the job's env on the process environment.
func mergeEnv(environ []string, job map[string]string) map[string]string {
	out := map[string]string{}
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok {
			out[k] = v
		}
	}
	for k, v := range job {
		out[k] = v
	}
	return out
}

func flatEnv(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// sweepStale removes cache-work directories abandoned by a crashed run.
func sweepStale(dir string, now time.Time) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > staleAfter {
			_ = specs.RemoveTree(filepath.Join(dir, e.Name()))
		}
	}
}
