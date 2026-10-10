// Purpose: locate the generated chart values and build the install overrides
// file, so no secret and no licence key ever reaches a process argv.
//
// Inputs: the project directory (holds .nself/generated/k8s), the domain,
// licence key and plugin list of an install.
//
// Outputs: absolute paths of values.yaml and secrets.yaml, and the install
// overlay as YAML bytes (piped to helm on stdin, never written to disk).
//
// Constraints: install and upgrade refuse (exit 1, "run nself k8s values")
// when either generated file is missing, and refuse a secrets.yaml that group
// or others can read. Any other stat error (permissions) is reported as is. The
// licence key is not part of the overlay: the chart does not read it yet
// (D-0311) and helm would store it in every release revision Secret.
package k8s

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

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
		st, err := os.Stat(p)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return ValuesFiles{}, fmt.Errorf("k8s: cannot read %s: %w", filepath.Join(GeneratedDir, filepath.Base(p)), err)
		}
		if err != nil || !st.Mode().IsRegular() {
			return ValuesFiles{}, fmt.Errorf("k8s: %w: %s: run nself k8s values", ErrValuesMissing, filepath.Join(GeneratedDir, filepath.Base(p)))
		}
	}
	if st, err := os.Stat(vf.Secrets); err == nil && runtime.GOOS != "windows" && st.Mode().Perm()&0o077 != 0 {
		return ValuesFiles{}, fmt.Errorf("k8s: %s is readable by group or others (mode %04o): chmod 600 it, or run nself k8s values again",
			filepath.Join(GeneratedDir, "secrets.yaml"), st.Mode().Perm())
	}
	return vf, nil
}

// overrides is the install-time values overlay. Field names follow the chart
// keys domain and plugins.install. There is no licence key field on purpose.
type overrides struct {
	Domain  string          `yaml:"domain,omitempty"`
	Plugins *overridePlugin `yaml:"plugins,omitempty"`
}

type overridePlugin struct {
	Install []string `yaml:"install"`
}

// overridesYAML returns the overlay for helm's stdin, or nil when there is
// nothing to override. plugins.install is a YAML list, so its indices are
// 0..n-1 by construction.
func overridesYAML(opts InstallOptions) ([]byte, error) {
	o := overrides{Domain: opts.Domain}
	if len(opts.Plugins) > 0 {
		o.Plugins = &overridePlugin{Install: opts.Plugins}
	}
	if o.Domain == "" && o.Plugins == nil {
		return nil, nil
	}
	data, err := yaml.Marshal(o)
	if err != nil {
		return nil, fmt.Errorf("k8s: encode overrides: %w", err)
	}
	return data, nil
}
