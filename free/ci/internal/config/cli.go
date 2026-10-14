package config

// Purpose: the logic of `nself ci config show` and `nself ci config migrate`,
// kept here (with injectable dependencies) so cmd/config.go stays a shim and
// the commands are testable without a process.
// Inputs:  argv after the command key, the output writers and Deps.
// Outputs: human text, or with --json one v1 envelope (contract:cli.json-
// envelope) whose data is showData or migrateData; the process exit code
// (0 ok, 1 usage or invalid config, 2 infrastructure).
// Constraints: `show` never writes; `migrate` writes `.nself/ci.yaml` only
// with --write; both report E623 as a warning; `show` refuses a leftover
// `.ci.yaml` (E621) because these commands run only under V15.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Deps are the process facts the commands read.
type Deps struct {
	Getenv  func(string) string
	Getwd   func() (string, error)
	HomeDir func() (string, error)
	Run     Runner
}

// DefaultDeps reads the real process.
func DefaultDeps() Deps {
	return Deps{Getenv: os.Getenv, Getwd: os.Getwd, HomeDir: os.UserHomeDir, Run: ExecRunner()}
}

type showData struct {
	Version     int                   `json:"version"`
	Path        string                `json:"path,omitempty"`
	FilePresent bool                  `json:"file_present"`
	Effective   Effective             `json:"effective"`
	Config      *model.PipelineConfig `json:"config,omitempty"`
	Warnings    []Warning             `json:"warnings"`
}

type migrateData struct {
	Sources  []string  `json:"sources"`
	Jobs     []string  `json:"jobs"`
	Diff     string    `json:"diff"`
	Written  bool      `json:"written"`
	Notes    []string  `json:"notes"`
	Warnings []Warning `json:"warnings"`
}

type showFlags struct {
	json                bool
	repo, timeout, home string
	cfg                 string
	noStatus            bool
}

func newShowFlags() (*flag.FlagSet, *showFlags) {
	o := &showFlags{}
	fs := flag.NewFlagSet("ci config show", flag.ContinueOnError)
	fs.BoolVar(&o.json, "json", false, "print one v1 JSON envelope")
	fs.StringVar(&o.repo, "repo", "", "repository root (env NSELF_CI_REPO; default: current directory)")
	fs.StringVar(&o.timeout, "timeout", "", "job timeout override (env NSELF_CI_TIMEOUT)")
	fs.StringVar(&o.home, "home", "", "state directory (env NSELF_CI_HOME)")
	fs.StringVar(&o.cfg, "config", "", "config file (default: <repo>/.nself/ci.yaml)")
	fs.BoolVar(&o.noStatus, "no-status", false, "do not post a commit status (env NSELF_CI_SKIP_STATUS)")
	return fs, o
}

// ShowFlags returns the flag set of `config show` for help generation.
func ShowFlags() *flag.FlagSet { fs, _ := newShowFlags(); return fs }

// MigrateFlags returns the flag set of `config migrate` for help generation.
func MigrateFlags() *flag.FlagSet { fs, _ := newMigrateFlags(); return fs }

type migrateFlags struct {
	json, write bool
	repo        string
}

func newMigrateFlags() (*flag.FlagSet, *migrateFlags) {
	o := &migrateFlags{}
	fs := flag.NewFlagSet("ci config migrate", flag.ContinueOnError)
	fs.BoolVar(&o.json, "json", false, "print one v1 JSON envelope")
	fs.BoolVar(&o.write, "write", false, "write .nself/ci.yaml (default: print the diff only)")
	fs.StringVar(&o.repo, "repo", "", "repository root (default: current directory)")
	return fs, o
}

// emitError renders an error in the requested form and returns its exit code.
func emitError(command string, asJSON bool, stdout, stderr io.Writer, err error) int {
	code, exit := CodeInvalid, 2
	var ce *Error
	if errors.As(err, &ce) {
		code, exit = ce.Code, 1
	}
	if asJSON {
		_ = output.WriteError(stdout, command, output.ErrorDetail{Code: code, Message: err.Error(), ExitCode: exit}, nil)
	} else {
		_, _ = fmt.Fprintln(stderr, err)
	}
	return exit
}

func warningTexts(ws []Warning) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.Code + ": " + w.Message
	}
	return out
}

func repoRoot(flagValue string, d Deps) (string, error) {
	if flagValue == "" {
		flagValue = d.Getenv("NSELF_CI_REPO")
	}
	if flagValue != "" {
		return filepath.Abs(flagValue)
	}
	return d.Getwd()
}

// ignoreWarning runs the E623 check; a check that cannot run says so.
func ignoreWarning(root string, d Deps) []Warning {
	w, err := CheckIgnored(context.Background(), root, d.Run)
	switch {
	case err != nil:
		return []Warning{{Code: CodeIgnored, Message: "could not check whether " + FileName + " is git-ignored: " + err.Error()}}
	case w != nil:
		return []Warning{*w}
	}
	return nil
}

// RunShow implements `nself ci config show [--json]`.
func RunShow(args []string, stdout, stderr io.Writer, d Deps) int {
	const command = "ci config show"
	fs, o := newShowFlags()
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	set := map[string]string{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = f.Value.String() })
	if o.noStatus {
		set["no-status"] = "true"
	}
	home, _ := d.HomeDir()
	cwd, err := d.Getwd()
	if err != nil {
		return emitError(command, o.json, stdout, stderr, err)
	}
	in := Inputs{Flags: set, Env: d.Getenv, Defaults: Defaults{Repo: cwd, Home: filepath.Join(home, ".nself", "ci")}}
	first, err := Resolve(in)
	if err != nil {
		return emitError(command, o.json, stdout, stderr, err)
	}
	root, err := repoRoot(first.Repo.Value.(string), d)
	if err != nil {
		return emitError(command, o.json, stdout, stderr, err)
	}
	if err := CheckLeftover(root); err != nil {
		return emitError(command, o.json, stdout, stderr, err)
	}
	path := o.cfg
	if path == "" {
		path = filepath.Join(root, FileName)
	}
	data := showData{Version: 1, Path: path, Warnings: []Warning{}}
	if loaded, err := Load(path); err == nil {
		in.Loaded = loaded
		data.FilePresent, data.Config, data.Version = true, &loaded.Config, loaded.Config.Version
		data.Warnings = append(data.Warnings, loaded.Warnings...)
		data.Warnings = append(data.Warnings, ignoreWarning(root, d)...)
	} else if !errors.Is(err, os.ErrNotExist) {
		return emitError(command, o.json, stdout, stderr, err)
	}
	if data.Effective, err = Resolve(in); err != nil {
		return emitError(command, o.json, stdout, stderr, err)
	}
	if o.json {
		if err := output.WriteData(stdout, command, data, &output.Meta{Warnings: warningTexts(data.Warnings)}); err != nil {
			return emitError(command, false, stdout, stderr, err)
		}
		return 0
	}
	printShow(stdout, stderr, data)
	return 0
}

func printShow(stdout, stderr io.Writer, d showData) {
	row := func(name string, v Value) {
		_, _ = fmt.Fprintf(stdout, "%-12s %-24v %s (%s)\n", name, v.Value, v.Source, v.Origin)
	}
	file := d.Path
	if !d.FilePresent {
		file += " (absent: defaults apply)"
	}
	_, _ = fmt.Fprintf(stdout, "config  %s\nversion %d\n", file, d.Version)
	row("repo", d.Effective.Repo)
	row("timeout", d.Effective.Timeout)
	row("skip_status", d.Effective.SkipStatus)
	row("home", d.Effective.Home)
	for _, id := range sortedKeys(d.Effective.Jobs) {
		j := d.Effective.Jobs[id]
		_, _ = fmt.Fprintf(stdout, "job %-20s timeout=%v (%s) required=%v (%s) path=%v retry=%v\n",
			id, j.Timeout.Value, j.Timeout.Source, j.Required.Value, j.Required.Source, j.Path.Value, j.RetryInfraMax.Value)
	}
	for _, w := range d.Warnings {
		_, _ = fmt.Fprintf(stderr, "warning %s: %s\n", w.Code, w.Message)
	}
}

// RunMigrate implements `nself ci config migrate [--write] [--json]`.
func RunMigrate(args []string, stdout, stderr io.Writer, d Deps) int {
	const command = "ci config migrate"
	fs, o := newMigrateFlags()
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	root, err := repoRoot(o.repo, d)
	if err != nil {
		return emitError(command, o.json, stdout, stderr, err)
	}
	m, err := Migrate(root)
	if errors.Is(err, ErrNothingToMigrate) {
		// Migration is idempotent: a repo with no legacy manifest is done.
		if o.json {
			empty := migrateData{Sources: []string{}, Jobs: []string{}, Notes: []string{}, Warnings: []Warning{}}
			_ = output.WriteData(stdout, command, empty, nil)
		} else {
			_, _ = fmt.Fprintln(stdout, "nothing to migrate: no .ci.yaml found")
		}
		return 0
	}
	if err != nil {
		return emitError(command, o.json, stdout, stderr, err)
	}
	diff, err := m.Diff(root)
	if err != nil {
		return emitError(command, o.json, stdout, stderr, err)
	}
	if o.write {
		if err := m.Write(root); err != nil {
			return emitError(command, o.json, stdout, stderr, err)
		}
	}
	data := migrateData{Sources: m.Sources, Jobs: m.Order, Diff: diff, Written: o.write, Notes: append([]string{}, m.Notes...), Warnings: []Warning{}}
	data.Warnings = append(data.Warnings, ignoreWarning(root, d)...)
	if o.json {
		if err := output.WriteData(stdout, command, data, &output.Meta{Warnings: warningTexts(data.Warnings)}); err != nil {
			return emitError(command, false, stdout, stderr, err)
		}
		return 0
	}
	if diff == "" {
		_, _ = fmt.Fprintln(stdout, FileName+" already holds the migrated jobs")
	} else {
		_, _ = fmt.Fprint(stdout, diff)
	}
	for _, n := range data.Notes {
		_, _ = fmt.Fprintln(stdout, "note:", n)
	}
	if o.write {
		_, _ = fmt.Fprintf(stdout, "wrote %s; delete the legacy file(s) by hand: %s\n", FileName, strings.Join(m.Sources, ", "))
	} else {
		_, _ = fmt.Fprintln(stdout, "nothing written; rerun with --write to create", FileName)
	}
	for _, w := range data.Warnings {
		_, _ = fmt.Fprintf(stderr, "warning %s: %s\n", w.Code, w.Message)
	}
	return 0
}
