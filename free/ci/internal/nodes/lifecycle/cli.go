package lifecycle

// Purpose: the command surface of `nself ci nodes add|list|show|test|drain|enable|revoke|remove|rotate`
// (P7-NODE-05, Epic D17): flags, one handler per subcommand, the v1 JSON envelope and the exit classes.
// Inputs:  argv after the subcommand key, an Env that opens the store.
// Outputs: a human table or one v1 envelope on stdout (errors: stderr, or an error envelope with --json);
//          exit 0, 1 (usage, unknown node), 2 (infrastructure, transport) or 3 (host key, revoked).
// Constraints: side-effect classes (the table entries in cmd/nodes_lifecycle.go): list, show read;
//          add, test, drain, enable, rotate remote; revoke, remove destructive. The admin's trust
//          fields come from these flags or the prompt only; no output holds key material, a token
//          or a remote command's stderr beyond one cut line.

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// Env opens the store for one command; override is the --project flag.
type Env struct {
	Open     func(ctx context.Context, project string) (*Deps, func(), error)
	Out, Err io.Writer
}

type flags struct {
	json, acceptsSet       bool
	project, name, hostKey string
	agentBinary, installer string
	mode                   string
	accepts, secrets, zone string
	ownership              string
}

// newFlags builds the flag set of one subcommand, including the flags later Tickets registered.
func newFlags(sub string, f *flags, errOut io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("nodes "+sub, flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	switch sub {
	case "add":
		fs.StringVar(&f.project, "project", "", "project id to authorize (default: the current repository)")
		fs.StringVar(&f.name, "name", "", "node name (default: the host); lower-cased it is the node id")
		fs.StringVar(&f.hostKey, "host-key", "", "pin the host key with exactly this SHA256:<fingerprint> (required without a terminal)")
		fs.StringVar(&f.agentBinary, "agent-binary", "", "local nself-ci-agent binary to install (ELF or Mach-O for the node)")
		fs.StringVar(&f.installer, "installer", "", "agent installer name (default local-file)")
		fs.StringVar(&f.accepts, "accepts", "", "trust classes the node accepts, comma list (default owner,internal)")
		fs.StringVar(&f.secrets, "secret-classes", "", "secret classes the node may hold, comma list (default none)")
		fs.StringVar(&f.zone, "privacy-zone", "", "privacy zone of the node")
		fs.StringVar(&f.ownership, "ownership", "operator", "ownership: operator, team or provider")
	case "rotate":
		fs.StringVar(&f.hostKey, "host-key", "", "the new host key's SHA256:<fingerprint>")
		fs.StringVar(&f.mode, "mode", HostKeyMode, "rotate mode")
	}
	for _, fn := range ext.flagFuncs(sub) {
		fn(fs)
	}
	return fs
}

// parse allows flags after the positional arguments.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// exitFor maps a code to the exit class: host key and revocation are auth (3), transport and store
// failures are infra (2), the rest is the caller's (1).
func exitFor(code string) int {
	switch code {
	case CodeHostKey, CodeRevoked:
		return output.ExitAuth
	case CodeNoRoute, "E604", "E605", "E606", "E607", "":
		return output.ExitInfra
	}
	return output.ExitUser
}

// result is what a handler returns: the machine data and the human renderer.
type result struct {
	data  any
	human func(w io.Writer)
}

// Run executes one subcommand and returns the process exit code.
func Run(sub string, args []string, env Env) int {
	var f flags
	fs := newFlags(sub, &f, env.Err)
	pos, err := parse(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		return 0 // the flag set already printed the usage
	}
	if err != nil {
		// A bad flag still honours --json: one error envelope, never an empty stdout.
		f.json = f.json || slices.ContainsFunc(args, func(a string) bool {
			return a == "--json" || a == "-json" || a == "--json=true" || a == "-json=true"
		})
		return fail(env, f, "ci nodes "+sub, usage("%s", oneLine(err)))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	wantPos := 1
	if sub == "list" {
		wantPos = 0
	}
	if len(pos) != wantPos {
		return fail(env, f, "ci nodes "+sub, usage("usage: nself ci nodes %s", usageLine(sub)))
	}
	if wantPos == 1 && sub != "add" {
		// A node id is a lower-case name; anything else (a path, a dot-dot) never reaches a lock file.
		pos[0] = strings.ToLower(pos[0])
		if verr := remote.ValidateNodeID(pos[0]); verr != nil {
			return fail(env, f, "ci nodes "+sub, usage("the node id is not valid (letters, digits, dot, dash, underscore)"))
		}
	}
	d, closeFn, err := env.Open(ctx, f.project)
	if err != nil {
		return fail(env, f, "ci nodes "+sub, newErr("E607", "store", "the CI store cannot be opened: %v", oneLine(err)))
	}
	defer closeFn()
	d.Stdout, d.Stderr = env.Out, env.Err
	var arg string
	if wantPos == 1 {
		arg = pos[0]
	}
	res, err := dispatch(ctx, d, sub, arg, f, fs)
	if err != nil {
		if res.human != nil && !f.json {
			res.human(env.Out)
		}
		return failWith(env, f, "ci nodes "+sub, err, partialReport(res, err))
	}
	if f.json {
		if werr := output.WriteData(env.Out, "ci nodes "+sub, res.data, nil); werr != nil {
			return output.ExitInfra
		}
		return 0
	}
	res.human(env.Out)
	return 0
}

func usageLine(sub string) string {
	switch sub {
	case "list":
		return "list [--json]"
	case "add":
		return "add <[user@]host> [--name N] [--host-key SHA256:<fp>] [--agent-binary PATH] [--accepts ...] [--secret-classes ...] [--privacy-zone Z] [--ownership O] [--json]"
	case "rotate":
		return "rotate <node> --host-key SHA256:<fp> [--mode M] [--json]"
	}
	return sub + " <node> [--json]"
}

// fail prints the error and returns its exit code.
func fail(env Env, f flags, command string, err error) int { return failWith(env, f, command, err, "") }

// partialReport is the revoke report as one JSON string for the error envelope's cause, only for a
// partial revoke (committed, cleanup incomplete): machine callers must see what is left.
func partialReport(res result, err error) string {
	var le *Error
	if res.data == nil || !errors.As(err, &le) || le.Reason != ReasonPartial {
		return ""
	}
	b, _ := json.Marshal(res.data)
	return string(b)
}

func failWith(env Env, f flags, command string, err error, cause string) int {
	code := CodeOf(err)
	exit := exitFor(code)
	if f.json {
		if code == "" {
			code = "E607"
		}
		_ = output.WriteError(env.Out, command, output.ErrorDetail{Code: code, Message: err.Error(), Cause: cause, ExitCode: exit}, nil)
		return exit
	}
	_, _ = io.WriteString(env.Err, err.Error()+"\n")
	return exit
}

// FlagSet returns the flag set of a subcommand (the table entry's Flags constructor).
func FlagSet(sub string) *flag.FlagSet {
	var f flags
	return newFlags(sub, &f, io.Discard)
}
