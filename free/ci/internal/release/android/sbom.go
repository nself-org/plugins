package android

// Purpose: the SBOM step of the lane (Epic CACHE D13/D18). After gradle exited 0
// and before the artifacts are collected, inventory each release APK with the
// pinned syft and write both documents where the publication looks for them:
// .nself-ci/sbom/android/<file>.apk.{cdx,spdx}.json, next to the record name
// android/<file>.apk.
// Inputs:  the finished job's workspace (an *os.Root the executor opened), the
// job, the CI home and the syft pin.
// Outputs: the documents inside the workspace; an E718 error (tool_missing,
// tool_version, tool_checksum, invalid) or a plain error when there is no APK.
// Constraints: only the pinned syft installed by `nself ci artifacts tools
// install` runs, through internal/sbom (never a syft on PATH); every write goes
// through the workspace root, so nothing lands outside it; an APK with no
// cataloguable component is accepted (an Android app has no package-manager
// database inside; the documents still bind the APK digest and syft version).

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sbom"
)

// SBOMStep generates the SBOM documents of the APKs a job built.
type SBOMStep struct {
	// Home is NSELF_CI_HOME ("" = the default CI home).
	Home string
	Pin  sbom.Pin
	// NewRunner builds the syft runner over the directory holding the APKs
	// (default: sbom.ExecRunner over the exec funnel, process isolation).
	NewRunner func(sourceDir string) sbom.Runner
}

// Applies reports whether job is the lane's job: a release job named android that declares the lane's APK
// artifact. A project's own android job with other outputs makes its own SBOMs (the publication needs two per artifact).
func Applies(name string, job model.Job) bool {
	if name != JobName || job.Kind != "release" {
		return false
	}
	for _, a := range job.Artifacts {
		if a.Path == APKGlob && a.Name == ArtifactName {
			return true
		}
	}
	return false
}

// Run inventories the APKs under the job's workdir in ws.
func (s SBOMStep) Run(ctx context.Context, job model.Job, ws *os.Root) error {
	workdir := path.Clean(job.Workdir)
	if job.Workdir == "" {
		workdir = "."
	}
	apkRel := path.Join(workdir, APKDir)
	apkRoot, err := ws.OpenRoot(apkRel)
	if err != nil {
		return fmt.Errorf("no APK directory %s after the build: %w", apkRel, err)
	}
	defer func() { _ = apkRoot.Close() }()
	apks, err := fs.Glob(apkRoot.FS(), path.Base(APKGlob))
	if err != nil || len(apks) == 0 {
		return fmt.Errorf("the build produced no APK matching %s", APKGlob)
	}
	targets := make([]sbom.Target, len(apks))
	for i, a := range apks {
		targets[i] = sbom.Target{Name: fmt.Sprintf("apk%d", i+1), Path: a, Kind: sbom.SourceFile, AllowEmpty: true}
	}
	runner := s.runner(apkRoot.Name())
	syft, err := sbom.Resolve(ctx, runner, s.Home, sbom.ModeProcess, s.Pin)
	if err != nil {
		return err
	}
	docs, err := sbom.Generate(ctx, runner, syft, s.Pin, targets)
	if err != nil {
		return err
	}
	dir := path.Join(workdir, SBOMDir, ArtifactName)
	if err = ws.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for i, d := range docs {
		for ext, body := range map[string][]byte{".cdx.json": d.CDX, ".spdx.json": d.SPDX} {
			if err = ws.WriteFile(path.Join(dir, apks[i]+ext), body, 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s SBOMStep) runner(dir string) sbom.Runner {
	if s.NewRunner != nil {
		return s.NewRunner(dir)
	}
	return sbom.ExecRunner{Executor: &exec.Executor{}, Home: s.Home, SourceDir: dir}
}
