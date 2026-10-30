package main

// Purpose: tests of manifestgen (P7-CI-62): spec validation names the row, rendering is deterministic and touches only
//          `commands`, and the committed plugin.json equals a fresh generation from the committed spec.
// Constraints: no network; reads ../../surface/commands.yaml and ../../plugin.json.

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const okSpec = `
binary: nself-ci
command: ci
root:
  summary: run
  side_effect: remote
  output: document
  json: envelope
  confirm: {flags: []}
commands:
  - {path: plan, summary: plan, side_effect: read, output: document, json: envelope}
`

func TestParseProblems(t *testing.T) {
	cases := map[string]struct{ old, new, want string }{
		"remote without confirm":   {"path: plan, summary: plan, side_effect: read", "path: plan, summary: plan, side_effect: remote", "plan: a remote command exposed"},
		"interactive not cli-only": {"output: document, json: envelope}\n", "output: interactive, json: envelope}\n", "plan: an interactive command"},
		"bad side effect":          {"side_effect: read", "side_effect: sudo", `plan: side_effect "sudo"`},
		"unknown key":              {"json: envelope}\n", "json: envelope, bogus: 1}\n", "bogus"},
		"confirm on read":          {"json: envelope}\n", "json: envelope, confirm: {flags: []}}\n", "plan: confirm is only for"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			// replace only in the commands row (the last occurrence)
			i := strings.LastIndex(okSpec, c.old)
			if i < 0 {
				t.Fatalf("fixture lacks %q", c.old)
			}
			_, err := Parse([]byte(okSpec[:i] + c.new + okSpec[i+len(c.old):]))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %v, want it to contain %q", err, c.want)
			}
		})
	}
	if _, err := Parse([]byte(okSpec)); err != nil {
		t.Fatal(err)
	}
}

func TestRenderOnlyCommands(t *testing.T) {
	s, err := Parse([]byte(okSpec))
	if err != nil {
		t.Fatal(err)
	}
	in := []byte(`{"name":"ci","commands":{"old":true},"zeta":[1]}`)
	a, err := Render(s, in)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Render(s, in)
	if !bytes.Equal(a, b) || !bytes.HasSuffix(a, []byte("}\n")) {
		t.Fatal("render is not deterministic or lacks the trailing newline")
	}
	var doc map[string]any
	if err := json.Unmarshal(a, &doc); err != nil {
		t.Fatal(err)
	}
	cmds, _ := doc["commands"].(map[string]any)
	if doc["name"] != "ci" || len(doc["zeta"].([]any)) != 1 || cmds == nil || cmds["old"] != nil {
		t.Fatalf("unexpected output: %s", a)
	}
	if _, ok := cmds["confirm"].(map[string]any); !ok || cmds["binary"] != "nself-ci" || cmds["command"] != "ci" {
		t.Fatalf("root fields lost: %s", a)
	}
}

func TestCommittedManifestCurrent(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--check"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
}

func TestCheckDetectsDrift(t *testing.T) {
	dir := t.TempDir()
	m := dir + "/plugin.json"
	if err := os.WriteFile(m, []byte(`{"commands":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := dir + "/s.yaml"
	if err := os.WriteFile(spec, []byte(okSpec), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"--check", "--spec", spec, "--manifest", m}, &out, &errb); code != 1 {
		t.Fatalf("exit %d, want 1: %s", code, errb.String())
	}
	if code := run([]string{"--spec", spec, "--manifest", m}, &out, &errb); code != 0 {
		t.Fatalf("write exit %d: %s", code, errb.String())
	}
	if code := run([]string{"--check", "--spec", spec, "--manifest", m}, &out, &errb); code != 0 {
		t.Fatalf("check after write exit %d", code)
	}
}
