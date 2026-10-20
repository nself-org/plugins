package main

// Purpose: `nself ci --coordinator <url>` (P7-NODE-19, Epic D14): registers the flag on the bare flag set,
// the coordinator client's RemoteResolver for `ci runs`, `ci runs cancel`, `ci logs` and `ci evidence`, and
// the options hook that sends a bare run to a TEAM coordinator (or to a live local `ci serve`) instead of
// running it in this process.
// Inputs:  --coordinator, NSELF_CI_COORDINATOR, the user config (ci.coordinator*), the repository.
// Outputs: the submitted run's contract:ci.events on stdout (--json) or its job results; exit 0 when the
//          run passed, 1 when it failed or the coordinator refused, 2 when it could not be reached.
// Constraints: V15 only (the flag lives on the V15 bare flag set, so the frozen v1.4 argv is untouched);
//          the revision under test never picks the coordinator (a committed ci.coordinator is ignored with
//          one warning); only the committed HEAD of a github.com repository is submitted, so a dirty tree
//          is reported and left out; flags the coordinator decides from its own policy are refused, not
//          dropped; with --detach the child process re-executes this and submits.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/coordclient"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// coordinatorFlag is the --coordinator value of the current invocation.
var coordinatorFlag string

// coordResolver is the registered RemoteResolver; repoRoot is what the warning scan looks at.
var coordResolver = &coordclient.Resolver{Inputs: func() coordclient.Inputs {
	return coordclient.Inputs{Flag: coordinatorFlag, RepoRoot: coordRepoRoot()}
}}

// Seams for tests.
var (
	coordExit   = os.Exit
	coordOut    io.Writer
	coordErr    io.Writer
	coordGit    = func(root string, args ...string) (string, bool) { return planGit(context.Background(), root, args...) }
	coordRootFn = func() string { return firstOf(bare.repo, os.Getenv("NSELF_CI_REPO")) }
)

func init() {
	bareFlags.StringVar(&coordinatorFlag, "coordinator", "", "send the run to this TEAM coordinator (https://host[:port]; env NSELF_CI_COORDINATOR, user config ci.coordinator); without it a live local `ci serve` takes the run")
	cirun.RegisterRemote(coordResolver)
	RegisterOptionsHook(coordinatorOptions)
}

func coordRepoRoot() string {
	if r := coordRootFn(); r != "" {
		return r
	}
	wd, _ := os.Getwd()
	return wd
}

// coordinatorOptions sends the run away when a coordinator or a live local serve is the target.
func coordinatorOptions(o *RunOptions) error {
	defer func() { coordinatorFlag = "" }() // a process runs one invocation; tests parse several
	if detachFlag || bare.help {
		return nil // the detached child re-executes with --coordinator and submits; --help prints help and sends nothing
	}
	cl, t, active, err := coordResolver.Client()
	if err == nil && !active {
		return nil
	}
	out, errw := coordOut, coordErr
	if out == nil {
		out = os.Stdout
	}
	if errw == nil {
		errw = os.Stderr
	}
	if err == nil {
		err = refuseUnsupported(o)
	}
	var in coordclient.RunInput
	if err == nil {
		in, err = submitInput(o, errw)
	}
	if err != nil {
		coordExit(coordFail(out, errw, o.JSON, err))
		return nil
	}
	in.JSON, in.Out, in.Err = o.JSON, out, errw
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	fmt.Fprintf(errw, "nself ci: running on %s (%s)\n", firstOf(t.URL, t.Socket), t.Kind)
	res, err := cl.Run(ctx, in)
	if err != nil {
		coordExit(coordFail(out, errw, o.JSON, err))
		return nil
	}
	coordExit(res.Exit)
	return nil // coordExit does not return in production
}

// refuseUnsupported rejects what a coordinator run cannot honor: it decides these from its own policy.
func refuseUnsupported(o *RunOptions) error {
	var bad []string
	add := func(on bool, name string) {
		if on {
			bad = append(bad, name)
		}
	}
	add(len(o.Adhoc) > 0, "-- <cmd>")
	add(o.Job != "", "--job")
	add(len(o.Services) > 0, "--service")
	add(o.Network != "", "--network")
	add(o.Container, "--container")
	add(o.Timeout != 0, "--timeout")
	add(o.Trust != "", "--trust")
	add(o.Trigger != "", "--trigger")
	add(o.Force, "--force")
	add(o.NoStatus, "--no-status")
	add(o.ConfirmFlaky > 0, "--confirm-flaky")
	// the scheduling and image flags are applied by hooks that run after this one, so read their values
	add(schedFlags.Prefer != "", "--prefer")
	add(schedFlags.Runner != "", "--runner")
	add(schedFlags.MaxQueue != "", "--max-queue")
	add(adhocFlags.Image != "", "--image")
	add(o.SHA != "", "--sha")
	add(bare.cfg != "", "--config")
	add(bare.base != "", "--base")
	if len(bad) == 0 {
		return nil
	}
	return clientapi.Failf(clientapi.ReasonBadRequest, "%s cannot be used with a coordinator: it decides these from its own policy", strings.Join(bad, ", "))
}

var (
	hexRev   = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	githubRe = regexp.MustCompile(`^(?:https://github\.com/|ssh://git@github\.com/|git@github\.com:)([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`)
)

// submitInput reads the project and the committed HEAD of the repository the run is for.
func submitInput(o *RunOptions, errw io.Writer) (coordclient.RunInput, error) {
	rev, ok := coordGit(o.Root, "rev-parse", "HEAD")
	rev = strings.ToLower(strings.TrimSpace(rev))
	if !ok || !hexRev.MatchString(rev) {
		return coordclient.RunInput{}, clientapi.Failf(clientapi.ReasonBadRequest, "%s has no commit to submit", o.Root)
	}
	remote, _ := coordGit(o.Root, "config", "--get", "remote.origin.url")
	m := githubRe.FindStringSubmatch(strings.TrimSpace(remote))
	if m == nil {
		return coordclient.RunInput{}, clientapi.Failf(clientapi.ReasonBadRequest, "remote origin of %s is not a github.com repository, so the coordinator cannot fetch it", o.Root)
	}
	if dirty, _ := coordGit(o.Root, "status", "--porcelain", "--untracked-files=no"); strings.TrimSpace(dirty) != "" {
		fmt.Fprintln(errw, "warning: uncommitted changes are not part of the run; the coordinator runs the committed HEAD "+rev[:7])
	}
	return coordclient.RunInput{Project: m[1] + "/" + m[2], Revision: rev, Pipeline: o.Pipeline}, nil
}

// coordFail prints a coordinator-client failure in the caller's mode and returns the exit status:
// 1 for a refusal or a bad request, 2 when the coordinator could not be reached.
func coordFail(out, errw io.Writer, asJSON bool, err error) int {
	exit, class := output.ExitUser, "usage"
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(errw, "interrupted; the run keeps going on the coordinator")
		return 130
	}
	if r := clientapi.ReasonOf(err); r == clientapi.ReasonTransport || r == clientapi.ReasonServeDown || r == "" {
		exit, class = output.ExitInfra, "infra"
	}
	if asJSON {
		d := output.ErrorDetail{Code: clientapi.CodeUserAPI, Message: strings.Join(strings.Fields(err.Error()), " "), ExitCode: exit, Class: class}
		if output.WriteError(out, "ci", d, nil) == nil {
			return exit
		}
	}
	fmt.Fprintln(errw, err)
	return exit
}
