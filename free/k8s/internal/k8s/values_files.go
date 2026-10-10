// Purpose: locate the generated chart values and build the install overrides
// file, so no secret and no licence key ever reaches a process argv.
//
// Inputs: the project directory (holds .nself/generated/k8s), the domain,
// licence key and plugin list of an install.
//
// Outputs: absolute paths of values.yaml and secrets.yaml, and an overrides
// file (mode 0600) written inside the private temp directory of the chart.
//
// Constraints: install and upgrade refuse (exit 1, "run nself k8s values")
// when either generated file is missing. The overrides file is the only place
// the licence key is written; helm reads it with --values.
package k8s

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// GeneratedDir is the project-relative directory `nself k8s values` writes to.
const GeneratedDir = ".nself/generated/k8s"

// ErrValuesMissing is wrapped by ResolveValues when a generated file is absent.
var ErrValuesMissing = errors.New("generated chart values not found")

// ValuesFiles holds the absolute paths of the generated chart values.
type ValuesFiles struct {
	Values  string
	Secrets string
}

// ResolveValues returns the generated values.yaml and secrets.yaml of the
// project in dir (default "."). A missing file is an error that tells the
// user to run `nself k8s values`.
func ResolveValues(dir string) (ValuesFiles, error) {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ValuesFiles{}, fmt.Errorf("k8s: project dir: %w", err)
	}
	vf := ValuesFiles{
		Values:  filepath.Join(abs, filepath.FromSlash(GeneratedDir), "values.yaml"),
		Secrets: filepath.Join(abs, filepath.FromSlash(GeneratedDir), "secrets.yaml"),
	}
	for _, p := range []string{vf.Values, vf.Secrets} {
		if st, err := os.Stat(p); err != nil || !st.Mode().IsRegular() {
			return ValuesFiles{}, fmt.Errorf("k8s: %w: %s: run nself k8s values", ErrValuesMissing, filepath.Join(GeneratedDir, filepath.Base(p)))
		}
	}
	return vf, nil
}

// overrides is the install-time values overlay. Field names follow the chart
// keys domain, license.key and plugins.install.
type overrides struct {
	Domain  string          `yaml:"domain,omitempty"`
	License *overrideLicKey `yaml:"license,omitempty"`
	Plugins *overridePlugin `yaml:"plugins,omitempty"`
}

type overrideLicKey struct {
	Key string `yaml:"key"`
}

type overridePlugin struct {
	Install []string `yaml:"install"`
}

// writeOverrides writes the overlay into dir (mode 0600) and returns its
// path, or "" when there is nothing to override. plugins.install is a YAML
// list, so its indices are 0..n-1 by construction.
func writeOverrides(dir string, opts InstallOptions) (string, error) {
	o := overrides{Domain: opts.Domain}
	if opts.LicenseKey != "" {
		o.License = &overrideLicKey{Key: opts.LicenseKey}
	}
	if len(opts.Plugins) > 0 {
		o.Plugins = &overridePlugin{Install: opts.Plugins}
	}
	if o.Domain == "" && o.License == nil && o.Plugins == nil {
		return "", nil
	}
	data, err := yaml.Marshal(o)
	if err != nil {
		return "", fmt.Errorf("k8s: encode overrides: %w", err)
	}
	p := filepath.Join(dir, "install-overrides.yaml")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return "", fmt.Errorf("k8s: write overrides: %w", err)
	}
	return p, nil
}
