package bench

// Purpose: the two fixture repositories the end-to-end scenarios run on: a
// tiny Go module and a medium Go plus Node repo (bench/testdata).
// Inputs:  the embedded fixture trees and a destination directory.
// Outputs: a committed git repository at the destination, with the V15 config
// (.nself/ci.yaml) naming exactly the checks the v1.4 gate runs on that repo.
// Constraints: files named *.fixture are stored with that suffix (embed refuses
// a go.mod, and a ci.yaml would be picked up by this repo's own gate) and are
// written without it; the git identity is fixed so the commit is hermetic.

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

//go:embed all:testdata/tiny all:testdata/medium
var fixtureFS embed.FS

// Fixture names.
const (
	Tiny   = "tiny"
	Medium = "medium"
)

// gitEnv makes git ignore the operator's configuration.
func gitEnv() []string {
	return append(baseEnv(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=cibench", "GIT_AUTHOR_EMAIL=cibench@example.invalid",
		"GIT_COMMITTER_NAME=cibench", "GIT_COMMITTER_EMAIL=cibench@example.invalid")
}

// baseEnv is the part of the operator's environment every child needs.
func baseEnv() []string {
	var env []string
	for _, k := range []string{"PATH", "HOME", "USER", "LANG", "TMPDIR", "GOCACHE", "GOMODCACHE", "GOPATH",
		"GOROOT", "GOFLAGS", "GOTOOLCHAIN", "GOPROXY", "GONOSUMDB", "GOSUMDB", "GOPRIVATE", "NVM_DIR", "PNPM_HOME",
		"DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_CONFIG", "COLIMA_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// Materialize writes fixture name under dst and commits it.
func Materialize(ctx context.Context, name, dst string) error {
	root := path.Join("testdata", name)
	err := fs.WalkDir(fixtureFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, root), "/")
		if rel == "" {
			return os.MkdirAll(dst, 0o755)
		}
		target := filepath.Join(dst, strings.TrimSuffix(filepath.FromSlash(rel), ".fixture"))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fixtureFS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		return fmt.Errorf("fixture %s: %w", name, err)
	}
	env := gitEnv()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"},
		{"-c", "commit.gpgsign=false", "commit", "-q", "-m", "fixture"}} {
		if _, err := must(ctx, append([]string{"git"}, args...), dst, env); err != nil {
			return err
		}
	}
	return nil
}
