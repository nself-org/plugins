// Purpose: `helm uninstall` for the nSelf release.
//
// Inputs: UninstallOptions (release name, optional kubeconfig, output seam).
//
// Outputs: helm's stdout (Stdout, default os.Stdout) and stderr (copied to
// os.Stderr); a *HelmError on failure, with NotFound set when helm says the
// release does not exist.
//
// Constraints: the caller has already confirmed the removal (--yes or the
// release-name prompt): this file never asks. helm runs with the same cleaned
// environment and binary lookup as install (helmEnv, helmBinary). Only the
// release name and --kubeconfig go on argv; helm's stderr is matched for the
// not-found case and never kept or returned.
package k8s

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// UninstallOptions holds parameters for helm uninstall.
type UninstallOptions struct {
	ReleaseName string
	Kubeconfig  string
	// Stdout receives helm's standard output (default os.Stdout).
	Stdout io.Writer
}

// uninstallArgs returns the helm argument list after the binary name.
func uninstallArgs(opts UninstallOptions) []string {
	release := opts.ReleaseName
	if release == "" {
		release = HelmReleaseName
	}
	args := []string{"uninstall", release}
	if opts.Kubeconfig != "" {
		args = append(args, "--kubeconfig", opts.Kubeconfig)
	}
	return args
}

// Uninstall runs `helm uninstall` for the release.
func Uninstall(ctx context.Context, opts UninstallOptions) error {
	helm, err := helmBinary()
	if err != nil {
		return err
	}
	announceTarget(os.Stderr, opts.Kubeconfig)
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, helm, uninstallArgs(opts)...)
	cmd.Stdout = opts.Stdout
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	cmd.Env = helmEnv(os.Environ())
	if err := cmd.Run(); err != nil {
		return &HelmError{Verb: "uninstall", Err: err, NotFound: releaseMissing(stderr.Bytes()),
			text: fmt.Sprintf("k8s: helm uninstall: %v", err)}
	}
	return nil
}
