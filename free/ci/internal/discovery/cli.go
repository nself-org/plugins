package discovery

// Purpose: `nself ci nodes discover [--timeout 5s] [--json]` (P7-NODE-12, Epic D12): browse for
//          `_nself-ci._tcp` for this invocation only and list the candidates, state discovered.
// Inputs:  argv after the key; the Guard.
// Outputs: a table, or one v1 JSON envelope, on stdout. Exit 0 (including no candidates), 1 (usage or
//          E667 discovery disabled), 2 (the network layer failed).
// Constraints: side-effect class read: it writes nothing to the registry or the store. A candidate is
//          never added, paired or authorized from here; the footer says how to pair.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

// Env is the command's streams and its Guard.
type Env struct {
	Out, Err io.Writer
	Guard    *Guard
}

type discoverFlags struct {
	json    bool
	timeout time.Duration
}

func newFlags(f *discoverFlags, errOut io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("nodes discover", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	fs.DurationVar(&f.timeout, "timeout", 5*time.Second, "how long to listen for answers (0.5s to 60s)")
	return fs
}

// FlagSet is the flag set of the command (the table entry's Flags constructor).
func FlagSet() *flag.FlagSet { return newFlags(&discoverFlags{}, io.Discard) }

const command = "ci nodes discover"

// DiscoverData is the --json payload.
type DiscoverData struct {
	Result
	Timeout string `json:"timeout"`
}

// RunDiscover runs the command and returns the process exit code.
func RunDiscover(args []string, env Env) int {
	var f discoverFlags
	fs := newFlags(&f, env.Err)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if fs.NArg() != 0 {
		return failCmd(env, f.json, "E607", output.ExitUser, errors.New("usage: nself ci nodes discover [--timeout 5s] [--json]"))
	}
	if f.timeout < MinTimeout || f.timeout > MaxTimeout {
		return failCmd(env, f.json, "E607", output.ExitUser, fmt.Errorf("--timeout must be between %s and %s", MinTimeout, MaxTimeout))
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout+10*time.Second)
	defer cancel()
	res, err := env.Guard.Browse(ctx, f.timeout)
	if err != nil {
		var de *Error
		if errors.As(err, &de) {
			return failCmd(env, f.json, CodeDisabled, output.ExitUser, err)
		}
		return failCmd(env, f.json, "E607", output.ExitInfra, err)
	}
	if f.json {
		if output.WriteData(env.Out, command, DiscoverData{Result: res, Timeout: f.timeout.String()}, nil) != nil {
			return output.ExitInfra
		}
		return 0
	}
	render(env.Out, res, f.timeout)
	return 0
}

func failCmd(env Env, asJSON bool, code string, exit int, err error) int {
	if asJSON {
		_ = output.WriteError(env.Out, command, output.ErrorDetail{Code: code, Message: oneLine(err.Error()), ExitCode: exit}, nil)
		return exit
	}
	_, _ = io.WriteString(env.Err, oneLine(err.Error())+"\n")
	return exit
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func render(w io.Writer, res Result, timeout time.Duration) {
	if len(res.Candidates) == 0 {
		_, _ = fmt.Fprintf(w, "no candidates answered within %s\n", timeout)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tADDRESS\tPORT\tINSTANCE\tSTATE")
	dup := false
	for _, c := range res.Candidates {
		state := c.State
		if c.DuplicateID {
			state += " (duplicate id)"
			dup = true
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", dash(c.WindowID), dash(strings.Join(c.Addresses, ",")), dash(portText(c.Port)), c.Instance, state)
	}
	_ = tw.Flush()
	if dup {
		_, _ = fmt.Fprintln(w, "More than one host answers with the same ID: only one can be the node you started. Do not pair until you know which.")
	}
	if res.Truncated > 0 || res.Flooded {
		_, _ = fmt.Fprintf(w, "The list is cut: %d more candidate(s) not shown%s. Records beyond the limit were dropped; look for a flood on the LAN.\n", res.Truncated, map[bool]string{true: ", and the raw record limit was reached", false: ""}[res.Flooded])
	}
	_, _ = fmt.Fprintln(w, "Candidates are hints from the LAN, not trusted and not authorized. Match the ID with the one `nself-ci-agent join --advertise` printed, then pair with `nself ci nodes pair`.")
}

func portText(p int) string {
	if p == 0 {
		return ""
	}
	return strconv.Itoa(p)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
