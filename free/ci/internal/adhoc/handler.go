package adhoc

// Purpose: the AdhocHandler of the run package (P7-CI-37 hook, Epic D22):
// `nself ci [flags] -- <cmd> [args…]` becomes a one-job pipeline named adhoc
// and goes through the normal Coordinator: same run loop, executor, events,
// logs and evidence. Nothing here creates an attempt (run.CreateAttempt does,
// so a registered run.Dispatch sees it) and nothing here starts a process.
// Inputs:  run.Options (parsed by cmd), the argv after `--`, the Flags cmd
// stored with SetFlags, the registered hooks.
// Outputs: the exit code of the run (0 pass, 1 the command failed, 2 infra).
// Constraints: the argv is exec'd as given, never through a shell; the job
// env is the minimal policy; the evidence is kind adhoc (a one-job run), so it
// never satisfies a gate; the process-wide redaction registry learns the
// secret-looking argv values before the job starts.

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/services"
)

// JobName is the id of the one job (and the name of the pipeline).
const JobName = "adhoc"

// DefaultTimeout bounds a job that has no --timeout.
const DefaultTimeout = "30m"

// ProcessWarning is the one line printed for --isolation process.
const ProcessWarning = "nself-ci: warning: --isolation process runs the command on this host with the host network and a writable copy of the workspace; it is not a sandbox"

var flags Flags

// SetFlags stores the ad-hoc flags of the current invocation (cmd/v15_adhoc.go).
// Trust is the --trust value: run.Options.Validate would demand a registered
// classifier for it, so the cmd hook moves it here and the handler enforces it.
func SetFlags(f Flags) { flags = f }

// Handler implements run.AdhocHandler. The zero value is the production one;
// tests set the fields.
type Handler struct {
	Hooks  cirun.Hooks              // unresolved extra hooks; the registered ones fill the rest
	Flags  *Flags                   // nil: the flags SetFlags stored
	Select Selector                 // nil: the isolation registry
	Docker services.Runner          // nil: the exec funnel
	Git    cirun.Capture            // git and gh calls (trust, revision); nil: report.Capture
	Tune   func(*cirun.Coordinator) // test seam: adjust the Coordinator before it runs
	Stdout io.Writer                // the run's output; nil: os.Stdout
	Stderr io.Writer                // the process warning; nil: os.Stderr
}

func init() { cirun.RegisterAdhoc(&Handler{}) }

// Adhoc runs argv as the one job.
func (h *Handler) Adhoc(ctx context.Context, o cirun.Options, argv []string) (int, error) {
	if len(argv) == 0 {
		return 1, usage("`nself ci --` needs a command after the dashes")
	}
	fl := flags
	if h.Flags != nil {
		fl = *h.Flags
	}
	base := h.Hooks.Resolved()
	trust, err := h.trustClass(ctx, o, fl, base)
	if err != nil {
		return 1, err
	}
	sel := h.Select
	if sel == nil {
		sel = RegistrySelector
	}
	mode, err := Resolve(ctx, o, fl, trust, sel)
	if err != nil {
		return 1, err
	}
	image := ""
	if mode.Level != "process" {
		if image, err = containerImage(ctx, base); err != nil {
			return 1, err
		}
	}
	home := stateHome(o)
	scratch := ""
	if mode.Level == "process" {
		var drop func()
		if scratch, drop, err = makeScratch(home); err != nil {
			return 1, err
		}
		defer drop()
		fmt.Fprintln(or(h.Stderr, os.Stderr), ProcessWarning)
	}
	if n := Register(cirun.Redactions, argv); n > 0 {
		fmt.Fprintf(or(h.Stderr, os.Stderr), "nself-ci: note: %d secret-looking argument value(s) are shorter than the redaction floor; they are masked in the recorded command line but not in job output\n", n)
	}
	pl, err := buildPlan(argv, mode, image)
	if err != nil {
		return 1, err
	}
	docker := h.Docker
	if docker == nil {
		docker = &services.Funnel{Home: home}
	}
	net := &serviceNet{docker: docker}
	defer net.Remove(ctx)
	hooks := h.Hooks
	// The handler classified the source and chose the isolation level above. The
	// coordinator's own check refuses any class but owner/internal (it runs jobs as
	// the operator) and a nil hook is refilled from the registered one, so it gets a
	// classifier that says owner: the operator typed this argv. The requested class
	// still reaches the evidence (source_trust) through evidenceStamp.
	hooks.Trust = operatorTyped{}
	hooks.Isolation = &isolator{inner: base.Isolation, mode: mode, scratch: scratch, net: net}
	hooks.Evidence = evidenceStamp{inner: base.Evidence, mode: mode, argv: Recorded(cirun.Redactions, argv), image: image}
	co := o
	co.Adhoc, co.Trust, co.Container, co.Network, co.Job, co.Pipeline = nil, "", false, "", JobName, ""
	c := &cirun.Coordinator{Opts: co, Hooks: hooks, Plan: pl, ConfigDigest: "builtin-defaults", Out: or(h.Stdout, os.Stdout), Capture: h.Git}
	if h.Tune != nil {
		h.Tune(c)
	}
	res, err := c.Run(ctx)
	return res.Exit, err
}

// operatorTyped is the coordinator's classifier for an ad-hoc job: the argv came
// from the operator, and the handler already enforced the isolation the requested
// class needs.
type operatorTyped struct{}

func (operatorTyped) Classify(context.Context, cirun.TrustRequest) (model.TrustClass, error) {
	return "owner", nil
}

func or(w, def io.Writer) io.Writer {
	if w == nil {
		return def
	}
	return w
}

// trustClass is the stricter of --trust and the registered classifier's verdict.
func (h *Handler) trustClass(ctx context.Context, o cirun.Options, fl Flags, base cirun.Hooks) (model.TrustClass, error) {
	t := model.TrustClass(fl.Trust)
	if t != "" && !t.Valid() {
		return "", usage("unknown --trust class %q (owner, collaborator, internal, untrusted)", fl.Trust)
	}
	if base.Trust == nil {
		return t, nil
	}
	capture := h.Git
	if capture == nil {
		capture = report.Capture
	}
	rev := cirun.GitRevision(ctx, o.Root, capture)
	got, err := base.Trust.Classify(ctx, cirun.TrustRequest{Root: o.Root, Revision: rev.SHA, Requested: fl.Trust})
	if err != nil {
		return "", err
	}
	if !got.Valid() {
		return "", usage("the trust classifier returned %q, which is not a trust class; nothing was run", got)
	}
	if rank(got) > rank(t) {
		t = got
	}
	return t, nil
}

func rank(t model.TrustClass) int {
	switch t {
	case "collaborator":
		return 1
	case "untrusted":
		return 2
	}
	return 0
}

// containerImage resolves the image through the registered hook: the --image
// value (NSELF_CI_IMAGE) or the pinned toolchain image, E645 while that is pending.
func containerImage(ctx context.Context, base cirun.Hooks) (string, error) {
	if base.Image == nil {
		return "", noIsolation("no container image hook is registered; pass --image <ref@digest>")
	}
	return base.Image.Image(ctx, cirun.AttemptInfo{JobName: JobName})
}

// buildPlan makes the one-job plan through the normal planner.
func buildPlan(argv []string, m Mode, image string) (*plan.Plan, error) {
	required, idempotent := true, false
	job := model.Job{Kind: "test", Run: append([]string(nil), argv...), Required: &required, Idempotent: &idempotent,
		Timeout: DefaultTimeout, Isolation: m.Level}
	if m.Level != "process" {
		job.Container = &model.ContainerConfig{Image: image}
	}
	pl, err := plan.Build(plan.Input{
		Config:  model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{JobName: job}},
		World:   plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, Docker: m.Level != "process"},
		Trigger: "local",
	})
	if err != nil {
		return nil, err
	}
	pl.Pipeline = model.PipelineRef{ID: JobName, Name: JobName, Kind: "adhoc"}
	return pl, nil
}

func stateHome(o cirun.Options) string {
	if o.Home != "" {
		return o.Home
	}
	if h := os.Getenv("NSELF_CI_HOME"); h != "" {
		return h
	}
	u, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "nself-ci")
	}
	return filepath.Join(u, ".nself", "ci")
}

// makeScratch creates the private HOME/TMPDIR of a process job.
func makeScratch(home string) (string, func(), error) {
	base := filepath.Join(home, "adhoc")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", nil, usage("cannot create the scratch directory: %v", err)
	}
	dir, err := os.MkdirTemp(base, "s-")
	if err != nil {
		return "", nil, usage("cannot create the scratch directory: %v", err)
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}
