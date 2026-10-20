// Package gitlabtmpl is the template set of the GitLab pipeline dispatch (P7-HOST-06, Epic D11): the
// include template, the runner tag allowlist, the artifact redirect hosts, the pinned build image and
// agent commits, and the renderer that turns them into the one include file
// `nself ci providers gitlab init` writes.
package gitlabtmpl

// Purpose: load the embedded data files (runner tags, redirect hosts, agent pins, image).
// Inputs:  the embedded *.yaml files of this directory.
// Outputs: typed, validated views; a bad embedded file is a build bug and fails the tests, never a
//          customer.
// Constraints: stdlib and yaml.v3 only; no I/O beyond the embedded files; a tag that is larger-runner
//          shaped ("large") is refused here even if someone added it to the file; the image is pinned
//          by digest.

import (
	"embed"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed nself-ci.gitlab-ci.yml.tmpl runner_tags.yaml redirect_hosts.yaml agent_pins.yaml image.yaml
var files embed.FS

// Tag is one allowed runner tag and the platform it provides.
type Tag struct {
	Tag  string `yaml:"tag"`
	OS   string `yaml:"os"`
	Arch string `yaml:"arch"`
}

var (
	tagRE    = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	commitRE = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
	imageRE  = regexp.MustCompile(`^[a-z0-9][a-z0-9./_-]*:[A-Za-z0-9._-]+@sha256:[0-9a-f]{64}$`)
)

func read(name string, into any) error {
	raw, err := files.ReadFile(name)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(raw, into)
}

// Tags returns the allowed runner tags, in file order.
func Tags() ([]Tag, error) {
	var doc struct {
		Tags []Tag `yaml:"tags"`
	}
	if err := read("runner_tags.yaml", &doc); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, t := range doc.Tags {
		switch {
		case !tagRE.MatchString(t.Tag), strings.Contains(t.Tag, "large"), seen[t.Tag]:
			return nil, fmt.Errorf("runner_tags.yaml: tag %q is not an allowed small shared-runner tag", t.Tag)
		case t.OS == "" || t.Arch == "":
			return nil, fmt.Errorf("runner_tags.yaml: tag %q has no platform", t.Tag)
		}
		seen[t.Tag] = true
	}
	if len(doc.Tags) == 0 {
		return nil, fmt.Errorf("runner_tags.yaml: no tags")
	}
	return doc.Tags, nil
}

// TagNames returns just the tag strings.
func TagNames() ([]string, error) {
	ts, err := Tags()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Tag
	}
	return out, nil
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

// SourceCommits returns the commits of nself-org/plugins the agent may be built from, oldest first.
func SourceCommits() ([]string, error) {
	var doc struct {
		Commits []string `yaml:"source_commits"`
	}
	if err := read("agent_pins.yaml", &doc); err != nil {
		return nil, err
	}
	if len(doc.Commits) == 0 {
		return nil, fmt.Errorf("agent_pins.yaml: no commits")
	}
	for _, c := range doc.Commits {
		if !commitRE.MatchString(c) {
			return nil, fmt.Errorf("agent_pins.yaml: %q is not a full commit id", c)
		}
	}
	return doc.Commits, nil
}

// Image returns the digest-pinned image of the job.
func Image() (string, error) {
	var doc struct {
		Image string `yaml:"image"`
	}
	if err := read("image.yaml", &doc); err != nil {
		return "", err
	}
	if !imageRE.MatchString(doc.Image) {
		return "", fmt.Errorf("image.yaml: %q is not pinned by digest", doc.Image)
	}
	return doc.Image, nil
}
