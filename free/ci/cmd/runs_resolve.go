package main

// Purpose: `nself ci runs resolve <attempt> --idempotency-key <key>
// --rerun|--abandon [--force] [--yes] [--reason text] [--json]` (P7-CACHE-05, Epic D9):
// the only way out of needs-operator, and of a stuck finalizing release
// attempt. The key is the NSELF_CI_IDEMPOTENCY_KEY the job received.
// Inputs:  argv after `runs resolve`; env NSELF_CI_HOME.
// Outputs: a line or one v1 JSON envelope; exit 0 resolved, 1 refused (E716)
// or usage, 2 infrastructure (a store that cannot be opened or read).
// Constraints: V15 only; asks for confirmation unless --yes (and refuses to
// run unconfirmed without a terminal); one audit row per action; the key never
// reaches the log or the audit trail. The key guards against resolving the wrong
// attempt (it derives from the public attempt id) and is not a secret, hence
// argv. Recover runs first, like every `runs` read, so an attempt of a dead
// coordinator is already needs-operator.

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/release/job"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

const resolveUsage = "usage: nself ci runs resolve <attempt> --idempotency-key <key> --rerun|--abandon [--force] [--yes] [--reason text] [--json]"

func init() { registerRunsSub("resolve", runResolve) }

type resolveArgs struct {
	attempt, key, reason string
	action               job.Action
	yes, asJSON, force   bool
	rerun, abandon       bool
}

// resolveFlagSet declares the flags of `runs resolve` into a (nil: a throwaway). The machine transport renders
// booleans as --abandon=true, consent as --yes=true and puts -- before the positional, so the standard flag package parses it.
func resolveFlagSet(a *resolveArgs) *flag.FlagSet {
	if a == nil {
		a = &resolveArgs{}
	}
	fs := flag.NewFlagSet("runs resolve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&a.key, "idempotency-key", "", "key that makes the resolution run at most once")
	fs.StringVar(&a.reason, "reason", "", "why the attempt is resolved this way")
	fs.BoolVar(&a.rerun, "rerun", false, "run the release attempt again")
	fs.BoolVar(&a.abandon, "abandon", false, "abandon the release attempt")
	fs.BoolVar(&a.yes, "yes", false, "consent without a prompt")
	fs.BoolVar(&a.yes, "y", false, "consent without a prompt")
	fs.BoolVar(&a.force, "force", false, "resolve even when the attempt looks live")
	fs.BoolVar(&a.asJSON, "json", false, "print one v1 JSON envelope")
	return fs
}

func parseResolve(args []string) (resolveArgs, error) {
	var a resolveArgs
	pos, err := parseMixed(resolveFlagSet(&a), args)
	switch {
	case err != nil:
		return a, fmt.Errorf("unexpected argument: %v", err)
	case a.rerun && a.abandon:
		return a, errors.New("choose one of --rerun and --abandon")
	case len(pos) > 1:
		return a, fmt.Errorf("unexpected argument %q", pos[1])
	case len(pos) == 0:
		return a, errors.New("an attempt id is required")
	case a.key == "":
		return a, errors.New("--idempotency-key is required")
	case !a.rerun && !a.abandon:
		return a, errors.New("choose --rerun or --abandon")
	}
	a.attempt = pos[0]
	a.action = job.Rerun
	if a.abandon {
		a.action = job.Abandon
	}
	return a, nil
}

// confirm asks the operator unless --yes was given; without a terminal it refuses.
func confirm(a resolveArgs, in io.Reader, interactive bool) bool {
	if a.yes {
		return true
	}
	if !interactive {
		fmt.Fprintln(os.Stderr, "confirmation needed: run on a terminal or pass --yes")
		return false
	}
	fmt.Fprintf(os.Stderr, "%s attempt %s? A release attempt runs at most once. Type yes to continue: ", a.action, a.attempt)
	line, _ := bufio.NewReader(in).ReadString('\n')
	return strings.TrimSpace(line) == "yes"
}

// stdinIsTerminal reports whether stdin is a character device (a terminal).
func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func runResolve(args []string) int {
	a, err := parseResolve(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, resolveUsage)
		return 1
	}
	if code, ok := refuseWhileServe("`runs resolve`"); ok { // P7-NODE-19: a serve owns the store
		return code
	}
	ctx := context.Background()
	st, err := cirun.OpenStore(runsStorePath())
	if err != nil {
		return exitForError(err)
	}
	defer func() { _ = st.Close() }()
	if !confirm(a, os.Stdin, stdinIsTerminal()) { // recovery kills processes and rewrites run state: never before consent
		fmt.Fprintln(os.Stderr, "nothing changed")
		return 1
	}
	recoverForHuman(ctx, st) // a dead coordinator's attempt becomes needs-operator first
	actor := "operator"
	if u := os.Getenv("USER"); u != "" {
		actor += ":" + u
	}
	res, err := job.Resolve(ctx, st, job.Request{Attempt: a.attempt, Key: a.key, Action: a.action, Actor: actor, Reason: a.reason, Force: a.force})
	var je *job.Error
	if errors.As(err, &je) {
		if a.asJSON {
			_ = output.WriteError(os.Stdout, "ci runs resolve", output.ErrorDetail{Code: je.Code, Message: je.Error(), ExitCode: 1}, nil)
			return 1
		}
		fmt.Fprintln(os.Stderr, je)
		return 1
	}
	if err != nil {
		return exitForError(err)
	}
	if a.asJSON {
		if err = output.WriteData(os.Stdout, "ci runs resolve", res, nil); err != nil {
			return exitForError(err)
		}
		return 0
	}
	if res.Next != "" {
		fmt.Printf("%s resolved (rerun): the rerun is approved and audited; the coordinator may now create attempt %s\n", res.Attempt, res.Next)
		return 0
	}
	fmt.Printf("%s resolved (%s): the attempt is failed\n", res.Attempt, res.Action)
	return 0
}
