package main

// Purpose: `nself ci artifacts list|show|get|prune [--json]` (P7-CACHE-03,
// Epic D6) and the registry later Tickets add `artifacts <sub>` to, plus the
// `--cache` view slot (`nself ci artifacts --cache <sub>`, P7-CACHE-02).
// Inputs:  argv after `artifacts`; env NSELF_CI_HOME, NSELF_CI_REPO (the
// viewer's project), NSELF_CI_ROLE (member|release|admin, default admin: the
// operator of a PERSONAL store).
// Outputs: a table, key/value lines or one v1 JSON envelope; exit 0, 1 (usage,
// not found) or 2 (infrastructure, refusal).
// Constraints: V15 only; side-effect class: list/show read, get writes only
// into a fresh empty directory (authz.ArtifactGetDest), prune deletes;
// "not found" and "not visible" print the same E713; nothing prints a secret.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

var (
	artifactsSubs  = map[string]handler{}
	artifactsCache handler
)

// registerArtifactsSub adds a `nself ci artifacts <name>` subcommand.
func registerArtifactsSub(name string, h handler) {
	if _, dup := artifactsSubs[name]; dup || strings.ContainsAny(name, " -") {
		panic("nself-ci: invalid artifacts subcommand " + name)
	}
	artifactsSubs[name] = h
}

// registerArtifactsCache fills the `--cache` view slot (once).
func registerArtifactsCache(h handler) {
	if artifactsCache != nil {
		panic("nself-ci: duplicate artifacts --cache view")
	}
	artifactsCache = h
}

func artifactsFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("artifacts", flag.ContinueOnError)
	fs.Bool("json", false, "print one v1 JSON envelope")
	return fs
}

func init() {
	registerV15("artifacts", subcommandEntry{V15Only: true, Flags: artifactsFlags, Handler: runArtifacts})
	registerArtifactsSub("list", artifactsList)
	registerArtifactsSub("show", artifactsShow)
	registerArtifactsSub("get", artifactsGet)
	registerArtifactsSub("prune", artifactsPrune)
}

const artifactsUsage = "usage: nself ci artifacts list [--run R] [--attempt A] [--name N] [--kind K] | show <id> | get <id> --dest <dir> | prune [--release] [--older-than D] [--dry-run] | --cache <sub>  (all take --json)"

func runArtifacts(args []string) int {
	switch {
	case len(args) > 0 && args[0] == "--cache":
		if artifactsCache == nil {
			fmt.Fprintln(os.Stderr, "nself ci artifacts --cache: the cache view is not installed in this build")
			return 1
		}
		return artifactsCache(args[1:])
	case len(args) > 0 && artifactsSubs[args[0]] != nil:
		return artifactsSubs[args[0]](args[1:])
	}
	fmt.Fprintln(os.Stderr, artifactsUsage)
	return 1
}

// artifactsEnv is what every subcommand needs.
type artifactsEnv struct {
	st     *store.Store
	reader records.Reader
	blobs  *cas.Store
	viewer authz.Viewer
	json   bool
}

func (e *artifactsEnv) close() { _ = e.st.Close() }

// parseArtifacts parses flags anywhere in args and returns the positionals.
func parseArtifacts(name string, args []string, define func(*flag.FlagSet)) (*flag.FlagSet, []string, error) {
	fs := flag.NewFlagSet("artifacts "+name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Bool("json", false, "print one v1 JSON envelope")
	fs.String("project", "", "viewer project (default: NSELF_CI_REPO or the working directory)")
	if define != nil {
		define(fs)
	}
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return fs, nil, err
		}
		if fs.NArg() == 0 {
			break
		}
		pos, args = append(pos, fs.Arg(0)), fs.Args()[1:]
	}
	return fs, pos, nil
}

func openArtifacts(fs *flag.FlagSet) (*artifactsEnv, error) {
	st, err := cirun.OpenStore(runsStorePath())
	if err != nil {
		return nil, err
	}
	blobs, err := cas.New(records.StoreDir(os.Getenv("NSELF_CI_HOME")))
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	project := fs.Lookup("project").Value.String()
	if project == "" {
		if project, err = evidenceRoot(""); err != nil {
			_ = st.Close()
			return nil, err
		}
	}
	role := authz.Role(firstOf(os.Getenv("NSELF_CI_ROLE"), string(authz.RoleAdmin)))
	return &artifactsEnv{st: st, blobs: blobs, reader: records.Reader{Ledger: st, Blobs: blobs}, viewer: authz.Viewer{Project: project, Role: role},
		json: fs.Lookup("json").Value.String() == "true"}, nil
}

// artifactsExit prints err and maps it to an exit status (E713 and E683 are the caller's: 1).
func artifactsExit(err error, command string, asJSON bool) int {
	code, status, class := cirun.CodeOf(err), output.ExitInfra, output.ClassInfra
	if code == records.CodeNotFound || code == authz.CodeArtifactPath {
		status, class = output.ExitUser, output.ClassUser
	}
	if asJSON {
		_ = output.WriteError(os.Stdout, command, output.ErrorDetail{Code: orDefault(code, "E607"), Message: err.Error(), ExitCode: status, Class: class}, nil)
		return status
	}
	fmt.Fprintln(os.Stderr, err)
	return status
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func artifactsList(args []string) int {
	var run, attempt, name, kind *string
	fs, _, err := parseArtifacts("list", args, func(fs *flag.FlagSet) {
		run, attempt = fs.String("run", "", "only this run"), fs.String("attempt", "", "only this attempt")
		name, kind = fs.String("name", "", "only this artifact name"), fs.String("kind", "", "only this kind")
	})
	if err != nil {
		return 1
	}
	env, err := openArtifacts(fs)
	if err != nil {
		return artifactsExit(err, "ci artifacts list", false)
	}
	defer env.close()
	recs, err := env.reader.List(context.Background(), env.viewer, records.Filter{Run: *run, Attempt: *attempt, Name: *name, Kind: records.Kind(*kind)})
	if err != nil {
		return artifactsExit(err, "ci artifacts list", env.json)
	}
	if env.json {
		return writeArtifactsJSON("ci artifacts list", orEmpty(recs))
	}
	for _, r := range recs {
		fmt.Printf("%-21s %-9s %12d  %s  %s%s\n", r.ID, r.Kind, r.Size, r.Name, shortDigest(r.SHA256), map[bool]string{true: "  [missing]"}[r.Missing])
	}
	return 0
}

func orEmpty(r []records.Record) []records.Record {
	if r == nil {
		return []records.Record{}
	}
	return r
}

func shortDigest(d string) string {
	if len(d) > 12 {
		return d[:12]
	}
	return d
}

func writeArtifactsJSON(command string, data any) int {
	if err := output.WriteData(os.Stdout, command, data, nil); err != nil {
		return artifactsExit(err, command, false)
	}
	return 0
}
