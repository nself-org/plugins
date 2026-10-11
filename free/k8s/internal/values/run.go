// Purpose: the behaviour behind `nself-k8s values [--check]`, kept out of
// package main so it is testable: resolve the compose model, then either
// write values.yaml and secrets.yaml or compare an existing values.yaml.
//
// Inputs: Options (project dir, output dir, check flag, Runner).
//
// Outputs: a Result (what was written, or the parity report) used by both the
// human text (Result.WriteText) and the --json document; a non-nil error is a
// *Error with a Kind, and means exit 1 in human mode.
//
// Constraints: secret values are never printed and never in a Result; check
// mode writes nothing. Run prints nothing: the caller decides the rendering.
package values

import (
	"context"
	"fmt"
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
