package main

// Purpose: `nself-ci-agent join <enroll-url>` (P7-NODE-07, Epic D5): enroll this node with a coordinator
//          that runs `nself ci nodes pair`.
// Inputs:  the enroll URL (argv), --expect-fingerprint SHA256:<fp>, --name; the pairing code from a TTY
//          prompt or the first line of stdin (never argv, never the environment; there is no code flag).
// Outputs: the root fingerprint to compare, this node's CSR key fingerprint and node-key id (print before
//          the request, for the admin), then the enrollment saved under <state>/pki (root, chain, sealed
//          TLS key, agent URL). Exit 0 enrolled, 1 refused or failed, 2 usage.
// Constraints: the pinned root is confirmed by an interactive "yes" on a terminal or by an exact
//          --expect-fingerprint; neither is a refusal (E660 fingerprint_unconfirmed), before any code or
//          CSR leaves the node; a clock more than 120 s off is E660 clock_skew; the PreStart hooks run
//          first (the production guard of P7-NODE-14 registers one), then the PostStart hooks (--advertise);
//          registered join hooks run last. Flags other files add (--advertise) arrive through RegisterFlags.

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
)

func init() {
	must(RegisterSubcommand("join", "enroll this node with a coordinator (pairing code from a terminal or stdin)", runJoin))
}

// isTTY reports whether f is a character device. /dev/null passes too, which is safe: the prompt then
// reads EOF and the join is refused as unconfirmed.
func isTTY(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func runJoin(ctx context.Context, args []string, sio IO) int {
	fs := flag.NewFlagSet("join", flag.ContinueOnError)
	fs.SetOutput(sio.Stderr)
	expect := fs.String("expect-fingerprint", "", "the root fingerprint SHA256:<fp> shown by nself ci nodes pair (required without a terminal)")
	name := fs.String("name", "", "node name (default: the host name)")
	applyFlags("join", fs)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent join [--expect-fingerprint SHA256:<fp>] [--name N] [--advertise] <enroll-url>")
		return 2
	}
	if err := RunPreStart(ctx, "join"); err != nil {
		fmt.Fprintf(sio.Stderr, "nself-ci-agent join: %v\n", err)
		return 1
	}
	if err := RunPostStart(ctx, "join"); err != nil {
		fmt.Fprintf(sio.Stderr, "nself-ci-agent join: %v\n", err)
		return 1
	}
	state, err := stateDir()
	if err != nil {
		fmt.Fprintf(sio.Stderr, "nself-ci-agent join: %v\n", err)
		return 1
	}
	key, err := nodekey.LoadOrCreate(state)
	if err != nil {
		fmt.Fprintf(sio.Stderr, "nself-ci-agent join: node key: %v\n", err)
		return 1
	}
	host, _ := os.Hostname()
	if *name == "" {
		*name = host
	}
	*name = strings.ToLower(*name)
	tty := false
	if f, ok := sio.Stdin.(*os.File); ok {
		tty = isTTY(f)
	}
	in := bufio.NewReader(sio.Stdin)
	line := func(q string) (string, error) {
		_, _ = fmt.Fprint(sio.Stderr, q)
		s, err := in.ReadString('\n')
		if err != nil && (err != io.EOF || s == "") {
			return "", err
		}
		return strings.TrimSpace(s), nil
	}
	opts := enroll.JoinOptions{
		URL: fs.Arg(0), ExpectFingerprint: *expect, Name: *name, OS: runtime.GOOS, Arch: runtime.GOARCH, AgentVersion: Version,
		NodeKey: key.Public(), NodeKeyID: key.KeyID(), NodeSign: key.Sign, Out: sio.Stderr,
		Code: func() (string, error) { return line("Pairing code: ") },
	}
	if tty {
		opts.Confirm = func(fp string) (bool, error) {
			fmt.Fprintf(sio.Stderr, "The coordinator's root fingerprint is\n  %s\nCompare it with the one `nself ci nodes pair` printed.\n", fp)
			a, err := line("Type yes to trust it: ")
			return strings.EqualFold(a, "yes"), err
		}
	}
	res, err := enroll.Join(ctx, opts)
	if err != nil {
		fmt.Fprintf(sio.Stderr, "nself-ci-agent join: %v\n", err)
		return 1
	}
	if err = enroll.SaveCredential(state, res, time.Now()); err != nil {
		fmt.Fprintf(sio.Stderr, "nself-ci-agent join: the certificate was issued but could not be saved: %v\n", err)
		return 1
	}
	fmt.Fprintf(sio.Stdout, "Joined as %s. Certificate valid until %s (renews at two thirds of its life).\n", res.NodeID, res.NotAfter.Local().Format(time.DateTime))
	if err = pairing.RunJoinHooks(ctx, res, state); err != nil {
		fmt.Fprintf(sio.Stderr, "nself-ci-agent join: enrolled, but %v\n", err)
		return 1
	}
	return 0
}
