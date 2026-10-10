// Purpose: the behaviour behind `nself-k8s values [--check]`, kept out of
// package main so it is testable: resolve the compose model, then either
// write values.yaml and secrets.yaml or compare an existing values.yaml.
//
// Inputs: Options (project dir, output dir, check flag, Runner, writer).
//
// Outputs: files in OutDir (generate mode) or a parity report on Out (check
// mode); a non-nil error means exit 1.
//
// Constraints: secret values are never printed; check mode writes nothing.
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
	Out        io.Writer
}

// Run executes one `values` invocation.
func Run(ctx context.Context, o Options) error {
	if o.OutDir == "" {
		o.OutDir = filepath.Join(o.ProjectDir, ".nself", "generated", "k8s")
	}
	if o.Runner == nil {
		o.Runner = ExecRunner
	}
	model, err := Resolve(ctx, o.ProjectDir, o.Runner)
	if err != nil {
		return err
	}
	if o.Check {
		return runCheck(o, model)
	}
	routes, err := LoadRoutes(o.ProjectDir)
	if err != nil {
		return err
	}
	v, s, err := Map(model, routes)
	if err != nil {
		return err
	}
	if err := Write(o.OutDir, v, s); err != nil {
		return err
	}
	fmt.Fprintf(o.Out, "wrote %s and %s (%d mapped, %d unsupported)\n",
		filepath.Join(o.OutDir, ValuesFile), filepath.Join(o.OutDir, SecretsFile), len(v.Services), len(v.Unsupported))
	for _, u := range v.Unsupported {
		fmt.Fprintf(o.Out, "unsupported: %s: %s\n", u.Name, u.Reason)
	}
	for _, u := range v.UnmappedRoute {
		fmt.Fprintf(o.Out, "unmapped route: %s: %s\n", u.Name, u.Reason)
	}
	return nil
}

func runCheck(o Options, model *Model) error {
	path := filepath.Join(o.OutDir, ValuesFile)
	existing, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading existing values (run nself k8s values first): %w", err)
	}
	diffs, err := Check(existing, model)
	if err != nil {
		return err
	}
	for _, d := range diffs {
		fmt.Fprintln(o.Out, d)
	}
	if len(diffs) > 0 {
		return fmt.Errorf("parity check failed: %d difference(s) between values.yaml and the compose model", len(diffs))
	}
	fmt.Fprintf(o.Out, "parity ok: %d compose service(s) match %s\n", len(model.Services), path)
	return nil
}
