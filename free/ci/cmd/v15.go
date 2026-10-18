package main

// Purpose: V15 dispatch and the bare `nself ci` run (P7-CI-37): parse the
// bare flags, detect → plan, hand the plan to the coordinator and map the
// result to an exit code. Bare `nself ci -- <cmd…>` goes to the ad hoc handler.
// Inputs:  argv after `nself ci`; env NSELF_CI_REPO, NSELF_CI_HOME.
// Outputs: the human (or registered) renderer on stdout; exit 0 (pass),
// 1 (gate failed or usage error), 2 (infrastructure or unplaceable job).
// Constraints: the flag set is a package-level var so later Tickets add flags
// from their own files; option hooks run in registration order after parsing;
// this file is dispatch only, the engine lives in internal/run.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// RunOptions are the options of a bare run; option hooks change them.
type RunOptions = cirun.Options

var optionsHooks []func(*RunOptions) error

// errFlagParse marks a flag error the flag package already printed.
var errFlagParse = errors.New("flag parse error")

// RegisterOptionsHook adds a hook applied after flag parsing and before the
// run starts. An error aborts the run with its code.
func RegisterOptionsHook(f func(*RunOptions) error) { optionsHooks = append(optionsHooks, f) }

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// bareFlags is the flag set of bare `nself ci`; other files add flags to it.
var bareFlags = flag.NewFlagSet("ci", flag.ContinueOnError)

var bare struct {
	opts                  RunOptions
	services              multiFlag
	repo, cfg, base       string
	jsonOut, stream, help bool
}

func init() {
	f, o := bareFlags, &bare.opts
	f.StringVar(&o.Pipeline, "pipeline", "", "named pipeline to run (default: every non-drill job)")
	f.StringVar(&o.Job, "job", "", "run only this job and its dependencies")
	f.StringVar(&o.Trigger, "trigger", "", "plan the pipelines listing this trigger (push, pull_request, tag, manual, schedule, api, local)")
	f.BoolVar(&o.JSON, "json", false, "print machine output")
	f.BoolVar(&o.Stream, "stream", false, "stream job output as it happens")
	f.BoolVar(&o.Container, "container", false, "run jobs in the pinned toolchain container")
	f.Var(&bare.services, "service", "disposable service a job needs (repeatable)")
	f.StringVar(&o.Network, "network", "", "network scope for jobs: none, internet, restricted, lan")
	f.BoolVar(&o.Force, "force", false, "ignore the host capacity reservations")
	f.BoolVar(&o.NoStatus, "no-status", false, "do not post the nself-ci commit status")
	f.DurationVar(&o.Timeout, "timeout", 0, "per-job timeout (0 = the job's own)")
	f.StringVar(&o.Trust, "trust", "", "trust class of the source: owner, collaborator, internal, untrusted")
	f.StringVar(&o.SHA, "sha", "", "not supported in V15; use: nself ci evidence verify REV")
	f.StringVar(&bare.repo, "repo", "", "repository root (env NSELF_CI_REPO; default: current directory)")
	f.StringVar(&bare.cfg, "config", "", "config file (default: <repo>/.nself/ci.yaml)")
	f.StringVar(&bare.base, "base", "", "base commit for affected selection (env NSELF_CI_BASE_SHA)")
	f.BoolVar(&bare.help, "help", false, "show this help")
	f.BoolVar(&bare.help, "h", false, "show this help")
}

// runV15 selects the longest v1.5-only command, the bare run, or a legacy key.
func runV15(args []string) int {
	if problems := model.CodeProblems(); len(problems) != 0 {
		fmt.Fprintln(os.Stderr, "invalid error-code registry:", strings.Join(problems, "; "))
		return 2
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runBare(args)
	}
	for n := len(args); n >= 1; n-- {
		if hasFlagToken(args[:n]) {
			continue
		}
		if entry, ok := v15Subcommands[strings.Join(args[:n], " ")]; ok {
			return entry.Handler(args[n:])
		}
	}
	return run(args)
}

// exitForError maps an error to the process exit code and prints it.
func exitForError(err error) int {
	fmt.Fprintln(os.Stderr, err)
	switch cirun.CodeOf(err) {
	case cirun.CodeSHA, cirun.CodeTrust, cirun.CodeRun, cirun.CodeOptions:
		return 1
	}
	var ce *config.Error
	var pe *plan.Error
	if errors.As(err, &ce) || (errors.As(err, &pe) && pe.Code != plan.CodeUnplaceable) {
		return 1
	}
	return 2
}

// v15Help lists the bare flags and the visible V15 subcommands.
func v15Help() string {
	var b strings.Builder
	b.WriteString("Usage: nself ci [flags] [-- <cmd…>]\n       nself ci <command> [args]\n\nFlags:\n")
	bareFlags.SetOutput(&b)
	bareFlags.PrintDefaults()
	var names []string
	for k, e := range v15Subcommands {
		if !e.Hidden {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	b.WriteString("\nCommands:\n")
	for _, k := range names {
		b.WriteString("  " + k + "\n")
	}
	return b.String()
}

// parseBare splits argv at `--`, parses flags and applies the option hooks.
func parseBare(args []string) (RunOptions, error) {
	var adhoc []string
	for i, a := range args {
		if a == "--" {
			args, adhoc = args[:i], args[i+1:]
			break
		}
	}
	bare.opts, bare.services, bare.help = RunOptions{}, nil, false
	bare.repo, bare.cfg, bare.base = "", "", ""
	clearSchedFlags()
	bareFlags.SetOutput(os.Stderr)
	if err := bareFlags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return RunOptions{}, err
		}
		return RunOptions{}, errFlagParse
	}
	o := bare.opts
	o.Services, o.Adhoc = append([]string(nil), bare.services...), adhoc
	if rest := bareFlags.Args(); len(rest) > 0 {
		bare.repo = rest[0]
	}
	root := firstOf(bare.repo, os.Getenv("NSELF_CI_REPO"))
	var err error
	if root == "" {
		root, err = os.Getwd()
	} else {
		root, err = filepath.Abs(root)
	}
	if err != nil {
		return o, err
	}
	o.Root, o.Home = root, os.Getenv("NSELF_CI_HOME")
	if o.PolicyValues, err = schedRevisionValues(o.Trigger); err != nil {
		return o, err
	}
	for _, h := range optionsHooks {
		if err = h(&o); err != nil {
			return o, err
		}
	}
	return o, nil
}

// preparePlan runs E621, config load, detection and the planner.
func preparePlan(ctx context.Context, o RunOptions) (*plan.Plan, string, func(plan.PlannedJob) bool, error) {
	if err := config.CheckLeftover(o.Root); err != nil {
		return nil, "", nil, err
	}
	cfg, digest := model.PipelineConfig{Version: 1}, "builtin-defaults"
	path := firstOf(bare.cfg, filepath.Join(o.Root, config.FileName))
	loaded, err := config.Load(path)
	switch {
	case err == nil:
		cfg = loaded.Config
		if raw, e := os.ReadFile(path); e == nil {
			sum := sha256.Sum256(raw)
			digest = hex.EncodeToString(sum[:])
		}
	case !errors.Is(err, os.ErrNotExist) || bare.cfg != "":
		return nil, "", nil, err
	}
	facts, checks, err := planChecks(o.Root, cfg)
	if err != nil {
		return nil, "", nil, err
	}
	p, err := plan.Build(plan.Input{Config: cfg, Facts: facts, Checks: checks, Revision: planRevision(ctx, o.Root, bare.base),
		World: planWorld(cfg, checks), Pipeline: o.Pipeline, Trigger: model.Trigger(o.Trigger)})
	if err != nil {
		return nil, "", nil, err
	}
	real := map[string]bool{}
	for _, c := range checks {
		real[c.ID] = c.Substantive
	}
	subst := func(pj plan.PlannedJob) bool { return pj.Check == "" || real[pj.Check] }
	return p, digest, subst, nil
}

// runBare is bare `nself ci`.
func runBare(args []string) int {
	o, err := parseBare(args)
	switch {
	case errors.Is(err, flag.ErrHelp) || (err == nil && bare.help):
		fmt.Print(v15Help())
		return 0
	case errors.Is(err, errFlagParse):
		return 1 // the flag package already printed it
	case err != nil:
		return exitForError(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer stop()
	hooks := cirun.Hooks{}.Resolved()
	if err = o.Validate(hooks); err != nil {
		return exitForError(err)
	}
	if len(o.Adhoc) > 0 {
		code, err := hooks.Adhoc.Adhoc(ctx, o, o.Adhoc)
		if err != nil {
			return exitForError(err)
		}
		return code
	}
	p, digest, subst, err := preparePlan(ctx, o)
	if err != nil {
		return exitForError(err)
	}
	c := &cirun.Coordinator{Opts: o, Plan: p, ConfigDigest: digest, ConfigOutside: configOutside(o.Root), Substantive: subst, Out: os.Stdout}
	res, err := c.Run(ctx)
	if err != nil {
		return exitForError(err)
	}
	return res.Exit
}

// configOutside is true when --config names a file other than the repo's own
// .nself/ci.yaml: that run cannot be bound to the commit.
func configOutside(root string) bool {
	if bare.cfg == "" {
		return false
	}
	abs, err := filepath.Abs(bare.cfg)
	return err != nil || filepath.Clean(abs) != filepath.Join(root, config.FileName)
}
