package provision

// Purpose: the two commands, `nself ci nodes provision` and `nself ci nodes
//   verify`, ported from core `nself runner provision|verify`
//   (cmd/commands/runner_provision.go, runner_verify.go, runner.go): same
//   flags, defaults, checks, messages and exit codes. cmd/nodes_provision.go
//   only registers them.
// Inputs:  argv after the subcommand key; Deps (streams, env lookup, executor
//   factory) so tests run without a host.
// Outputs: provisioning steps or the parity matrix on stdout; "Error: ..." on
//   stderr; the process exit code (0, or 1 for any failure, as core).
// Constraints: the registration token comes from --token or the
//   GITHUB_RUNNER_TOKEN env var and nowhere else (never the project .env),
//   and is never printed. Every check core made before touching a host is
//   kept, in core's order: --github-url, token, one --host, then the manifest.

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

//go:embed help/provision.txt help/verify.txt
var helpFS embed.FS

// Deps carries the process seams the commands use.
type Deps struct {
	Out, Err io.Writer
	Getenv   func(string) string
	// Executors builds one Executor per --host value (local when none).
	Executors func(hosts []string, sshKey string) []Executor
	Ctx       context.Context
}

// DefaultDeps is the real process wiring.
func DefaultDeps() Deps {
	return Deps{Out: os.Stdout, Err: os.Stderr, Getenv: os.Getenv, Executors: ExecutorsFromFlags, Ctx: context.Background()}
}

// ExecutorsFromFlags builds one Executor per --host flag value, or a single
// LocalExecutor when no --host is given. "local" names this machine.
func ExecutorsFromFlags(hosts []string, sshKey string) []Executor {
	if len(hosts) == 0 {
		return []Executor{LocalExecutor{}}
	}
	executors := make([]Executor, len(hosts))
	for i, h := range hosts {
		if h == "local" {
			executors[i] = LocalExecutor{}
			continue
		}
		executors[i] = NewSSHExecutor(h, sshKey)
	}
	return executors
}

// Help returns the captured core help for "provision" or "verify".
func Help(name string) (string, bool) {
	b, err := helpFS.ReadFile("help/" + name + ".txt")
	return string(b), err == nil
}

// finish prints a returned error the way core's main does ("Error: <err>" on
// stderr) and returns core's exit status: the ExitCode of any error in the
// chain that has one (an ssh or local process failure carries that process's
// status, such as 255 for an ssh connection error), otherwise 1.
func finish(d Deps, err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintf(d.Err, "Error: %v\n", err)
	var coder interface{ ExitCode() int }
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return 1
}

// parseCommand parses args into fs; it reports whether help was printed.
func parseCommand(fs *flagSet, name string, args []string, d Deps) (done bool, code int) {
	if err := fs.parse(args); err != nil {
		return true, finish(d, err)
	}
	if fs.help.on {
		txt, _ := Help(name)
		fmt.Fprint(d.Out, txt)
		return true, 0
	}
	return false, 0
}

// RunProvision is `nself ci nodes provision`.
func RunProvision(args []string, d Deps) int {
	fs := newFlagSet()
	host := fs.add("host", kindSlice)
	sshKey := fs.add("ssh-key", kindString)
	instances := fs.add("instances", kindInt)
	instances.num = 1
	installRoot := fs.add("install-root", kindString)
	githubURL := fs.add("github-url", kindString)
	labels := fs.add("labels", kindSlice)
	token := fs.add("token", kindString)
	asJSON := fs.add("json", kindBool)
	fs.add("no-monorepo", kindBool)
	fs.add("no-deprecation-warnings", kindBool)
	if done, code := parseCommand(fs, "provision", args, d); done {
		return code
	}
	if asJSON.on {
		return finish(d, fmt.Errorf("nself ci nodes provision does not support --json"))
	}
	tok := token.str
	if tok == "" {
		tok = d.Getenv("GITHUB_RUNNER_TOKEN")
	}
	if githubURL.str == "" {
		return finish(d, fmt.Errorf("--github-url is required (the repo or org runners register against)"))
	}
	if tok == "" {
		return finish(d, fmt.Errorf("a runner registration token is required: pass --token or set GITHUB_RUNNER_TOKEN"))
	}
	if len(host.list) > 1 {
		return finish(d, fmt.Errorf("provision takes at most one --host per invocation; run it once per host"))
	}
	m, err := LoadEmbeddedManifest()
	if err != nil {
		return finish(d, err)
	}
	ex := d.Executors(host.list, sshKey.str)[0]
	opts := ProvisionOptions{Instances: instances.num, InstallRoot: installRoot.str,
		GithubURL: githubURL.str, RegToken: tok, Labels: labels.list}
	fmt.Fprintf(d.Out, "Provisioning %s (%d instance(s))...\n", ex.Label(), opts.Instances)
	result, err := Provision(d.Ctx, ex, m, opts)
	for _, step := range result.Steps {
		fmt.Fprintf(d.Out, "  [%s] %s\n", step.Name, step.Output)
	}
	if err != nil {
		return finish(d, err)
	}
	fmt.Fprintln(d.Out, "Provision complete.")
	return 0
}

// RunVerify is `nself ci nodes verify`.
func RunVerify(args []string, d Deps) int {
	fs := newFlagSet()
	host := fs.add("host", kindSlice)
	sshKey := fs.add("ssh-key", kindString)
	asJSON := fs.add("json", kindBool)
	fs.add("no-monorepo", kindBool)
	fs.add("no-deprecation-warnings", kindBool)
	if done, code := parseCommand(fs, "verify", args, d); done {
		return code
	}
	m, err := LoadEmbeddedManifest()
	if err != nil {
		return finish(d, err)
	}
	reports := VerifyHosts(d.Ctx, d.Executors(host.list, sshKey.str), m)
	if asJSON.on {
		enc := json.NewEncoder(d.Out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(reports); err != nil {
			return finish(d, err)
		}
	} else {
		fmt.Fprint(d.Out, RenderMatrix(reports))
	}
	if FoundProblems(reports) {
		return 1 // output is already written, as core's silent errs.Exit(1)
	}
	return 0
}

// FoundProblems reports true when any reachable host has a failing check,
// any host is entirely unreachable, or any check drifts across hosts: the
// three conditions that make `verify` a usable CI gate, not just an FYI.
func FoundProblems(reports []HostReport) bool {
	for _, r := range reports {
		if r.Err != "" {
			return true
		}
		for _, c := range r.Checks {
			if c.Status == StatusFail {
				return true
			}
		}
	}
	return len(DetectDrift(reports)) > 0
}
