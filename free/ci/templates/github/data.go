// Package githubtmpl is the template set of the GitHub-hosted dispatch (P7-HOST-03, Epic D5):
// the workflow template, the runner label allowlist, the per-OS minute multipliers, the artifact
// redirect hosts and the pinned actions, and the renderer that turns them into the one workflow
// file `nself ci providers github init` writes.
package githubtmpl

// Purpose: load the embedded data files (labels, multipliers, redirect hosts, action pins).
// Inputs:  the embedded *.yaml files of this directory.
// Outputs: typed, validated views; a bad embedded file is a build bug and panics in the tests,
//          never at a customer.
// Constraints: stdlib and yaml.v3 only; no I/O beyond the embedded files; every label comes from
//          labels.yaml and a label that is larger-runner shaped (-large, -xlarge) is refused here
//          even if someone added it to the file.

import (
	"embed"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed nself-ci-dispatch.yml.tmpl actions.yaml agent_pins.yaml labels.yaml multipliers.yaml redirect_hosts.yaml
var files embed.FS

// Label is one allowed runner label and the platform it provides.
type Label struct {
	Label string `yaml:"label"`
	OS    string `yaml:"os"`
	Arch  string `yaml:"arch"`
}

// Pin is one action pinned by full commit SHA.
type Pin struct {
	Uses    string `yaml:"uses"`
	Version string `yaml:"version"`
}

var (
	labelRE = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)
	pinRE   = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}$`)
)

func read(name string, into any) error {
	raw, err := files.ReadFile(name)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(raw, into)
}

// Labels returns the allowed runner labels, in file order.
func Labels() ([]Label, error) {
	var doc struct {
		Labels []Label `yaml:"labels"`
	}
	if err := read("labels.yaml", &doc); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, l := range doc.Labels {
		switch {
		case !labelRE.MatchString(l.Label), strings.Contains(l.Label, "large"), seen[l.Label]:
			return nil, fmt.Errorf("labels.yaml: label %q is not an allowed standard label", l.Label)
		case l.OS == "" || l.Arch == "":
			return nil, fmt.Errorf("labels.yaml: label %q has no platform", l.Label)
		}
		seen[l.Label] = true
	}
	return doc.Labels, nil
}

// LabelNames returns just the label strings.
func LabelNames() ([]string, error) {
	ls, err := Labels()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.Label
	}
	return out, nil
}

// Multiplier is the minute multiplier of an OS ("linux", "windows", "macos"); 0 when unknown.
func Multiplier(os string) (int, error) {
	var doc struct {
		Multipliers map[string]int `yaml:"multipliers"`
	}
	if err := read("multipliers.yaml", &doc); err != nil {
		return 0, err
	}
	return doc.Multipliers[os], nil
}

// RedirectHosts returns the hosts an artifact download may be redirected to.
func RedirectHosts() ([]string, error) {
	var doc struct {
		Hosts []string `yaml:"hosts"`
	}
	if err := read("redirect_hosts.yaml", &doc); err != nil {
		return nil, err
	}
	return doc.Hosts, nil
}

// Actions returns the pinned actions by short name (checkout, setup-go, upload-artifact).
func Actions() (map[string]Pin, error) {
	var doc struct {
		Actions map[string]Pin `yaml:"actions"`
	}
	if err := read("actions.yaml", &doc); err != nil {
		return nil, err
	}
	for name, p := range doc.Actions {
		if !pinRE.MatchString(p.Uses) || p.Version == "" {
			return nil, fmt.Errorf("actions.yaml: %s is not pinned by a full commit SHA with a version", name)
		}
	}
	return doc.Actions, nil
}

// SourceCommits returns the commits of nself-org/plugins the agent may be built from, oldest first.
func SourceCommits() ([]string, error) {
	var doc struct {
		Commits []string `yaml:"source_commits"`
	}
	if err := read("agent_pins.yaml", &doc); err != nil {
		return nil, err
	}
	for _, c := range doc.Commits {
		if !commitRE.MatchString(c) {
			return nil, fmt.Errorf("agent_pins.yaml: %q is not a full commit id", c)
		}
	}
	return doc.Commits, nil
}
