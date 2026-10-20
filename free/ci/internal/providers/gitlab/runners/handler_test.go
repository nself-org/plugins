package runners_test

// Purpose: the demand handler tests: the fixed tag set, the cap, the pinned image, the command and the
// synthetic pipeline a pending job becomes.
// Inputs:  hand-built trigger facts of the shape the Job Hook source produces.
// Outputs: pass or fail.
// Constraints: no network; a real store behind the runnerjob ledger; V15 mode set per test.

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

const (
	testImage = "docker.io/gitlab/gitlab-runner@sha256:77d03a5c881ff8f97381de4f96a66c080733eeab9c9cf502bd19916a18969113"
	simSHA    = "cccccccccccccccccccccccccccccccccccccccc"
)

// bind gives the test a fresh store and ledger and V15 mode, and returns the store.
func bind(t *testing.T) *store.Store {
	t.Helper()
	st, _ := bindAt(t)
	return st
}

// bindAt is bind that also returns the directory of the store.
func bindAt(t *testing.T) (*store.Store, string) {
	t.Helper()
	t.Setenv("NSELF_V15", "1")
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	runnerjob.Use(runnerjob.NewLedger(st, clock))
	t.Cleanup(func() { runnerjob.Use(nil); _ = st.Close() })
	return st, dir
}

// handler is a Handler with the default tags, the given cap and the fixed test image and instance.
func handler(maxPending int, tags ...string) *runners.Handler {
	if len(tags) == 0 {
		tags = []string{"nself"}
	}
	return &runners.Handler{
		Settings: func(context.Context) (runners.Settings, error) {
			return runners.Settings{Tags: tags, MaxPending: maxPending}, nil
		},
		Instance: func(context.Context) (string, string, error) {
			return "https://gitlab.example.com", "gitlab.example.com", nil
		},
		Image: func() (string, error) { return testImage, nil },
	}
}

func labelsFact(labels ...string) model.Fact[[]string] {
	return model.Fact[[]string]{Value: &labels, Source: "api", ObservedAt: epoch, Confidence: "known"}
}

func strFact(s string) model.Fact[string] {
	return model.Fact[string]{Value: &s, Source: "payload", ObservedAt: epoch, Confidence: "known"}
}

// demandFor is the facts of a pending job with the given tags, as the Job Hook source builds them.
func demandFor(job string, tags ...string) model.TriggerFacts {
	return model.TriggerFacts{Schema: "ci.trigger-facts/v1", Source: "gitlab", Kind: "runner_demand", DeliveryID: "d-1", ReceivedAt: epoch, Auth: "per-repo",
		Repo: testProject, Revision: simSHA, RevisionSource: "payload", RunnerLabels: labelsFact(tags...), ProviderJobID: strFact(job), ProviderRunID: strFact("3001")}
}

// TestFixedTags is declared in registrar_test.go for the registration; this one proves the demand half.
func TestDemandTagsAreAFixedSet(t *testing.T) {
	bind(t)
	ctx := context.Background()
	h := handler(50, "nself", "fast")
	for name, tags := range map[string][]string{
		"extra tag": {"nself", "ubuntu-latest"}, "no nself tag": {"fast"}, "no tags": {}, "case folded": {"NSELF"},
		"unicode fold": {"nself", "faıst"}, "reserved": {"nself", "nself-glr-01"},
	} {
		_, err := h.Synthesize(ctx, demandFor("4001", tags...))
		if code(err) != "E708" || reason(err) != "label.not_allowed" {
			t.Errorf("%s %v: want E708 label.not_allowed, got %v (%s)", name, tags, err, reason(err))
		}
	}
	for _, tags := range [][]string{{"nself"}, {"nself", "fast"}, {"fast", "nself"}} {
		if _, err := h.Synthesize(ctx, demandFor("4002", tags...)); err != nil {
			t.Errorf("%v: %v", tags, err)
		}
	}
}

// TestSynthesize: a pending job becomes one provider-runner pipeline whose job has the stdin-credential
// kind, the pinned image, the run-single command without a token, and the forge host for the network scope.
func TestSynthesize(t *testing.T) {
	bind(t)
	ctx := context.Background()
	p, err := handler(2).Synthesize(ctx, demandFor("4001", "nself"))
	if err != nil || p == nil {
		t.Fatalf("synthesize: %v", err)
	}
	j := p.Jobs[0]
	if !runnerjob.IsRunnerPipeline(p) || runnerjob.BandOf(p) != "background" || j.RunnerCredential != "gitlab-runner-token" || !j.RemoteOnly ||
		j.Container.Image != testImage || j.Isolation != "sandboxed-container" || len(j.Secrets) != 0 || p.ID != "provider-runner-gitlab-4001" {
		t.Fatalf("pipeline %+v", p)
	}
	if j.Env["NSELF_RUNNERJOB_PROVIDER"] != "gitlab" || j.Env["NSELF_RUNNERJOB_REPO"] != testProject || j.Env["NSELF_RUNNERJOB_DEMAND"] != "4001" ||
		j.Env["NSELF_RUNNERJOB_HOSTS"] != "gitlab.example.com" {
		t.Fatalf("env %v", j.Env)
	}
	cmd := strings.Join(j.Run, " ")
	for _, want := range []string{"gitlab-runner run-single", "--url https://gitlab.example.com", "--executor shell", "--max-builds 1", "--wait-timeout 120"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q lacks %q", cmd, want)
		}
	}
	for _, banned := range []string{"--token", "-t ", "glrt-", "register", "CI_SERVER_TOKEN"} {
		if strings.Contains(cmd, banned) {
			t.Errorf("command %q carries %q", cmd, banned)
		}
	}
	if a, err := handler(50).Synthesize(ctx, demandFor("4003", "nself")); err != nil || a.Jobs[0].Matrix.Platform != "linux/amd64" {
		t.Fatalf("default arch: %v", err)
	}
	if a, err := handler(50, "nself", "arm64").Synthesize(ctx, demandFor("4004", "nself", "arm64")); err != nil || a.Jobs[0].Matrix.Platform != "linux/arm64" {
		t.Fatalf("arm64: %v", err)
	}
}

// TestDemandCapAndRefusals: the cap is per project (E703, offered again), the handler answers E704 until
// the ledger is bound, and a wrong source, kind, facts, V14 mode or image pin are refusals.
func TestDemandCapAndRefusals(t *testing.T) {
	st := bind(t)
	ctx := context.Background()
	h := handler(2)
	for _, job := range []string{"1", "2"} {
		if _, err := h.Synthesize(ctx, demandFor(job, "nself")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.Synthesize(ctx, demandFor("3", "nself")); code(err) != "E703" || providers.ClassOf(err) != providers.Quota {
		t.Fatalf("third demand: want E703 quota, got %v", err)
	}
	if _, err := h.Synthesize(ctx, demandFor("1", "nself")); err != nil {
		t.Fatalf("an already reserved demand is not capped again: %v", err)
	}
	runnerjob.Use(nil)
	if _, err := h.Synthesize(ctx, demandFor("9", "nself")); code(err) != "E704" {
		t.Fatalf("unbound ledger: want E704, got %v", err)
	}
	runnerjob.Use(runnerjob.NewLedger(st, clock))
	bad := demandFor("5", "nself")
	bad.Source = "github"
	if _, err := h.Synthesize(ctx, bad); code(err) != "E702" {
		t.Fatalf("wrong source: %v", err)
	}
	bad = demandFor("5", "nself")
	bad.ProviderJobID = model.Fact[string]{}
	if _, err := h.Synthesize(ctx, bad); code(err) != "E702" {
		t.Fatalf("no job id: %v", err)
	}
	bad = demandFor("not-a-number", "nself")
	if _, err := h.Synthesize(ctx, bad); code(err) != "E702" {
		t.Fatalf("job id not digits: %v", err)
	}
	sub := demandFor("6", "nself")
	sub.Repo = "acme/sub/widgets" // a subgroup project is not configurable (HOST-21 ruling 2)
	if _, err := h.Synthesize(ctx, sub); code(err) != "E702" {
		t.Fatalf("subgroup project: %v", err)
	}
	noPin := handler(2)
	noPin.Image = func() (string, error) { return "", context.Canceled }
	if _, err := noPin.Synthesize(ctx, demandFor("7", "nself")); code(err) != "E700" {
		t.Fatalf("no pin: %v", err)
	}
	t.Setenv("NSELF_V15", "")
	if _, err := h.Synthesize(ctx, demandFor("8", "nself")); code(err) != "E700" {
		t.Fatalf("v14: %v", err)
	}
}

// TestHandlerIsRegistered: init registers the handler for source gitlab and the minter for its kind.
func TestHandlerIsRegistered(t *testing.T) {
	if errs := runners.RegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
	if providers.DemandHandlerFor("gitlab") == nil || runnerjob.KindOf("gitlab") != "gitlab-runner-token" {
		t.Fatal("the gitlab handler or its credential kind is not registered")
	}
	runnerjob.Use(nil)
	t.Setenv("NSELF_V15", "1")
	if _, err := providers.DemandHandlerFor("gitlab").Synthesize(context.Background(), demandFor("1", "nself")); code(err) != "E704" {
		t.Fatalf("the registered handler must answer E704 before the ledger is bound: %v", err)
	}
}

// TestRunnerImagePin proves acceptance 6 (the image pin, first half): the embedded table pins the image by
// digest and the fragment images.d/ci-gitlab-runner.json carries the same reference (the assemble check
// proves the rest).
func TestRunnerImagePin(t *testing.T) {
	ref, err := runners.PinnedImage()
	if err != nil || ref != testImage {
		t.Fatalf("pinned image %q %v", ref, err)
	}
	b, rerr := readFile(filepath.Join("..", "..", "..", "..", "..", "..", "images.d", "ci-gitlab-runner.json"))
	if rerr != nil || !strings.Contains(b, strings.Replace(ref, "gitlab-runner@", "gitlab-runner:v19.4.1@", 1)) {
		t.Fatalf("fragment %q %v", b, rerr)
	}
}

func TestInstanceOf(t *testing.T) {
	for in, want := range map[string]string{
		"https://gitlab.com/api/v4": "https://gitlab.com", "https://gitlab.example.com/api/v4/": "https://gitlab.example.com",
		"https://gitlab.example.com:443/api/v4": "https://gitlab.example.com:443", "https://git.lan/gitlab/api/v4": "https://git.lan/gitlab",
	} {
		got, host, ok := runners.InstanceOf(in)
		if !ok || got != want || host == "" || strings.Contains(host, ":") {
			t.Errorf("%s: %q %q %v", in, got, host, ok)
		}
	}
	for _, in := range []string{"http://gitlab.com/api/v4", "https://u:p@gitlab.com/api/v4", "https://gitlab.com/api/v4?x=1", "", "gitlab.com",
		"https://10.0.0.5/api/v4", "https://[::1]/api/v4", "https://git.lan:8443/api/v4", "https://git.lan:80/api/v4"} {
		if _, _, ok := runners.InstanceOf(in); ok {
			t.Errorf("%q must be refused", in)
		}
	}
}

// TestInstanceFromPolicy (Opus should-fix): an instance the restricted network scope cannot reach is refused
// before any runner is minted: a private base, an IP literal, a port other than 443.
func TestInstanceFromPolicy(t *testing.T) {
	ctx := context.Background()
	t.Cleanup(func() { runtime.UsePolicy(nil) })
	use := func(kv map[string]string) {
		reg, err := trust.NewRegistry(runtime.Keys()...)
		if err != nil {
			t.Fatal(err)
		}
		eff, _, err := reg.Merge("", scope(trust.SourceUser, kv))
		if err != nil {
			t.Fatal(err)
		}
		runtime.UsePolicy(func(context.Context) (*policy.Effective, error) { return &eff, nil })
	}
	runtime.UsePolicy(nil)
	if inst, host, err := runners.InstanceFromPolicy(ctx); err != nil || inst != "https://gitlab.com" || host != "gitlab.com" {
		t.Fatalf("defaults: %q %q %v", inst, host, err)
	}
	use(map[string]string{runtime.KeyGitLabAPIURL: `"https://git.example.com/api/v4"`})
	if inst, _, err := runners.InstanceFromPolicy(ctx); err != nil || inst != "https://git.example.com" {
		t.Fatalf("operator URL: %q %v", inst, err)
	}
	for name, kv := range map[string]map[string]string{
		"private base": {runtime.KeyGitLabAPIURL: `"https://git.example.com/api/v4"`, runtime.KeyGitLabAllowPrivateBase: `true`},
		"ip literal":   {runtime.KeyGitLabAPIURL: `"https://10.1.2.3/api/v4"`},
		"odd port":     {runtime.KeyGitLabAPIURL: `"https://git.example.com:8443/api/v4"`},
	} {
		use(kv)
		if _, _, err := runners.InstanceFromPolicy(ctx); code(err) != "E700" || reason(err) != "runner.instance_unreachable" {
			t.Errorf("%s: want E700 runner.instance_unreachable, got %v", name, err)
		}
	}
}
