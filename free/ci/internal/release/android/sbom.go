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
// database inside; the documents still bind the APK digest and syft version),
// so the directory of each Gradle dependency lockfile is inventoried as well
// (gradle-deps-N, D-1271); a build with no lockfile gets the APK documents only.

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

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
	names := make([]string, len(docs))
	for i := range docs {
		names[i] = apks[i]
	}
	locks, err := s.lockfileDocs(ctx, ws, workdir, syft)
	if err != nil {
		return err
	}
	docs = append(docs, locks...)
	for _, d := range locks {
		names = append(names, d.Name)
	}
	dir := path.Join(workdir, SBOMDir, ArtifactName)
	if err = ws.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for i, d := range docs {
		for ext, body := range map[string][]byte{".cdx.json": d.CDX, ".spdx.json": d.SPDX} {
			if err = ws.WriteFile(path.Join(dir, names[i]+ext), body, 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

// lockfileDocs inventories the directory of every Gradle dependency lockfile
// under the workdir (D-1271): the APK documents bind the file but list no
// Gradle dependency, these do. Documents are named gradle-deps-N.
func (s SBOMStep) lockfileDocs(ctx context.Context, ws *os.Root, workdir, syft string) ([]sbom.Doc, error) {
	root, err := ws.OpenRoot(workdir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	dirs, err := lockfileDirs(root.FS())
	if err != nil {
		return nil, err
	}
	var docs []sbom.Doc
	for i, d := range dirs {
		target := sbom.Target{Name: fmt.Sprintf("gradle-deps-%d", i+1), Path: ".", Kind: sbom.SourceDir}
		out, err := sbom.Generate(ctx, s.runner(filepath.Join(root.Name(), filepath.FromSlash(d))), syft, s.Pin, []sbom.Target{target})
		if err != nil {
			return nil, err
		}
		docs = append(docs, out...)
	}
	return docs, nil
}

// maxLockDirs bounds the lockfile directories one build may inventory.
const maxLockDirs = 32

// skipDirs are never searched for lockfiles: build output, caches, VCS data and our own output.
var skipDirs = map[string]bool{"build": true, ".gradle": true, "node_modules": true, ".git": true, ".nself-ci": true}

// lockfileDirs lists, sorted and without duplicates, the directories holding a
// gradle.lockfile or a legacy gradle/dependency-locks/*.lockfile.
func lockfileDirs(fsys fs.FS) ([]string, error) {
	seen := map[string]bool{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		dir := path.Dir(p)
		if d.Name() == "gradle.lockfile" || (strings.HasSuffix(d.Name(), ".lockfile") && path.Base(dir) == "dependency-locks") {
			seen[dir] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(seen) > maxLockDirs {
		return nil, sbom.Failf(sbom.ReasonInvalid, "the build holds Gradle lockfiles in %d directories, more than the %d allowed", len(seen), maxLockDirs)
	}
	dirs := make([]string, 0, len(seen))
	for d := range seen {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return dirs, nil
}

func (s SBOMStep) runner(dir string) sbom.Runner {
	if s.NewRunner != nil {
		return s.NewRunner(dir)
	}
	return sbom.ExecRunner{Executor: &exec.Executor{}, Home: s.Home, SourceDir: dir}
}
