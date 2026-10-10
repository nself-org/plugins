// Package k8s runs the Helm CLI for the nSelf chart that the plugin embeds.
//
// Purpose: install, upgrade and inspect the nSelf release. The chart is
// extracted from the embedded filesystem to a private temp directory for each
// call; the chart values come from `nself k8s values`
// (.nself/generated/k8s/{values,secrets}.yaml) and are passed with --values.
//
// Inputs: InstallOptions (project dir, embedded chart, release, kubeconfig,
// domain, licence key, plugins).
//
// Outputs: helm's stdout and stderr, inherited.
//
// Constraints: a secret value or the licence key never appears on argv: both
// travel in values files. The kubeconfig is passed with --kubeconfig only when
// the caller gives one (the --cluster flag); otherwise helm reads KUBECONFIG
// itself. There is no remote chart repository. buildArgs (args.go) assembles the
// argv so tests can check it without running helm.
package k8s

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
)

// HelmReleaseName is the default Helm release name used by nself k8s install.
const HelmReleaseName = "nself"

// DefaultTimeout is the default `helm --timeout` for install and upgrade.
const DefaultTimeout = "10m"

// ErrHelmNotFound is returned when the helm binary is not in PATH.
var ErrHelmNotFound = fmt.Errorf("k8s: helm binary not found; install from https://helm.sh")

// InstallOptions holds parameters for helm install/upgrade.
type InstallOptions struct {
	ReleaseName string
	Domain      string
	Kubeconfig  string
	LicenseKey  string
	Plugins     []string
	// ProjectDir holds .nself/generated/k8s (default ".").
	ProjectDir string
	// Chart is the filesystem holding the chart at ChartRoot (the embed.FS).
	Chart fs.FS
	// ChartRoot is the chart directory inside Chart.
	ChartRoot string
	// Wait makes helm wait until the release is ready; Timeout bounds it.
	Wait    bool
	Timeout string
}

// helmBinary locates the helm binary.
func helmBinary() (string, error) {
	path, err := exec.LookPath("helm")
	if err != nil {
		return "", ErrHelmNotFound
	}
	return path, nil
}

// Install runs `helm install` for the embedded nSelf chart.
func Install(ctx context.Context, opts InstallOptions) error {
	return run(ctx, "install", opts)
}

// Upgrade runs `helm upgrade` for the embedded nSelf chart. The generated
// values are applied afresh (no --reuse-values), so a service removed from the
// compose model disappears from the release.
func Upgrade(ctx context.Context, opts InstallOptions) error {
	return run(ctx, "upgrade", opts)
}

// run checks the generated values, extracts the chart and runs helm verb.
func run(ctx context.Context, verb string, opts InstallOptions) error {
	helm, err := helmBinary()
	if err != nil {
		return err
	}
	vf, err := ResolveValues(opts.ProjectDir)
	if err != nil {
		return err
	}
	root := opts.ChartRoot
	if root == "" {
		root = "charts/nself"
	}
	chartDir, cleanup, err := ExtractChart(opts.Chart, root)
	if err != nil {
		return err
	}
	defer cleanup()
	overrides, err := writeOverrides(parentDir(chartDir), opts)
	if err != nil {
		return err
	}
	args := buildArgs(verb, opts, chartDir, vf, overrides)
	cmd := exec.CommandContext(ctx, helm, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("k8s: helm %s: %w", verb, err)
	}
	return nil
}

// Status returns the Helm release status summary.
func Status(ctx context.Context, releaseName, kubeconfig string) (string, error) {
	helm, err := helmBinary()
	if err != nil {
		return "", err
	}
	if releaseName == "" {
		releaseName = HelmReleaseName
	}
	args := []string{"status", releaseName, "--output", "json"}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	cmd := exec.CommandContext(ctx, helm, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("k8s: helm status: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
