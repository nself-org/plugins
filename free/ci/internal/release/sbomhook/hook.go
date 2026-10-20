// Package sbomhook writes the SBOMs of every release job that ends with the
// release.sbom check (debt D-1271, Epic CACHE D13).
package sbomhook

// Purpose: one workspace hook for every release job, not only the Android
// lane: after the job's command exited 0 and before its outputs are collected,
// inventory each artifact the job declares with the pinned syft and write both
// documents where the publication looks for them,
// .nself-ci/sbom/<record name>.{cdx,spdx}.json.
// Inputs:  the attempt (run.AttemptInfo) and its workspace root.
// Outputs: the documents inside the workspace; the check's E718 refusal as an
// error, which fails the attempt.
// Constraints: only a release job whose run ends with release.sbom is touched
// (the entry is dropped from the command by gates/sbom.StripCheck, never run);
// the check itself is gates/sbom.Run, so the tool, its pin and the failure
// rules are the ones that check defines; every write goes through the
// workspace root; the artifacts are the job's own declarations (kind sbom
// excluded) expanded exactly as the collector will; Register puts the hook
// AHEAD of the registered ones, because the artifacts hook collects what this
// one writes.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/exec"
	sbomgate "github.com/nself-org/plugins/free/ci/internal/gates/sbom"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sbom"
)

// Hook is the workspace hook.
type Hook struct {
	// Pin returns the syft pin (default: the one compiled into the binary).
	Pin func() (sbom.Pin, error)
	// NewRunner builds the syft runner for job over sourceDir, the directory
	// holding the artifacts (default: sbom.ExecRunner over the exec funnel,
	// under the job's own isolation).
	NewRunner func(job model.Job, home, sourceDir string) sbom.Runner
}

// Applies reports whether the hook has work for job.
func Applies(job model.Job) bool {
	return job.Kind == "release" && sbomgate.RunsCheck(job) && sbomgate.RequireForRelease(job) == nil
}

// AfterJob implements run.WorkspaceHook.
func (h Hook) AfterJob(ctx context.Context, a run.AttemptInfo, ws *os.Root) error {
	if !Applies(a.Job) {
		return nil
	}
	workdir := path.Clean(a.Job.Workdir)
	root, err := ws.OpenRoot(workdir)
	if err != nil {
		return fmt.Errorf("the job workdir %s is missing after the build: %w", workdir, err)
	}
	defer func() { _ = root.Close() }()
	decls, err := records.Normalize(a.Job, true, nil)
	if err != nil {
		return err
	}
	var files []records.Named
	if files, err = records.Names(ctx, root, releasable(decls)); err != nil {
		return err
	}
	if len(files) == 0 {
		return sbom.Failf(sbom.ReasonInvalid, "the job declares no released artifact to inventory (%s)", sbomgate.CheckID)
	}
	pin, err := h.pin()
	if err != nil {
		return err
	}
	// One syft run per artifact, each over its own directory: the runner copies
	// its source dir into a private workspace, and the build tree is not needed.
	groups := map[string][]int{}
	var dirs []string
	for i, f := range files {
		d := path.Dir(f.Rel)
		if _, seen := groups[d]; !seen {
			dirs = append(dirs, d)
		}
		groups[d] = append(groups[d], i)
	}
	for _, d := range dirs {
		idx := groups[d]
		targets := make([]sbom.Target, len(idx))
		for j, i := range idx {
			kind := sbom.SourceFile
			if files[i].Decl.Kind == records.KindImage {
				kind = sbom.SourceImage
			}
			targets[j] = sbom.Target{Name: fmt.Sprintf("artifact%d", j+1), Path: path.Base(files[i].Rel), Kind: kind}
		}
		in := sbomgate.Input{Job: a.Job, Runner: h.runner(a.Job, a.Home, filepath.Join(root.Name(), filepath.FromSlash(d))), Home: a.Home, Pin: pin, Targets: targets}
		out := sbomgate.Run(ctx, in)
		if out.Check.Result != "pass" {
			return errors.New(out.Check.Excerpt)
		}
		for j, doc := range out.Docs {
			if err = writeDocs(ws, workdir, files[idx[j]].Name, doc); err != nil {
				return err
			}
		}
	}
	return nil
}

// releasable drops the declarations the publication does not treat as released
// files: its own SBOMs, logs, coverage and benchmarks.
func releasable(decls []records.Decl) []records.Decl {
	out := make([]records.Decl, 0, len(decls))
	for _, d := range decls {
		switch d.Kind {
		case records.KindSBOM, records.KindLog, records.KindCoverage, records.KindBench:
		default:
			out = append(out, d)
		}
	}
	return out
}

func writeDocs(ws *os.Root, workdir, record string, d sbom.Doc) error {
	base := path.Join(workdir, sbom.OutDir, record)
	if err := ws.MkdirAll(path.Dir(base), 0o700); err != nil {
		return err
	}
	if err := ws.WriteFile(base+".cdx.json", d.CDX, 0o600); err != nil {
		return err
	}
	return ws.WriteFile(base+".spdx.json", d.SPDX, 0o600)
}

func (h Hook) pin() (sbom.Pin, error) {
	if h.Pin != nil {
		return h.Pin()
	}
	return sbom.EmbeddedPin()
}

func (h Hook) runner(job model.Job, home, dir string) sbom.Runner {
	if h.NewRunner != nil {
		return h.NewRunner(job, home, dir)
	}
	iso := string(job.Isolation)
	if iso == "" {
		iso = "process"
	}
	return sbom.ExecRunner{Executor: &exec.Executor{}, Home: home, SourceDir: dir, Isolation: iso, Container: job.Container}
}

// Register puts the hook ahead of the registered workspace hooks and returns the function that removes it.
func Register(h Hook) func() {
	old := run.Registered.Workspaces
	run.Registered.Workspaces = append([]run.WorkspaceHook{h}, old...)
	return func() { run.Registered.Workspaces = old }
}
