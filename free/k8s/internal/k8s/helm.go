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
// Constraints: a secret value never appears on argv: secrets travel in the
// generated secrets.yaml. The licence key is not given to helm at all until the
// chart reads it (D-0311): helm stores every value in the release Secret.
// The install overlay (domain, plugins) goes to helm on stdin (--values -), so
// no overlay file exists to leave behind. helm runs with a cleaned environment
// (env.go). The kubeconfig is passed with --kubeconfig only when the caller
// gives one (the --cluster flag); otherwise helm reads KUBECONFIG itself. There
// is no remote chart repository. buildArgs (args.go) assembles the argv so
// tests can check it without running helm.
package k8s

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
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
	// LicenseKey is read from the environment but not passed to helm until the
	// chart consumes it (D-0311); see HasUnusedLicence.
	LicenseKey string
	Plugins    []string
	// ProjectDir holds .nself/generated/k8s (default ".").
	ProjectDir string
	// Chart is the filesystem holding the chart at ChartRoot (the embed.FS).
	Chart fs.FS
	// ChartRoot is the chart directory inside Chart.
	ChartRoot string
	// Wait makes helm wait until the release is ready; Timeout bounds it.
	Wait    bool
	Timeout string
	// Stdout receives helm's standard output (default os.Stdout). The --json
	// mode points it at stderr so stdout carries only the envelope.
	Stdout io.Writer
}

// HelmError is a failed helm invocation. Error() is the exact text the plugin
// printed before errors were classified; the fields let cmd pick an error code
// without parsing that text.
type HelmError struct {
	Verb string // install, upgrade, status, uninstall
	Err  error  // the exec error (an *exec.ExitError for a non-zero helm exit)
	// NotFound is true when helm said the release does not exist.
	NotFound bool
	text     string
}

func (e *HelmError) Error() string { return e.text }
func (e *HelmError) Unwrap() error { return e.Err }

// ValuesError is a generated-values problem other than a missing file (a
// secrets.yaml that others can read, an unreadable file). Error() is the inner
// text, unchanged.
type ValuesError struct{ Err error }

func (e *ValuesError) Error() string { return e.Err.Error() }
func (e *ValuesError) Unwrap() error { return e.Err }

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
		if errors.Is(err, ErrValuesMissing) {
			return err
		}
		return &ValuesError{Err: err}
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
	overlay, err := overridesYAML(opts)
	if err != nil {
		return err
	}
	args := buildArgs(verb, opts, chartDir, vf, len(overlay) > 0)
	announceTarget(os.Stderr, opts.Kubeconfig)
	cmd := exec.CommandContext(ctx, helm, args...)
	cmd.Stdout = opts.Stdout
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = os.Stderr
	cmd.Env = helmEnv(os.Environ())
	if len(overlay) > 0 {
		cmd.Stdin = bytes.NewReader(overlay)
	}
	if err := cmd.Run(); err != nil {
		return runError(verb, opts, err)
	}
	return nil
}

// runError wraps a helm failure. A failed first install leaves the release in
// state "failed": helm then refuses both install ("cannot re-use a name that is
// still in use") and upgrade ("has no deployed releases"), so say how to leave it.
func runError(verb string, opts InstallOptions, err error) error {
	if verb != "install" {
		return &HelmError{Verb: verb, Err: err, text: fmt.Sprintf("k8s: helm %s: %v", verb, err)}
	}
	release := opts.ReleaseName
	if release == "" {
		release = HelmReleaseName
	}
	return &HelmError{Verb: verb, Err: err, text: fmt.Sprintf("k8s: helm install: %v\nIf the release was left in a failed state, remove it with 'helm uninstall %s' "+
		"(add --kubeconfig for your cluster) and run 'nself k8s install' again; 'nself k8s upgrade' only works once a "+
		"revision is deployed", err, release)}
}

// announceTarget tells which kubeconfig helm will use, so a stale current
// context that points at a real cluster is visible before anything is applied.
func announceTarget(w io.Writer, kubeconfig string) {
	switch {
	case kubeconfig != "":
		fmt.Fprintf(w, "k8s: target kubeconfig %s (--cluster)\n", kubeconfig)
	case os.Getenv("KUBECONFIG") != "":
		fmt.Fprintf(w, "k8s: target kubeconfig %s (KUBECONFIG)\n", os.Getenv("KUBECONFIG"))
	default:
		fmt.Fprintln(w, "k8s: target kubeconfig ~/.kube/config (current context)")
	}
}
