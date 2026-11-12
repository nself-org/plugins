package main

// Purpose: P7-WIRE-02 acceptance. The project id a run places with is the one a node is authorized
// for (policy.ProjectIDGit locally, the forge repo name under serve), so a linked worktree or a
// symlinked path finds the node `ci plan` reports eligible, and a node authorized for another
// project stays ineligible. Every case drives the real V15 entry by argv; the nodes are rows in
// the state store the run opens.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const wire02Node = "w02n1"

// wire02Repo is a git repository with the planview fixture committed, a state home holding one online node
// authorized for the given project ids, and the environment pointing the V15 entry at both.
func wire02Repo(t *testing.T, authorized func(repo string) []string) (repo, home string) {
	t.Helper()
	repo, home = placementRepo(t)
	t.Setenv("NSELF_V15", "1")
	t.Setenv("HOME", t.TempDir())
	oldWhere := whereValue
	t.Cleanup(func() { whereValue = oldWhere })
	ctx := context.Background()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	c, err := planview.LocalCapability(planview.LocalFacts{Project: repo, Platform: runtime.GOOS + "/" + runtime.GOARCH, CPUs: 8, MemMB: 8192, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	lan, owned, uid := "lan", "owned", true
	c.Identity.ID, c.Identity.Name, c.Identity.Provider, c.Identity.Transport = wire02Node, wire02Node, "ssh", "ssh"
	c.Identity.SSH = &model.CapabilitySSH{Hostname: "w02.invalid", Port: 22, User: "ci"}
	c.SecretsEligible = true
	c.Location.Kind.Value, c.Economics.Class.Value = &lan, &owned
	c.Separation.UIDSeparation = model.Fact[bool]{Value: &uid, Source: "self", ObservedAt: time.Now(), Confidence: "known"}
	c.Lifecycle.AuthorizedProjects = authorized(repo)
	doc, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.NodePut(ctx, store.NodeRecord{ID: wire02Node, Name: wire02Node, State: "offline", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"), Capability: doc, ObservedAt: time.Now().UnixNano()}, "test"); err != nil {
		t.Fatal(err)
	}
	admin, err := store.NewAdminAuthority("test")
	if err != nil {
		t.Fatal(err)
	}
	as := func(v model.Isolation) model.Fact[model.Isolation] {
		return model.Fact[model.Isolation]{Value: &v, Source: "assigned", ObservedAt: time.Now(), Confidence: "known"}
	}
	acc, net, sec, zone := []model.TrustClass{"owner", "internal"}, []model.NetworkScope{"none", "internet", "restricted", "lan", "privileged"}, []model.SecretClass{"none", "project"}, model.PrivacyZone("private-infrastructure")
	trust := model.CapabilityTrust{Isolation: as("container"),
		Accepts:       model.Fact[[]model.TrustClass]{Value: &acc, Source: "assigned", ObservedAt: time.Now(), Confidence: "known"},
		Network:       model.Fact[[]model.NetworkScope]{Value: &net, Source: "assigned", ObservedAt: time.Now(), Confidence: "known"},
		SecretClasses: model.Fact[[]model.SecretClass]{Value: &sec, Source: "assigned", ObservedAt: time.Now(), Confidence: "known"},
		PrivacyZone:   model.Fact[model.PrivacyZone]{Value: &zone, Source: "assigned", ObservedAt: time.Now(), Confidence: "known"}}
	reg := registry.New(st)
	if _, err = reg.SetTrust(ctx, wire02Node, trust, admin); err != nil {
		t.Fatal(err)
	}
	if _, err = reg.SetState(ctx, wire02Node, "online", "", "test"); err != nil {
		t.Fatal(err)
	}
	return repo, home
}

func wire02GitID(t *testing.T, dir string) string {
	t.Helper()
	return policy.ProjectIDGit(context.Background(), policy.Runner(report.Capture), dir)
}

// wire02Run runs the gate from dir on the node and returns its stderr and stdout.
func wire02Run(t *testing.T, dir string) string {
	t.Helper()
	t.Setenv("NSELF_CI_REPO", dir)
	resetPlanFlags()
	_, out, errs := capture(t, func() int { return runV15([]string{"--where", wire02Node, "--no-status"}) })
	return out + errs
}

func wire02Refused(out string) bool {
	return strings.Contains(out, "not authorized") || strings.Contains(out, "node.discovered") || strings.Contains(out, "no eligible")
}

func TestWire02LinkedWorktreeEligible(t *testing.T) {
	repo, _ := wire02Repo(t, func(repo string) []string { return []string{wire02GitID(t, repo)} })
	if out := wire02Run(t, repo); wire02Refused(out) {
		t.Fatalf("main checkout: node refused as unauthorized (control): %s", out)
	}
	wt := filepath.Join(t.TempDir(), "linked")
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", "-q", "--detach", wt).CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %v %s", err, out)
	}
	link := filepath.Join(t.TempDir(), "sym")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	for name, dir := range map[string]string{"linked worktree": wt, "symlinked path": link} {
		if got, want := wire02GitID(t, dir), wire02GitID(t, repo); got != want {
			t.Fatalf("%s: project id %q != main %q", name, got, want)
		}
		if out := wire02Run(t, dir); wire02Refused(out) {
			t.Errorf("%s: the run placed with a project id the node is not authorized for: %s", name, out)
		}
	}
}

func TestWire02UnauthorizedProjectRefused(t *testing.T) {
	repo, _ := wire02Repo(t, func(string) []string { return []string{"some/other-project"} })
	if out := wire02Run(t, repo); !wire02Refused(out) {
		t.Fatalf("a node authorized for another project was not refused: %s", out)
	}
}

// wire02Serve dispatches one queued github trigger for repoName through the real serve dispatcher. Only
// the clone (a network fetch) and the forge trust lookup (a forge API call) are replaced: the clone is
// a temp checkout of the fixture repo, so its path is nothing like the forge repo name.
func wire02Serve(t *testing.T, repoName string, authorized []string) string {
	t.Helper()
	src, home := wire02Repo(t, func(string) []string { return authorized })
	sha := strings.TrimSpace(wire02Git(t, src, "rev-parse", "HEAD"))
	clone, run := dispatchCloneRef, dispatchRun
	t.Cleanup(func() { dispatchCloneRef, dispatchRun = clone, run })
	dispatchCloneRef = func(string, string, string) (string, error) {
		dir := filepath.Join(t.TempDir(), "clone")
		wire02Git(t, "", "clone", "-q", src, dir)
		return dir, nil
	}
	dispatchRun = func(ctx context.Context, c *cirun.Coordinator) (cirun.Result, error) {
		c.Hooks.Trust = wire02Owner{}
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		return c.Run(ctx)
	}
	raw, err := json.Marshal(model.TriggerFacts{Schema: "ci.trigger-facts/v1", Source: "github", Repo: repoName, Revision: sha, Kind: "push", Auth: "per-repo"})
	if err != nil {
		t.Fatal(err)
	}
	row := store.IntakeRow{Source: "github", Project: repoName, Revision: sha, Facts: raw}
	var res coordinator.RunResult
	var derr error
	_, so, se := capture(t, func() int {
		res, derr = dispatchPipeline(home, &v15ServeFlags{timeout: 20, verbose: true})(context.Background(), row)
		return 0
	})
	out := ""
	if derr != nil {
		out = derr.Error()
	}
	return out + " " + fmt.Sprint(res) + so + se
}

type wire02Owner struct{}

func (wire02Owner) Classify(context.Context, cirun.TrustRequest) (model.TrustClass, error) {
	return "owner", nil
}

func wire02Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return string(out)
}

func TestWire02ServeCloneUsesRepoName(t *testing.T) {
	out := wire02Serve(t, "acme/widgets", []string{"acme/widgets"})
	if !strings.Contains(out, wire02Node+":") || strings.Contains(out, "node.discovered") {
		t.Fatalf("a serve job from a temp clone was not eligible on a node authorized for its forge repo: %s", out)
	}
}

func TestWire02ServeUnauthorizedRepoRefused(t *testing.T) {
	out := wire02Serve(t, "acme/widgets", []string{"acme/other"})
	if !strings.Contains(out, "node.discovered") {
		t.Fatalf("a node authorized for another repo was not refused: %s", out)
	}
}
