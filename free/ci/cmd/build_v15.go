package main

// Purpose: `nself ci build --artifact android [dir]` under V15 (P7-CACHE-15,
// Epic CACHE D18): a release pipeline on the operator's own machine. The
// project's required jobs run first, then the `android` release job (the
// project's own `kind: release` job of that name, else the synthesized
// pipeline `android-release`); the run publishes the signed release directory
// (P7-CACHE-18) and this command prints the APK path and that directory.
// Inputs:  the v1.4 argv: --artifact android [dir] --upload --tag --owner --repo
// --timeout -v/--verbose; env NSELF_CI_REPO, NSELF_CI_HOME.
// Outputs: the run output, then "APK:" and "Release:" lines; exit 0, 1 (the
// run, the publication or the upload failed, or a usage error), 2 (flags).
// Constraints: V15 only and hidden from help (like `ci run`), removed with the
// v1.4 dispatch at v1.6; the key `build` in the v1.4 table stays frozen; side-
// effect class: remote with --upload (gh), write otherwise; the keystore is
// reached only through the release/ANDROID_KEYSTORE_* compat source on this
// machine; `--upload` is the explicit publish step and never overwrites; the
// APK uploaded is the verified copy in the release directory.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/drill"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/release/android"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sbom"
)

// buildCapture is the exec funnel `--upload` runs gh through (a test replaces it).
var buildCapture android.Capture = report.Capture

// newSBOMStep builds the SBOM step of a run (a test replaces it with a fake syft runner).
var newSBOMStep = func(home string, pin sbom.Pin) android.SBOMStep { return android.SBOMStep{Home: home, Pin: pin} }

func init() {
	registerV15("build", subcommandEntry{V15Only: true, Hidden: true, Handler: runBuildV15,
		Flags: func() *flag.FlagSet { return buildV15Flags(&buildOpts{}, flag.ContinueOnError) }})
}

// buildV15Flags declares the unchanged v1.4 flags of `build`.
func buildV15Flags(o *buildOpts, eh flag.ErrorHandling) *flag.FlagSet {
	fs := flag.NewFlagSet("build", eh)
	fs.StringVar(&o.artifact, "artifact", "android", "artifact type to build (only \"android\")")
	fs.BoolVar(&o.upload, "upload", false, "attach the APK to a GitHub release (never overwrites an asset)")
	fs.StringVar(&o.tag, "tag", "", "release tag to upload to (required with --upload)")
	fs.StringVar(&o.owner, "owner", "", "GitHub owner for --upload (default: from the git remote)")
	fs.StringVar(&o.repo, "repo", "", "GitHub repo for --upload (default: from the git remote)")
	fs.BoolVar(&o.verbose, "v", false, "accepted for v1.4 argv; the run output is always printed")
	fs.BoolVar(&o.verbose, "verbose", false, "alias for -v")
	fs.IntVar(&o.timeout, "timeout", 900, "timeout of the synthesized android job, in seconds")
	return fs
}

// buildLane is a planned Android lane.
type buildLane struct {
	plan        *plan.Plan
	pipeline    string
	synthesized bool
	digest      string // the config digest the release gate compares (attest, of the committed config)
	subst       func(plan.PlannedJob) bool
}

// planBuildLane resolves and plans the lane for the Android project at workdir.
func planBuildLane(ctx context.Context, root, workdir string, timeout time.Duration) (*buildLane, error) {
	sha, ok := planGit(ctx, root, "rev-parse", "HEAD")
	if sha = strings.TrimSpace(sha); !ok || sha == "" {
		return nil, errors.New("the Android lane needs a git repository with a commit: the release is bound to the revision")
	}
	bound, err := evidence.InputsAt(ctx, report.Capture, root, sha)
	if err != nil {
		return nil, err
	}
	cfg := model.PipelineConfig{Version: 1}
	loaded, err := config.Load(filepath.Join(root, config.FileName))
	switch {
	case err == nil:
		cfg = loaded.Config
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}
	facts, checks, err := planChecks(root, cfg)
	if err != nil {
		return nil, err
	}
	build := func(c model.PipelineConfig, pipeline string) (*plan.Plan, error) {
		if c, err = drill.Expand(root, c, pipeline, ""); err != nil {
			return nil, err
		}
		rev := planRevision(ctx, root, "")
		rev.BaseKnown, rev.Changed = false, nil // a release plans the whole pipeline
		return plan.Build(plan.Input{Config: c, Facts: facts, Checks: checks, Revision: rev, World: planWorld(c, checks), Pipeline: pipeline})
	}
	base, err := build(cfg, "")
	if err != nil {
		return nil, err
	}
	lane, err := android.Resolve(android.Input{Config: cfg, Workdir: workdir, Timeout: timeout, Required: android.RequiredJobs(base), Env: android.EnvFrom(os.Getenv)})
	if err != nil {
		return nil, err
	}
	p, err := build(lane.Config, lane.Pipeline)
	if err != nil {
		return nil, err
	}
	real := map[string]bool{}
	for _, c := range checks {
		real[c.ID] = c.Substantive
	}
	return &buildLane{plan: p, pipeline: lane.Pipeline, synthesized: lane.Synthesized, digest: bound.Config,
		subst: func(pj plan.PlannedJob) bool { return pj.Check == "" || real[pj.Check] }}, nil
}

func runBuildV15(args []string) int {
	var o buildOpts
	pos, err := parseInterspersed(buildV15Flags(&o, flag.ContinueOnError), args)
	if errors.Is(err, flag.ErrHelp) {
		return 0 // v1.4 exited 0 on -h
	}
	if err != nil {
		return 2
	}
	if o.dir = "."; len(pos) > 0 {
		o.dir = pos[0]
	}
	switch {
	case o.upload && o.tag == "":
		fmt.Fprintln(os.Stderr, "Error: --tag is required with --upload")
		return 1
	case o.artifact != "android":
		fmt.Fprintf(os.Stderr, "error: --artifact %q not supported (only \"android\")\n", o.artifact)
		return 1
	}
	if o.timeout < 1 || o.timeout > 24*3600 {
		fmt.Fprintln(os.Stderr, "error: --timeout must be between 1 and 86400 seconds")
		return 1
	}
	if o.upload { // fail before the build, not after it: the tag always, owner and repo when given
		if err = android.CheckTarget(firstOf(o.owner, "x"), firstOf(o.repo, "x"), o.tag); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	root, err := evidenceRoot("")
	if os.Getenv("NSELF_CI_REPO") == "" && err == nil { // run from a subdirectory: the release is bound to the repository
		if top, ok := planGit(ctx, root, "rev-parse", "--show-toplevel"); ok {
			root = strings.TrimSpace(top)
		}
	}
	dir, derr := filepath.Abs(o.dir)
	rel, rerr := relInside(root, dir)
	if err != nil || derr != nil || rerr != nil {
		fmt.Fprintf(os.Stderr, "error: %s is not inside the repository %s (run from the repository or set NSELF_CI_REPO)\n", o.dir, root)
		return 1
	}
	lane, err := planBuildLane(ctx, root, filepath.ToSlash(rel), time.Duration(o.timeout)*time.Second)
	if err != nil {
		return exitForError(err)
	}
	pin, err := sbom.EmbeddedPin()
	if err != nil {
		return exitForError(err)
	}
	defer android.Install(newSBOMStep(os.Getenv("NSELF_CI_HOME"), pin))()
	fmt.Printf("nself-ci build --artifact android — %s (pipeline %s)\n", dir, lane.pipeline)
	opts := RunOptions{Root: root, Home: os.Getenv("NSELF_CI_HOME"), Pipeline: lane.pipeline, NoStatus: true, Full: true}
	c := &cirun.Coordinator{Opts: opts, Plan: lane.plan, ConfigDigest: lane.digest, Substantive: lane.subst, Out: os.Stdout}
	res, err := c.Run(ctx)
	if err != nil {
		return exitForError(err)
	}
	if res.Exit != 0 {
		return res.Exit
	}
	return finishBuild(ctx, o, root, res.RunID)
}

// finishBuild prints the published APKs and the release directory, then uploads when asked.
func finishBuild(ctx context.Context, o buildOpts, root, runID string) int {
	relDir, ok, err := publishedDir(ctx, runID)
	if err != nil || !ok {
		fmt.Fprintf(os.Stderr, "error: run %s passed but no release directory was published; recover with: nself ci artifacts sign %s\n", runID, runID)
		return 1
	}
	apks, _ := filepath.Glob(filepath.Join(relDir, "artifacts", android.ArtifactName, "*.apk"))
	if len(apks) == 0 {
		fmt.Fprintf(os.Stderr, "error: the release %s holds no APK\n", relDir)
		return 1
	}
	for _, a := range apks {
		fmt.Printf("APK: %s\n", a)
	}
	fmt.Printf("Release: %s\n", relDir)
	if !o.upload {
		return 0
	}
	return uploadBuiltV15(ctx, o, root, apks)
}

// uploadBuiltV15 is the explicit publish step: gh release upload, never --clobber.
func uploadBuiltV15(ctx context.Context, o buildOpts, root string, apks []string) int {
	owner, repo := o.owner, o.repo
	if owner == "" || repo == "" {
		url, _ := planGit(ctx, root, "remote", "get-url", "origin")
		ro, rr, ok := android.ParseRemote(url)
		if !ok {
			fmt.Fprintln(os.Stderr, "error: cannot resolve the GitHub remote for --upload\nhint: pass --owner and --repo")
			return 1
		}
		owner, repo = firstOf(owner, ro), firstOf(repo, rr)
	}
	if err := android.Upload(ctx, buildCapture, owner, repo, o.tag, apks); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("✓ Uploaded %d APK(s) to %s/%s@%s\n", len(apks), owner, repo, o.tag)
	return 0
}

// relInside returns dir relative to root when dir is root or below it; symlinks (/var on macOS) are resolved first.
func relInside(root, dir string) (string, error) {
	r, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	d, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(r, d)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("outside the repository")
	}
	return rel, nil
}
