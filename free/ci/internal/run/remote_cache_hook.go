package run

// Purpose: attach coordinator cache plans and finalizers to remote deliveries.
// Inputs: coordinator project facts and one delivered lease. Outputs: plan and Sink.
// Constraints: cache failures remain best effort; only passing attempts finalize.
import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	cr "github.com/nself-org/plugins/free/ci/internal/cache/remote"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

var remoteCaches sync.Map // attempt id -> *remoteCacheRecord

type remoteCacheRecord struct {
	run, attempt string
	lease        interface {
		Finalize(context.Context, time.Duration) error
		Retract(context.Context) error
	}
	store interface {
		AttemptStateEpoch(context.Context, string) (model.JobState, int64, error)
	}
}

type remoteCacheFinalize struct{}

func init() {
	RegisterAttemptFinalize(remoteCacheFinalize{})
	RegisterAfterRun(remoteCacheFinalize{})
}
func (remoteCacheFinalize) Wants(a AttemptInfo) bool {
	_, ok := remoteCaches.Load(a.AttemptID)
	return ok
}
func (remoteCacheFinalize) AttemptFinalize(ctx context.Context, a AttemptInfo, _ int64) error {
	if r, ok := remoteCaches.Load(a.AttemptID); ok {
		// The shared finalize seam has a 15s background deadline. Cache refs
		// receive their complete 30s window, plus bounded commit time.
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), 35*time.Second)
		defer cancel()
		if err := r.(*remoteCacheRecord).lease.Finalize(bg, 30*time.Second); err != nil {
			cacheOn.warn("remote cache save refused: " + firstLineOf(err.Error()))
		}
	}
	return nil
}

// AfterRun forgets every session, including failed jobs which never finalized.
// A cancel that won after a commit retracts only this attempt's own entries.
func (remoteCacheFinalize) AfterRun(ctx context.Context, r RunInfo) error {
	remoteCaches.Range(func(k, v any) bool {
		e := v.(*remoteCacheRecord)
		if e.run != r.RunID || !remoteCaches.CompareAndDelete(k, v) {
			return true
		}
		state, _, err := e.store.AttemptStateEpoch(ctx, e.attempt)
		if err != nil || state == "passed" {
			return true
		}
		if err := e.lease.Retract(ctx); err != nil {
			cacheOn.warn("remote cache retract refused: " + firstLineOf(err.Error()))
		}
		return true
	})
	return nil
}

// remoteCachePlatform uses the selected node's measured platform for cache keys.
// Unknown platform facts disable cache planning instead of borrowing host facts.
func remoteCachePlatform(node model.Capability) string {
	os, arch := node.Platform.OS.Value, node.Platform.Arch.Value
	if os == nil || arch == nil || *os == "" || *arch == "" || strings.ContainsAny(*os+*arch, "/ \t\r\n") {
		return ""
	}
	return *os + "/" + *arch
}

// prepareCache runs before transport.Deliver and preserves the artifact Sink.
func prepareCache(ctx context.Context, sink transport.Sink, lease *protocol.Lease, root, node string, hosted bool, platform string) transport.Sink {
	if hosted {
		return sink
	}
	base, ok := sink.(*remoteSink)
	if !ok {
		if s, yes := sink.(leaseSink); yes {
			base = s.remoteSink
		}
	}
	if base == nil || root == "" || platform == "" || off(os.Getenv("NSELF_CI_CACHE_DISABLE")) || off(lease.Job.Env["NSELF_CI_CACHE_DISABLE"]) || topLevelDisabled(root) || lease.Job.Cache != nil && lease.Job.Cache.Disable {
		return sink
	}
	f, err := base.st.ArtifactFacts(ctx, lease.AttemptID)
	if err != nil {
		return sink
	}
	p, ok, err := base.st.PipelineByID(ctx, f.Run)
	if err != nil || !ok {
		return sink
	}
	workdir, err := workdirOf(root, lease.Job.Workdir)
	if err != nil {
		return sink
	}
	detected, err := detect.DefaultRegistry().Detect(os.DirFS(workdir), func(string) bool { return true })
	if err != nil {
		return sink
	}
	var scope, keys []string
	if c := lease.Job.Cache; c != nil {
		scope, err = specs.CheckPaths(c.Paths)
		if err != nil {
			return sink
		}
		keys = c.Keys
	}
	detected = specs.Narrow(detected, scope)
	a := AttemptInfo{RunID: f.Run, AttemptID: lease.AttemptID, Job: lease.Job, JobName: f.Job, Home: base.home, Store: base.st, Epoch: int64(lease.Epoch)}
	att := CacheAttestation{Protection: string(authz.Unprotected)}
	if CacheAttest != nil {
		att, err = CacheAttest(ctx, a, p)
		if err != nil {
			return sink
		}
	}
	env := mergeEnv(os.Environ(), lease.Job.Env)
	priv, _ := specs.PrivateDeps(specs.PrivInput{Dirs: factDirs(root, workdir, detected), Home: cacheOn.home(), Environ: flatEnv(env)})
	facts := cacheFacts(a, p, att, priv)
	home := records.StoreDir(base.home)
	blobs, err := cas.New(home)
	if err != nil {
		return sink
	}
	es := entries.New(base.st, blobs, entries.Options{Home: home, Project: func(string) authz.ProjectFacts { return authz.ProjectFacts{NoPrivateDeps: !priv} }})
	image := ""
	if lease.Job.Container != nil {
		_, image, _ = strings.Cut(lease.Job.Container.Image, "@")
		if !strings.HasPrefix(image, "sha256:") {
			image = ""
		}
	}
	isolation := string(lease.Job.Isolation)
	if isolation == "" {
		isolation = "process"
	}
	// A remote process toolchain cannot be probed on the coordinator. Resolve
	// safely disables those specs until coordinator facts supply its identity.
	plan, err := cr.BuildPlan(ctx, cr.PlanInput{Entries: es, Backend: base.st, Facts: facts, Detected: detected, Scope: scope, ExtraKeys: append(keys, "job="+f.Job), Environ: flatEnv(env),
		Platform: platform, Isolation: isolation, Container: isolation == "container", Image: image})
	if err != nil || plan == nil {
		return sink
	}
	pol, err := records.PolicyFromEnv(os.Getenv)
	if err != nil {
		return sink
	}
	pol.StorePath = home
	h := &cr.Handler{Ledger: base.st, Entries: es, Blobs: blobs, Limits: transfer.Limits{Disk: func(n int64) error {
		if err := pol.CheckDisk(n); err != nil {
			return &entries.Refused{Reason: entries.ReasonDiskFloor, Err: err}
		}
		return nil
	}}}
	l := h.Bind(cr.Binding{AttemptID: lease.AttemptID, LeaseID: lease.LeaseID, Epoch: int64(lease.Epoch), Node: node, Facts: facts, Plan: plan,
		Fence: func(ctx context.Context) error {
			return staleLease(base.eng.Fence(ctx, lease.LeaseID, int64(lease.Epoch)))
		},
		Ready: func(ctx context.Context) error {
			ra := base.attempt(lease.AttemptID)
			if ra == nil {
				return fmt.Errorf("unknown cache attempt")
			}
			return base.markRunning(ctx, ra)
		}})
	if plan.Access == "rw" && len(plan.Save) > 0 {
		remoteCaches.Store(lease.AttemptID, &remoteCacheRecord{run: f.Run, attempt: lease.AttemptID, lease: l, store: base.st})
	}
	lease.Cache = plan
	return cacheLeaseSink{Sink: sink, l: l}
}

type cacheLeaseSink struct {
	transport.Sink
	l *cr.Lease
}

func (s cacheLeaseSink) Blob(ctx context.Context, h transport.BlobHeader) (transport.BlobWriter, error) {
	if h.Stream == cr.StreamCache {
		return s.l.Blob(ctx, h)
	}
	return s.Sink.Blob(ctx, h)
}
func (s cacheLeaseSink) CacheRef(ctx context.Context, r protocol.CacheRef) error {
	return s.l.CacheRef(ctx, r)
}
func (s cacheLeaseSink) Result(ctx context.Context, r protocol.Result) error {
	if err := s.l.Declare(ctx); err != nil {
		cacheOn.warn("remote cache plan refused: " + firstLineOf(err.Error()))
	}
	return s.Sink.Result(ctx, r)
}
