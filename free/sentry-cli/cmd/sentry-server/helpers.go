package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Locating the project, and re-invoking nself.
//
// Purpose: `nself sentry-server provision` chains into `nself build` and
// `nself start` once it has a server. Inside the CLI those two helpers lived in
// deploy.go; here they need writing out, and one of them needs a real change.
//
// Inputs: the working directory, and the arguments to pass on.
//
// Outputs: the project root, and the exit status of the nself invocation.
//
// Constraints: runCLISelf in the CLI resolves the binary with os.Executable,
// meaning "run me again". That is correct there and wrong here: os.Executable
// in this process is nself-sentry-server, so chaining would re-run the plugin
// rather than the CLI. This looks up nself on PATH instead, and says so
// clearly when it is not there.

// projectMarkers are the files whose presence marks an nself project root. The
// same set the CLI uses to decide it is inside a project.
var projectMarkers = []string{".env", ".env.dev", ".env.staging", ".env.prod"}

// projectRoot walks up from the working directory to the nself project root,
// falling back to the working directory when there is no marker above it.
func projectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	dir := cwd
	for {
		for _, m := range projectMarkers {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd, nil
		}
		dir = parent
	}
}

// runCLISelf invokes the nself CLI with args, used to chain build and start
// after provisioning.
func runCLISelf(ctx context.Context, workdir string, args ...string) error {
	bin, err := exec.LookPath("nself")
	if err != nil || bin == "" {
		return fmt.Errorf("nself is not on PATH; this plugin chains into `nself %s` and cannot find the CLI", args[0])
	}
	c := exec.CommandContext(ctx, bin, args...)
	c.Dir = workdir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = os.Environ()
	return c.Run()
}
