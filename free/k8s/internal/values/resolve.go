// Purpose: resolve the project's compose model exactly as `nself restart`
// does: `docker compose --env-file <each line of .nself/compose-env-files.txt>
// -f <each line of .nself/compose-files.txt> config --format json`. Compose
// itself performs ${VAR} interpolation and plugin env-file handling; this
// package never parses either.
//
// Inputs: a project directory holding the two manifests (contract
// build.generated-compose), and a Runner (injectable for tests).
//
// Outputs: Model (Resolve), the argv (Argv).
//
// Constraints: no cli import. Fail closed when the env-file manifest is
// missing but .nself/compose.env exists (stale build). The runner's stdout is
// the compose JSON and holds secrets: it is never logged and errors carry
// only a short stderr excerpt. The pull-fallback image override file is a
// machine-local mirror and is deliberately not part of the k8s model.
package values

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	composeManifest    = ".nself/compose-files.txt"
	envManifest        = ".nself/compose-env-files.txt"
	composeEnvFile     = ".nself/compose.env"
	userOverrideFile   = "docker-compose.override.yml"
	defaultComposeFile = "docker-compose.yml"
)

// Runner runs argv[0] with argv[1:] in dir and returns stdout.
type Runner func(ctx context.Context, dir string, argv []string) ([]byte, error)

// ReadManifests returns the compose files and env files in the order
// `nself restart` passes them. Relative lines resolve against projectDir.
func ReadManifests(projectDir string) (composeFiles, envFiles []string, err error) {
	lines, err := readLines(filepath.Join(projectDir, composeManifest))
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	if len(lines) == 0 {
		lines = []string{defaultComposeFile}
	}
	for _, l := range lines {
		composeFiles = append(composeFiles, abs(projectDir, l))
	}
	if !listed(composeFiles, userOverrideFile) {
		if _, statErr := os.Stat(filepath.Join(projectDir, userOverrideFile)); statErr == nil {
			composeFiles = append(composeFiles, filepath.Join(projectDir, userOverrideFile))
		}
	}
	elines, err := readLines(filepath.Join(projectDir, envManifest))
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	if len(elines) == 0 {
		if _, statErr := os.Lstat(filepath.Join(projectDir, composeEnvFile)); statErr == nil {
			return nil, nil, fmt.Errorf("%s is missing or empty but %s exists: run nself build", envManifest, composeEnvFile)
		}
	}
	for _, l := range elines {
		envFiles = append(envFiles, abs(projectDir, l))
	}
	return composeFiles, envFiles, nil
}

// Argv builds the compose command line (without the leading binary).
func Argv(composeFiles, envFiles []string) []string {
	args := []string{"compose"}
	for _, e := range envFiles {
		args = append(args, "--env-file", e)
	}
	for _, f := range composeFiles {
		args = append(args, "-f", f)
	}
	return append(args, "config", "--format", "json")
}

// Resolve runs compose through run and parses the result.
func Resolve(ctx context.Context, projectDir string, run Runner) (*Model, error) {
	files, envs, err := ReadManifests(projectDir)
	if err != nil {
		return nil, err
	}
	out, err := run(ctx, projectDir, append([]string{"docker"}, Argv(files, envs)...))
	if err != nil {
		return nil, fmt.Errorf("docker compose config failed: %w", err)
	}
	return ParseModel(out)
}

// ExecRunner is the production Runner. If the docker CLI has no compose
// plugin it falls back to a standalone docker-compose binary.
func ExecRunner(ctx context.Context, dir string, argv []string) ([]byte, error) {
	out, errOut, err := execOnce(ctx, dir, argv)
	if err != nil && len(argv) > 2 && !hasComposePlugin(ctx, argv[0]) {
		if p, lerr := exec.LookPath("docker-compose"); lerr == nil {
			out, errOut, err = execOnce(ctx, dir, append([]string{p}, argv[2:]...))
		}
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, excerpt(errOut))
	}
	return out, nil
}

// hasComposePlugin reports whether `<docker> compose version` works.
func hasComposePlugin(ctx context.Context, docker string) bool {
	return exec.CommandContext(ctx, docker, "compose", "version").Run() == nil
}

func execOnce(ctx context.Context, dir string, argv []string) (out, errOut []byte, err error) {
	if len(argv) == 0 {
		return nil, nil, errors.New("empty command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err = cmd.Run()
	return so.Bytes(), se.Bytes(), err
}

func excerpt(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

func abs(dir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, p)
}

func listed(files []string, base string) bool {
	for _, f := range files {
		if filepath.Base(f) == base {
			return true
		}
	}
	return false
}
