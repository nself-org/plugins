package githubtmpl

import (
	"strings"
	"testing"
)

const fixtureCommit = "0123456789abcdef0123456789abcdef01234567"

func TestDataFiles(t *testing.T) {
	labels, err := Labels()
	if err != nil || len(labels) != 11 {
		t.Fatalf("labels: %d %v", len(labels), err)
	}
	for _, l := range labels {
		if strings.Contains(l.Label, "large") {
			t.Errorf("larger runner label %s", l.Label)
		}
	}
	if m, _ := Multiplier("macos"); m != 10 {
		t.Errorf("macos multiplier = %d", m)
	}
	if m, _ := Multiplier("windows"); m != 2 {
		t.Errorf("windows multiplier = %d", m)
	}
	if m, _ := Multiplier("linux"); m != 1 {
		t.Errorf("linux multiplier = %d", m)
	}
	hosts, _ := RedirectHosts()
	if len(hosts) != 3 || hosts[0] != "api.github.com" {
		t.Errorf("hosts = %v", hosts)
	}
	if _, err := Actions(); err != nil {
		t.Error(err)
	}
	commits, err := SourceCommits()
	if err != nil || len(commits) == 0 {
		t.Errorf("agent pins: %v %v", commits, err)
	}
}

func TestRenderRefuses(t *testing.T) {
	for name, o := range map[string]Options{
		"no source":       {},
		"short commit":    {AgentSource: "source", AgentCommit: "abc"},
		"no pins":         {AgentSource: "release", ReleaseVersion: "1.5.0"},
		"http pin":        {AgentSource: "release", ReleaseVersion: "1.5.0", Pins: []ReleasePin{{RunnerOS: "Linux", RunnerArch: "X64", URL: "http://x.test/a", SHA256: strings.Repeat("a", 64)}}},
		"quote in url":    {AgentSource: "release", ReleaseVersion: "1.5.0", Pins: []ReleasePin{{RunnerOS: "Linux", RunnerArch: "X64", URL: "https://x.test/a'b", SHA256: strings.Repeat("a", 64)}}},
		"unknown version": {Version: 7, AgentSource: "source", AgentCommit: fixtureCommit},
	} {
		if _, err := Render(o); err == nil {
			t.Errorf("%s: rendered", name)
		}
	}
}
