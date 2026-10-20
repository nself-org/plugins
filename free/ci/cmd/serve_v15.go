package main

// Purpose: `serve` in V15 mode (P7-NODE-10, Epic D7): the TEAM coordinator. It keeps every v1.4 flag and
// environment variable, adds --data-dir and --force, defaults --addr to 127.0.0.1:3845, refuses to start
// on a deploy host before any port is bound, stores every accepted trigger durably and runs each queued
// pipeline through the P7-CI-37 orchestrator on a fresh checkout.
// Inputs:  argv after `serve`; env GITHUB_WEBHOOK_SECRET, NSELF_CI_ALLOWED_REPOS, NSELF_CI_ALLOW_UNSANDBOXED,
//          NSELF_CI_HOME; flags that coordinator.RegisterListener and RegisterFlag add.
// Outputs: HTTP on the webhook address until SIGINT/SIGTERM; exit 0, 1 on error, 2 on usage.
// Constraints: registered with V15Only, so the v1.4 `serve` entry and its argv stay frozen; the fail-closed
//          defaults (secret, allowlist, sandbox) are unchanged; the start guard (E663) runs before the
//          store is opened and before anything is bound.

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/backup"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/serve"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// v15ServeAddr is the V15 default webhook address: loopback, so a reverse proxy or tunnel fronts it.
const v15ServeAddr = "127.0.0.1:3845"

// serveGuardLocal reads this host and serveGuardHome is the CI home the guard reads; tests replace both
// so the guard never reads the developer's machine.
var (
	serveGuardLocal = func() deployguard.Local { return deployguard.NewLocal(deployguard.FunnelRun) }
	serveGuardHome  = ""
	// serveDispatch builds the Dispatcher; tests replace it to skip the real checkout.
	serveDispatch func(dataDir string, f *v15ServeFlags) coordinator.Dispatcher = dispatchPipeline
)

func init() {
	registerV15("serve", subcommandEntry{V15Only: true, Handler: runServeV15,
		Flags: func() *flag.FlagSet { fs, _, _ := newServeV15Flags(); return fs }})
}

// v15ServeFlags are the values of the V15 serve flags.
type v15ServeFlags struct {
	addr, secret, workdir, dataDir string
	concurrency, timeout           int
	verbose, insecure, unsandboxed bool
	force                          bool
	allowed                        listFlag
	listenerAddrs                  func() map[string]string
}

// newServeV15Flags builds the flag set: the v1.4 flags first, then the V15 ones, then every registered
// listener and flag.
func newServeV15Flags() (*flag.FlagSet, *v15ServeFlags, error) {
	f := &v15ServeFlags{}
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.StringVar(&f.addr, "addr", v15ServeAddr, "Listen address (host:port). Port 3845 = nself-ci-serve per F10 port registry")
	fs.StringVar(&f.secret, "secret", "", "HMAC-SHA256 webhook secret (overrides GITHUB_WEBHOOK_SECRET env)")
	fs.IntVar(&f.concurrency, "concurrency", 2, "Max concurrent CI jobs")
	fs.StringVar(&f.workdir, "workdir", "/tmp/nself-ci-workdirs", "Base directory for ephemeral checkout workdirs")
	fs.IntVar(&f.timeout, "timeout", 600, "Per-job timeout in seconds")
	fs.BoolVar(&f.verbose, "v", false, "Verbose gate output")
	fs.BoolVar(&f.verbose, "verbose", false, "Alias for -v (core spelling)")
	fs.BoolVar(&f.insecure, "insecure", false, "DANGEROUS: start without a webhook secret (disables signature verification).")
	fs.BoolVar(&f.unsandboxed, "allow-unsandboxed", false, "DANGEROUS: run the gate directly on this host when Docker is unavailable, instead of refusing the job.")
	fs.Var(&f.allowed, "allowed-repos", "Comma-separated or repeatable \"owner/repo\" allowlist.")
	fs.StringVar(&f.dataDir, "data-dir", "", "Coordinator state directory (state.db); default NSELF_CI_HOME or ~/.nself/ci")
	fs.BoolVar(&f.force, "force", false, "Run every accepted trigger, even when an identical pipeline is live (audited)")
	f.listenerAddrs = coordinator.AddFlags(fs)
	return fs, f, nil
}

// runServeV15 is the V15 `serve` handler.
func runServeV15(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serveV15(ctx, args, nil)
}

// serveV15 parses args and runs the coordinator until ctx ends. hook, when set, may adjust the
// coordinator Config before it starts (tests inject a listener and a clock); production passes nil.
func serveV15(ctx context.Context, args []string, hook func(*coordinator.Config)) int {
	fs, f, _ := newServeV15Flags()
	fs.SetOutput(os.Stderr)
	if _, err := parseInterspersed(fs, args); err != nil { // flags after a positional argument still count, as in v1.4
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	report.SetProcessRole(report.RoleTeam)
	home := records.StoreDir(f.dataDir)
	warns, err := deployguard.StartGuard(ctx, firstOf(serveGuardHome, home), serveGuardLocal(), "nself ci serve")
	for _, w := range warns {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if backup.Pending(home) { // a restore began here and did not finish: the dir may hold a half-restored state
		fmt.Fprintf(os.Stderr, "Error: E668: an earlier `nself ci serve restore` did not finish on %s; run `nself ci serve restore` again, or undo the restore by hand (the printed rollback line also removes %s) and delete that file, before starting the coordinator (incomplete_restore)\n", home, filepath.Join(home, "restore.pending"))
		return 1
	}
	secret := firstOf(f.secret, os.Getenv("GITHUB_WEBHOOK_SECRET"))
	if secret == "" && f.insecure {
		fmt.Fprintln(os.Stderr, "warning: GITHUB_WEBHOOK_SECRET not set: webhook signature verification DISABLED (insecure)")
	}
	if err = coordinator.CheckSecret(secret, f.insecure); err != nil { // before anything is created on disk
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	unsandboxed := f.unsandboxed || strings.EqualFold(os.Getenv("NSELF_CI_ALLOW_UNSANDBOXED"), "true")
	f.unsandboxed = unsandboxed
	if err = os.MkdirAll(f.workdir, 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create workdir %s: %v\n", f.workdir, err)
		return 1
	}
	st, err := cirun.OpenStore(filepath.Join(home, "state.db"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot open the coordinator store: %v\n", err)
		return 1
	}
	defer func() { _ = st.Close() }()
	end, err := serveModeStart(ctx, fs, st, os.Stderr) // mode check (E664) and coordinator facts, before anything is bound
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer end()
	cfg := coordinator.Config{
		Store: st, Addr: f.addr, Secret: secret, Insecure: f.insecure, Force: f.force,
		AllowedRepos: append(append([]string(nil), f.allowed.vals...), splitAllowedReposEnv(os.Getenv("NSELF_CI_ALLOWED_REPOS"))...),
		PolicyDigest: "default", Concurrency: f.concurrency, Listeners: f.listenerAddrs(),
		Dispatch: serveDispatch(home, f),
		Logf:     func(format string, a ...any) { fmt.Fprintf(os.Stderr, "nself-ci-serve: "+format+"\n", a...) },
	}
	undoWire, err := serveWire(ctx, st, home, "", &cfg) // listeners' store, pairing service and the SIGTERM drain
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer undoWire()
	if hook != nil {
		hook(&cfg)
	}
	c, err := coordinator.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "nself-ci-serve: listening on %s  concurrency=%d  timeout=%ds\n", cfg.Addr, f.concurrency, f.timeout)
	if err = c.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// pipelineFacts are the source facts the webhook and manual routes store with a trigger.
type pipelineFacts struct {
	Trigger   string          `json:"trigger"`
	CloneURL  string          `json:"clone_url"`
	Synthetic json.RawMessage `json:"synthetic"`
	// Submitter is the audit label and role the user API stored ("user:bob role=member"); "" for a webhook.
	Submitter string `json:"submitter"`
}

// dispatchPipeline runs one queued trigger: clone the revision into a fresh directory, plan it and run the
// orchestrator. A failed gate is a result (exit 1), not an error; an error means the run could not happen.
func dispatchPipeline(home string, f *v15ServeFlags) coordinator.Dispatcher {
	return func(ctx context.Context, row store.IntakeRow) (coordinator.RunResult, error) {
		var facts pipelineFacts
		_ = json.Unmarshal(row.Facts, &facts)
		if facts.Synthetic != nil { // runner-demand pipelines are P7-HOST work: fail closed rather than run the repo default
			return coordinator.RunResult{}, errors.New("synthetic pipelines are not dispatched by this coordinator")
		}
		// a stored row is checked again here: nothing that reaches git may start with "-" or leave the project's host
		if !coordinator.ValidProject(row.Project) || !coordinator.ValidRevision(row.Revision) || (row.Pipeline != "" && !coordinator.ValidPipeline(row.Pipeline)) {
			return coordinator.RunResult{}, errors.New("refused: project, revision or pipeline is not valid")
		}
		cloneURL, err := coordinator.SafeCloneURL(row.Source, row.Project, facts.CloneURL)
		if err != nil {
			return coordinator.RunResult{}, err
		}
		dir, err := serve.CloneRef(cloneURL, row.Revision, f.workdir)
		defer func() {
			if dir != "" {
				_ = os.RemoveAll(dir)
			}
		}()
		if err != nil {
			return coordinator.RunResult{}, fmt.Errorf("checkout failed: %w", err)
		}
		o := RunOptions{Root: dir, Home: home, Pipeline: row.Pipeline, Trigger: serveTrigger(facts.Trigger),
			Container: !f.unsandboxed, Timeout: time.Duration(f.timeout) * time.Second, Stream: f.verbose}
		p, digest, subst, err := preparePlan(ctx, o)
		if err != nil {
			return coordinator.RunResult{}, fmt.Errorf("plan failed: %w", err)
		}
		if err = submitterGate(ctx, roles.Current(), facts, row.Project, p); err != nil {
			return coordinator.RunResult{}, err
		}
		out := io.Discard
		if f.verbose {
			out = os.Stderr
		}
		res, err := (&cirun.Coordinator{Opts: o, Plan: p, ConfigDigest: digest, Substantive: subst, Out: out}).Run(ctx)
		if err != nil {
			return coordinator.RunResult{}, err
		}
		if ctx.Err() != nil && string(res.Summary.Result) == "cancelled" { // cut short by shutdown: hand the row back, never record it as done
			return coordinator.RunResult{}, ctx.Err()
		}
		return coordinator.RunResult{PipelineID: res.RunID, Exit: res.Exit}, nil
	}
}

// serveTrigger maps a stored event name to a --trigger value ("" plans every non-drill job).
func serveTrigger(event string) string {
	switch event {
	case "push", "pull_request", "api", "schedule", "manual", "tag":
		return event
	}
	return ""
}
