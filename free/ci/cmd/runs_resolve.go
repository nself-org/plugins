package main

// Purpose: `nself ci runs resolve <attempt> --idempotency-key <key>
// --rerun|--abandon [--yes] [--reason text] [--json]` (P7-CACHE-05, Epic D9):
// the only way out of needs-operator, and of a stuck finalizing release
// attempt. The key is the NSELF_CI_IDEMPOTENCY_KEY the job received.
// Inputs:  argv after `runs resolve`; env NSELF_CI_HOME.
// Outputs: a line or one v1 JSON envelope; exit 0 resolved, 1 refused (E716)
// or usage, 2 infrastructure.
// Constraints: V15 only; asks for confirmation unless --yes (and refuses to
// run unconfirmed without a terminal); one audit row per action; the key never
// reaches the log or the audit trail.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/release/job"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

const resolveUsage = "usage: nself ci runs resolve <attempt> --idempotency-key <key> --rerun|--abandon [--yes] [--reason text] [--json]"

func init() { registerRunsSub("resolve", runResolve) }

type resolveArgs struct {
	attempt, key, reason string
	action               job.Action
	yes, asJSON          bool
}

func parseResolve(args []string) (resolveArgs, error) {
	var a resolveArgs
	value := func(i *int, name string) (string, error) {
		if *i+1 >= len(args) {
			return "", fmt.Errorf("%s needs a value", name)
		}
		*i++
		return args[*i], nil
	}
	for i := 0; i < len(args); i++ {
		var err error
		switch arg := args[i]; {
		case arg == "--idempotency-key":
			a.key, err = value(&i, arg)
		case strings.HasPrefix(arg, "--idempotency-key="):
			a.key = strings.TrimPrefix(arg, "--idempotency-key=")
		case arg == "--reason":
			a.reason, err = value(&i, arg)
		case strings.HasPrefix(arg, "--reason="):
			a.reason = strings.TrimPrefix(arg, "--reason=")
		case arg == "--rerun" || arg == "--abandon":
			if a.action != "" {
				return a, errors.New("choose one of --rerun and --abandon")
			}
			a.action = job.Action(strings.TrimPrefix(arg, "--"))
		case arg == "--yes" || arg == "-y":
			a.yes = true
		case arg == "--json":
			a.asJSON = true
		case strings.HasPrefix(arg, "-") || a.attempt != "":
			return a, fmt.Errorf("unexpected argument %q", arg)
		default:
			a.attempt = arg
		}
		if err != nil {
			return a, err
		}
	}
	switch {
	case a.attempt == "":
		return a, errors.New("an attempt id is required")
	case a.key == "":
		return a, errors.New("--idempotency-key is required")
	case a.action == "":
		return a, errors.New("choose --rerun or --abandon")
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
	ctx := context.Background()
	st, err := cirun.OpenStore(runsStorePath())
	if err != nil {
		return exitForError(err)
	}
	defer func() { _ = st.Close() }()
	_ = cirun.Recover(ctx, st, "", os.Getenv("NSELF_CI_HOME")) // a dead coordinator's attempt becomes needs-operator first
	if !confirm(a, os.Stdin, stdinIsTerminal()) {
		fmt.Fprintln(os.Stderr, "nothing changed")
		return 1
	}
	actor := "operator"
	if u := os.Getenv("USER"); u != "" {
		actor += ":" + u
	}
	res, err := job.Resolve(ctx, st, job.Request{Attempt: a.attempt, Key: a.key, Action: a.action, Actor: actor, Reason: a.reason})
	var je *job.Error
	if errors.As(err, &je) {
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
