// Purpose: the behaviour behind `nself-k8s values [--check]`, kept out of
// package main so it is testable: resolve the compose model, then either
// write values.yaml and secrets.yaml or compare an existing values.yaml.
//
// Inputs: Options (project dir, output dir, check flag, Runner).
//
// Outputs: a Result (what was written, or the parity report) used by both the
// human text (Result.WriteText, byte-identical to the output before the --json
// work) and the --json document; a non-nil error is an *Error with a Kind, and
// means exit 1 in human mode.
//
// Constraints: secret values are never printed and never in a Result; check
// mode writes nothing. Run prints nothing: the caller decides the rendering.
package values

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Options configures Run.
type Options struct {
	ProjectDir string
	OutDir     string // default <ProjectDir>/.nself/generated/k8s
	Check      bool
	Runner     Runner // default ExecRunner
}

// Run executes one `values` invocation. The Result is valid, with its Diffs,
// even when the error is a parity failure.
func Run(ctx context.Context, o Options) (*Result, error) {
	if o.OutDir == "" {
		o.OutDir = filepath.Join(o.ProjectDir, ".nself", "generated", "k8s")
	}
	if o.Runner == nil {
		o.Runner = ExecRunner
	}
	model, err := Resolve(ctx, o.ProjectDir, o.Runner)
	if err != nil {
		return nil, &Error{Kind: KindCompose, Err: err}
	}
	if o.Check {
		return runCheck(o, model)
	}
	routes, err := LoadRoutes(o.ProjectDir)
	if err != nil {
		return nil, &Error{Kind: KindOther, Err: err}
	}
	v, s, err := Map(model, routes)
	if err != nil {
		return nil, &Error{Kind: KindOther, Err: err}
	}
	if err := Write(o.OutDir, v, s); err != nil {
		return nil, &Error{Kind: KindOther, Err: err}
	}
	return &Result{
		Mode:           ModeGenerate,
		ValuesFile:     filepath.Join(o.OutDir, ValuesFile),
		SecretsFile:    filepath.Join(o.OutDir, SecretsFile),
		Mapped:         len(v.Services),
		Unsupported:    v.Unsupported,
		UnmappedRoutes: v.UnmappedRoute,
	}, nil
}

func runCheck(o Options, model *Model) (*Result, error) {
	path := filepath.Join(o.OutDir, ValuesFile)
	existing, err := os.ReadFile(path)
	if err != nil {
		return nil, &Error{Kind: KindMissing, Err: fmt.Errorf("reading existing values (run nself k8s values first): %w", err)}
	}
	diffs, err := Check(existing, model)
	if err != nil {
		return nil, &Error{Kind: KindInvalid, Err: err}
	}
	res := &Result{Mode: ModeCheck, ValuesFile: path, Services: len(model.Services), Diffs: diffs}
	if len(diffs) > 0 {
		return res, &Error{Kind: KindParity, Err: fmt.Errorf("parity check failed: %d difference(s) between values.yaml and the compose model", len(diffs))}
	}
	return res, nil
}

// Modes of a Result.
const (
	ModeGenerate = "generate"
	ModeCheck    = "check"
)

// Failure kinds of an Error.
const (
	KindCompose = "compose" // docker compose config could not be resolved
	KindMissing = "missing" // --check without an existing values.yaml
	KindInvalid = "invalid" // the existing values.yaml cannot be read as values
	KindParity  = "parity"  // --check found differences
	KindOther   = "other"   // routes, mapping or write failure
)

// Error is a failed values run.
type Error struct {
	Kind string
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Item is one named entry with a reason (an unsupported service, an unmapped route).
type Item struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// Result is what a run did.
type Result struct {
	Mode           string        `json:"mode"`
	ValuesFile     string        `json:"values_file"`
	SecretsFile    string        `json:"secrets_file,omitempty"`
	Mapped         int           `json:"mapped"`
	Unsupported    []Unsupported `json:"-"`
	UnmappedRoutes []Unsupported `json:"-"`
	Services       int           `json:"services"`
	Diffs          []Diff        `json:"-"`
}

// Items converts an Unsupported list to Items (never nil, so JSON has []).
func Items(list []Unsupported) []Item {
	out := make([]Item, 0, len(list))
	for _, u := range list {
		out = append(out, Item{Name: u.Name, Reason: u.Reason})
	}
	return out
}

// WriteText prints the human report: the files written and what could not be
// mapped, or the parity differences and the parity-ok line.
func (r *Result) WriteText(w io.Writer) {
	if r.Mode == ModeCheck {
		for _, d := range r.Diffs {
			fmt.Fprintln(w, d)
		}
		if len(r.Diffs) == 0 {
			fmt.Fprintf(w, "parity ok: %d compose service(s) match %s\n", r.Services, r.ValuesFile)
		}
		return
	}
	fmt.Fprintf(w, "wrote %s and %s (%d mapped, %d unsupported)\n", r.ValuesFile, r.SecretsFile, r.Mapped, len(r.Unsupported))
	for _, u := range r.Unsupported {
		fmt.Fprintf(w, "unsupported: %s: %s\n", u.Name, u.Reason)
	}
	for _, u := range r.UnmappedRoutes {
		fmt.Fprintf(w, "unmapped route: %s: %s\n", u.Name, u.Reason)
	}
}
