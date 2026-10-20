package locality

// Purpose: the `cache` contributor over a fixture world: coordinator host, a node with a
// read-through copy, a node with nothing, a stale copy, jobs without specs or entries.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	platformLinux = "linux/amd64"
	imageDigest   = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	goSize        = int64(1500)
	pnpmSize      = int64(700)
)

var (
	nowMs      = time.Unix(1_000_000, 0).UnixMilli()
	ownerFacts = entries.Facts{Project: "/work/proj", Trust: "owner", Protection: "protected"}
	goFacts    = []detect.Fact{
		{Kind: "stack", Name: "go", Path: "go.mod", Confidence: detect.Certain},
		{Kind: "lockfile", Name: "go", Path: "go.sum", SHA256: strings.Repeat("a1", 32), Confidence: detect.Certain},
	}
	pnpmFacts = []detect.Fact{{Kind: "package_manager", Name: "pnpm", Path: "pnpm-lock.yaml", Confidence: detect.Certain},
		{Kind: "lockfile", Name: "pnpm", Path: "pnpm-lock.yaml", SHA256: strings.Repeat("b2", 32), Confidence: detect.Certain}}
)

// fakeLedger is a read-only in-memory Ledger; it counts its calls.
type fakeLedger struct {
	mu      sync.Mutex
	entries map[string][]entries.Manifest
	rows    []store.CacheLocalityRow
	err     error
	reads   int
}

func (f *fakeLedger) CacheEntries(_ context.Context, key string) ([]entries.Manifest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return f.entries[key], f.err
}

func (f *fakeLedger) CacheLocality(context.Context, string, string, string) ([]store.CacheLocalityRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return f.rows, f.err
}

func runner(id, platform string, host bool) sched.Runner {
	r := sched.Runner{ID: id, CoordinatorHost: host}
	if platform != "" {
		os, arch, _ := strings.Cut(platform, "/")
		r.Capability.Platform.OS.Value, r.Capability.Platform.Arch.Value = &os, &arch
	}
	return r
}

// planned returns the realm and key of job j's own-namespace entry for spec on platform.
func planned(t testing.TB, j Job, platform, spec string) candidate {
	t.Helper()
	for _, p := range planJob(context.Background(), j, platform, false, j.Probe).specs {
		if p.spec == spec {
			return p.candidates[0]
		}
	}
	t.Fatalf("no planned key for spec %s", spec)
	return candidate{}
}

func entry(c candidate, size int64) entries.Manifest {
	ns, _ := base64.RawURLEncoding.DecodeString(c.realm)
	return entries.Manifest{Namespace: ns, Key: c.key, Writer: entries.Writer{ScannedClean: true},
		Blobs: []entries.Blob{{Path: "a", SHA256: strings.Repeat("0", 64), Size: size}}}
}

func job(key string, detected []detect.Fact) Job {
	return Job{Key: key, Facts: ownerFacts, Detected: detected, Image: imageDigest}
}

type fixture struct {
	w      sched.World
	in     Inputs
	ledger *fakeLedger
	goJob  Job
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	goJob := job("build[linux]", goFacts)
	goKey := planned(t, goJob, platformLinux, "go")
	ledger := &fakeLedger{entries: map[string][]entries.Manifest{goKey.key: {entry(goKey, goSize)}},
		rows: []store.CacheLocalityRow{
			{Node: "node-b", Realm: goKey.realm, Key: goKey.key, Bytes: goSize, LastSeen: time.UnixMilli(nowMs).Add(-time.Hour)},
			{Node: "node-d", Realm: goKey.realm, Key: goKey.key, Bytes: goSize, LastSeen: time.UnixMilli(nowMs).Add(-RowMaxAge - time.Minute)},
		}}
	w := sched.World{NowMs: nowMs, Runners: []sched.Runner{
		runner("coordinator", platformLinux, true), runner("node-b", platformLinux, false), runner("node-c", platformLinux, false),
		runner("node-d", platformLinux, false), runner("node-nop", "", false)}}
	return fixture{w: w, ledger: ledger, goJob: goJob, in: Inputs{Ledger: ledger, Jobs: []Job{goJob,
		job("docs", nil),                           // no spec at all
		job("web", pnpmFacts),                      // spec, but no entry anywhere
		job("both", append(goFacts, pnpmFacts...)), // go has an entry, pnpm does not
	}}}
}

func miss(f sched.CacheFact) string {
	if f.MissBytes == nil {
		return "nil/" + f.Confidence
	}
	return strconv.FormatInt(*f.MissBytes, 10) + "/" + f.Confidence
}

func build(t *testing.T, fx fixture) map[string]map[string]sched.CacheFact {
	t.Helper()
	w := fx.w
	unbind := Bind(func(context.Context) (Inputs, error) { return fx.in, nil })
	defer unbind()
	if err := Contribute(context.Background(), &w); err != nil {
		t.Fatal(err)
	}
	return w.Cache
}

func TestLocalityFacts(t *testing.T) {
	fx := newFixture(t)
	got := build(t, fx)
	want := map[string]map[string]string{
		"coordinator": {"build[linux]": "0/known", "docs": "nil/unknown", "web": "nil/unknown", "both": "0/estimated"},
		"node-b":      {"build[linux]": "0/known", "docs": "nil/unknown", "web": "nil/unknown", "both": "0/estimated"},
		"node-c":      {"build[linux]": "1500/known", "docs": "nil/unknown", "web": "nil/unknown", "both": "1500/estimated"},
		"node-d":      {"build[linux]": "1500/known", "docs": "nil/unknown", "web": "nil/unknown", "both": "1500/estimated"}, // copy older than RowMaxAge
		"node-nop":    {"build[linux]": "nil/unknown", "docs": "nil/unknown", "web": "nil/unknown", "both": "nil/unknown"},   // platform never reported
	}
	for runner, jobs := range want {
		for key, w := range jobs {
			if g := miss(got[runner][key]); g != w {
				t.Errorf("%s %s = %s, want %s", runner, key, g, w)
			}
		}
	}
	if len(got) != len(want) {
		t.Fatalf("runners in Cache: %d, want %d", len(got), len(want))
	}
}

func TestLocalityNoSourceLeavesCacheEmpty(t *testing.T) {
	w := sched.World{NowMs: nowMs, Runners: []sched.Runner{runner("a", platformLinux, true)}}
	if err := Contribute(context.Background(), &w); err != nil || w.Cache != nil {
		t.Fatalf("no source: cache=%v err=%v", w.Cache, err)
	}
	unbind := Bind(func(context.Context) (Inputs, error) { return Inputs{}, errors.New("boom") })
	defer unbind()
	if err := Contribute(context.Background(), &w); err != nil || w.Cache != nil {
		t.Fatalf("source error must leave every fact unknown, not fail the build: cache=%v err=%v", w.Cache, err)
	}
}

func TestLocalityLedgerErrorIsUnknownNeverZero(t *testing.T) {
	fx := newFixture(t)
	fx.ledger.err = errors.New("db locked")
	var warned []error
	fx.in.Warn = func(err error) { warned = append(warned, err) }
	got := build(t, fx)
	for runner, jobs := range got {
		for key, f := range jobs {
			if f.MissBytes != nil || f.Confidence != unknown {
				t.Errorf("%s %s = %+v, want unknown with nil", runner, key, f)
			}
		}
	}
	if len(warned) == 0 {
		t.Fatal("a ledger failure must be reported through Warn")
	}
}

func TestLocalityLocalRunnerHoldsTheStore(t *testing.T) {
	fx := newFixture(t)
	local := runner("local", platformLinux, false)
	local.Capability.Identity = model.CapabilityIdentity{ID: "local", Transport: "local", Provider: "local"}
	fx.w.Runners = []sched.Runner{local}
	if g := miss(build(t, fx)["local"]["build[linux]"]); g != "0/known" {
		t.Fatalf("local runner = %s, want 0/known", g)
	}
}

func TestLocalityProcessJobsKeyOnlyOnStoreHolders(t *testing.T) {
	// No image: the toolchain is a probe. Only the coordinator (holder) can run it; a node cannot.
	j := Job{Key: "build", Facts: ownerFacts, Detected: goFacts,
		Probe: func(context.Context, []string) (string, error) { return "go version go1.22.0 linux/amd64", nil }}
	key := planned(t, j, platformLinux, "go")
	fx := newFixture(t)
	fx.in = Inputs{Ledger: &fakeLedger{entries: map[string][]entries.Manifest{key.key: {entry(key, 9)}}}, Jobs: []Job{j}}
	fx.w.Runners = []sched.Runner{runner("coordinator", platformLinux, true), runner("node-c", platformLinux, false)}
	got := build(t, fx)
	if g := miss(got["coordinator"]["build"]); g != "0/known" {
		t.Errorf("coordinator = %s, want 0/known", g)
	}
	if g := miss(got["node-c"]["build"]); g != "nil/unknown" {
		t.Errorf("node without a probe = %s, want nil/unknown", g)
	}
}

func TestLocalityProbeRunsOncePerBuild(t *testing.T) {
	calls := 0
	probe := func(context.Context, []string) (string, error) { calls++; return "go version go1.22.0", nil }
	jobs := make([]Job, 20)
	for i := range jobs {
		jobs[i] = Job{Key: "j" + strconv.Itoa(i), Facts: ownerFacts, Detected: goFacts, Probe: probe}
	}
	fx := newFixture(t)
	fx.in = Inputs{Ledger: &fakeLedger{}, Jobs: jobs}
	fx.w.Runners = []sched.Runner{runner("coordinator", platformLinux, true)}
	build(t, fx)
	if calls != 1 {
		t.Fatalf("go probe ran %d times for 20 jobs, want 1", calls)
	}
}

// TestLocalitySnapshotAndRemoteRules: the namespaces a job reads follow specs.Session on a store
// holder (own, then snapshots for container jobs only) and remote.BuildPlan on a node (trusted:
// own only; untrusted: the owner namespace, container jobs only); a snapshot entry counts only
// when its writer facts permit a snapshot.
func TestLocalitySnapshotAndRemoteRules(t *testing.T) {
	fork := entries.Facts{Project: "/work/proj", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork/x", PR: "7"}
	internal := entries.Facts{Project: "/work/proj", Trust: "internal", Protection: "protected"}
	ownerKey := planned(t, Job{Facts: ownerFacts, Detected: goFacts, Image: imageDigest}, platformLinux, "go")
	cases := []struct {
		name      string
		facts     entries.Facts
		container bool
		holder    bool
		writer    entries.Writer
		want      string
	}{
		{"fork container job on a cold node reads the owner snapshot", fork, true, false, entries.Writer{ScannedClean: true}, "11/known"},
		{"fork process job on a node gets no cache", fork, false, false, entries.Writer{ScannedClean: true}, "nil/unknown"},
		{"snapshot written with private deps is not eligible", fork, true, false, entries.Writer{ScannedClean: true, PrivateDeps: true}, "nil/unknown"},
		{"snapshot written after secrets is not eligible", fork, true, false, entries.Writer{ScannedClean: true, SecretsReceived: true}, "nil/unknown"},
		{"snapshot never scanned clean is not eligible", fork, true, false, entries.Writer{}, "nil/unknown"},
		{"trusted node job reads its own namespace only", internal, true, false, entries.Writer{ScannedClean: true}, "nil/unknown"},
		{"internal container job on the store holder reads the owner snapshot", internal, true, true, entries.Writer{ScannedClean: true}, "0/known"},
		{"internal process job on the store holder reads no snapshot", internal, false, true, entries.Writer{ScannedClean: true}, "nil/unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := entry(ownerKey, 11)
			m.Writer = c.writer
			fx := newFixture(t)
			fx.in = Inputs{Ledger: &fakeLedger{entries: map[string][]entries.Manifest{ownerKey.key: {m}}},
				Jobs: []Job{{Key: "pr", Facts: c.facts, Detected: goFacts, Image: imageDigest, Container: c.container}}}
			fx.w.Runners = []sched.Runner{runner("r", platformLinux, c.holder)}
			if g := miss(build(t, fx)["r"]["pr"]); g != c.want {
				t.Fatalf("got %s, want %s", g, c.want)
			}
		})
	}
}

// A spec the plan could not key (its probe failed) is an unknown component: the fact over the
// other specs is an estimate, never "known".
func TestLocalityUnkeyableSpecIsEstimated(t *testing.T) {
	probe := func(_ context.Context, argv []string) (string, error) {
		if argv[0] == "go" {
			return "", errors.New("go not installed")
		}
		return "v1.0.0", nil
	}
	j := Job{Key: "both", Facts: ownerFacts, Detected: append(append([]detect.Fact{}, goFacts...), pnpmFacts...), Probe: probe}
	key := planned(t, j, platformLinux, "pnpm")
	fx := newFixture(t)
	fx.in = Inputs{Ledger: &fakeLedger{entries: map[string][]entries.Manifest{key.key: {entry(key, 5)}}}, Jobs: []Job{j}}
	fx.w.Runners = []sched.Runner{runner("coordinator", platformLinux, true)}
	if g := miss(build(t, fx)["coordinator"]["both"]); g != "0/estimated" {
		t.Fatalf("got %s, want 0/estimated", g)
	}
}

func TestLocalityDigestStable(t *testing.T) {
	fx := newFixture(t)
	a, b := build(t, fx), build(t, fx)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) || len(ja) < 10 {
		t.Fatalf("two builds differ:\n%s\n%s", ja, jb)
	}
}
