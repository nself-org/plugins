package cloudvm_test

// Purpose: TestUserDataClosedList* and FuzzCloudInit: the rendered cloud-init carries exactly the closed
// list of values, turns password and root login off, masks sshd, keeps the pairing code in one 0600 file
// and out of every command line, and no value can add a key, a line or a command.
// Inputs:  canonical values and fuzzed values. Outputs: pass/fail. Constraints: synthetic values only.

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/providers/ephemeral/cloudvm"
)

const canonFP = "SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ"

func canonical(t testing.TB) cloudvm.UserData {
	t.Helper()
	code, err := pairing.GenerateCode()
	if err != nil {
		t.Fatal(err)
	}
	return cloudvm.UserData{EnrollURL: "https://127.0.0.1:3866/v1/enroll", RootFingerprint: canonFP, AgentURL: agentURL, AgentSHA256: agentSHA, PairingCode: code}
}

// parsed is the cloud-config split into the parts the tests look at.
type parsed struct {
	top            map[string]any
	paths          []string
	codeFile       string
	script         string
	runcmd         []any
	codeFileMode   string
	scriptFileMode string
}

func parse(t testing.TB, doc []byte) parsed {
	t.Helper()
	var top map[string]any
	if err := yaml.Unmarshal(doc, &top); err != nil {
		t.Fatalf("the user data is not YAML: %v", err)
	}
	p := parsed{top: top}
	files, _ := top["write_files"].([]any)
	for _, f := range files {
		m := f.(map[string]any)
		p.paths = append(p.paths, m["path"].(string))
		switch m["path"] {
		case "/run/nself-ci/code":
			p.codeFile, p.codeFileMode = m["content"].(string), m["permissions"].(string)
		case "/run/nself-ci/join.sh":
			p.script, p.scriptFileMode = m["content"].(string), m["permissions"].(string)
		}
	}
	p.runcmd, _ = top["runcmd"].([]any)
	return p
}

func TestUserDataClosedListShape(t *testing.T) {
	d := canonical(t)
	doc, err := cloudvm.Render(d)
	if err != nil {
		t.Fatal(err)
	}
	p := parse(t, doc)
	keys := make([]string, 0, len(p.top))
	for k := range p.top {
		keys = append(keys, k)
	}
	if want := []string{"disable_root", "runcmd", "ssh_pwauth", "write_files"}; !sameSet(keys, want) {
		t.Fatalf("top-level keys %v, want %v", keys, want)
	}
	if p.top["ssh_pwauth"] != false || p.top["disable_root"] != true {
		t.Fatalf("ssh_pwauth %v disable_root %v", p.top["ssh_pwauth"], p.top["disable_root"])
	}
	if !reflect.DeepEqual(p.paths, []string{"/run/nself-ci/code", "/run/nself-ci/join.sh"}) {
		t.Fatalf("files %v", p.paths)
	}
	if p.codeFileMode != "0600" || p.codeFile != d.PairingCode+"\n" {
		t.Fatalf("code file mode %q content %q", p.codeFileMode, p.codeFile)
	}
	stop, _ := p.runcmd[0].([]any)
	masked, _ := p.runcmd[1].([]any)
	if len(p.runcmd) != 3 || len(stop) < 4 || stop[0] != "systemctl" || stop[1] != "disable" || stop[2] != "--now" || !containsAll(stop, "ssh.service", "ssh.socket") ||
		len(masked) < 3 || masked[0] != "systemctl" || masked[1] != "mask" || !containsAll(masked, "ssh.service", "ssh.socket") || containsAll(masked, "sshd.service") {
		t.Fatalf("sshd is not stopped and masked first (the sshd.service alias is not named: masking an alias fails on Debian images): %v", p.runcmd)
	}
	if !reflect.DeepEqual(p.runcmd[2], []any{"/bin/sh", "/run/nself-ci/join.sh"}) {
		t.Fatalf("last command %v", p.runcmd[2])
	}
	// exactly the closed list: the code once (its file), every other value in the script
	if n := strings.Count(string(doc), d.PairingCode); n != 1 {
		t.Fatalf("the pairing code appears %d times in the user data", n)
	}
	for _, v := range []string{d.EnrollURL, d.RootFingerprint, d.AgentURL, d.AgentSHA256} {
		if strings.Count(p.script, v) != 1 {
			t.Errorf("value %q appears %d times in the script", v, strings.Count(p.script, v))
		}
	}
	if strings.Contains(p.script, d.PairingCode) || strings.Contains(p.script, "PAIRING") {
		t.Fatal("the pairing code is on a command line")
	}
	if !strings.Contains(p.script, "< /run/nself-ci/code") || !strings.Contains(p.script, "rm -f /run/nself-ci/code") || !strings.Contains(p.script, "sha256sum -c") {
		t.Fatalf("join must read the code from the file, remove it and check the agent sha256:\n%s", p.script)
	}
	for _, want := range []string{"export NSELF_CI_AGENT_HOME=", `export HOME="${HOME:-/root}"`, "--proto-redir '=https'"} {
		if !strings.Contains(p.script, want) {
			t.Fatalf("the join script lacks %q (cloud-init runs without HOME; redirects must stay https)", want)
		}
	}
	if got := strings.Count(p.script, "https://"); got != 2 {
		t.Fatalf("the script reaches %d https URLs, want the agent and the enroll URL only", got)
	}
	if len(cloudvm.Fields) != 5 || reflect.TypeOf(d).NumField() != len(cloudvm.Fields) {
		t.Fatalf("the closed list drifted: %v vs %d fields", cloudvm.Fields, reflect.TypeOf(d).NumField())
	}
}

func TestUserDataClosedListRefusals(t *testing.T) {
	base := canonical(t)
	bad := map[string]func(*cloudvm.UserData){
		"agent url with userinfo":   func(d *cloudvm.UserData) { d.AgentURL = "https://operator@agents.example.test/a" },
		"agent url with a query":    func(d *cloudvm.UserData) { d.AgentURL = agentURL + "?token=x" },
		"agent url with a fragment": func(d *cloudvm.UserData) { d.AgentURL = agentURL + "#x" },
		"agent url over http":       func(d *cloudvm.UserData) { d.AgentURL = "http://agents.example.test/a" },
		"agent url with a quote":    func(d *cloudvm.UserData) { d.AgentURL = "https://agents.example.test/a'b" },
		"enroll url with a newline": func(d *cloudvm.UserData) { d.EnrollURL = "https://h.example.test/x\nruncmd" },
		"enroll url without a host": func(d *cloudvm.UserData) { d.EnrollURL = "https:///x" },
		"fingerprint with a quote":  func(d *cloudvm.UserData) { d.RootFingerprint = "SHA256:" + strings.Repeat("a", 42) + "'" },
		"fingerprint too short":     func(d *cloudvm.UserData) { d.RootFingerprint = "SHA256:abc" },
		"sha256 in upper case":      func(d *cloudvm.UserData) { d.AgentSHA256 = strings.ToUpper(agentSHA) },
		"sha256 too short":          func(d *cloudvm.UserData) { d.AgentSHA256 = agentSHA[:63] },
		"code with a newline":       func(d *cloudvm.UserData) { d.PairingCode = d.PairingCode + "\nruncmd: x" },
		"code of the wrong shape":   func(d *cloudvm.UserData) { d.PairingCode = "short" },
		"empty code":                func(d *cloudvm.UserData) { d.PairingCode = "" },
	}
	for name, mod := range bad {
		t.Run(name, func(t *testing.T) {
			d := base
			mod(&d)
			if doc, err := cloudvm.Render(d); err == nil || cloudvm.ReasonOf(err) != cloudvm.ReasonUserData {
				t.Fatalf("want E698 user_data, got %v (doc %d bytes)", err, len(doc))
			}
		})
	}
}

func containsAll(list []any, want ...string) bool {
	for _, w := range want {
		found := false
		for _, x := range list {
			found = found || x == w
		}
		if !found {
			return false
		}
	}
	return true
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]bool{}
	for _, x := range a {
		m[x] = true
	}
	for _, x := range b {
		if !m[x] {
			return false
		}
	}
	return true
}

// mask replaces each value by a placeholder, longest first.
func mask(s string, d cloudvm.UserData) string {
	for _, r := range [][2]string{{d.EnrollURL, "<ENROLL>"}, {d.AgentURL, "<AGENT>"}, {d.RootFingerprint, "<FP>"}, {d.AgentSHA256, "<SHA>"}, {d.PairingCode, "<CODE>"}} {
		s = strings.ReplaceAll(s, r[0], r[1])
	}
	return s
}

// FuzzCloudInit: whatever the values are, a render that succeeds has the canonical structure: the same
// keys, files, commands and script lines, and the values only in their own places.
func FuzzCloudInit(f *testing.F) {
	d := canonical(f)
	f.Add(d.EnrollURL, d.RootFingerprint, d.AgentURL, d.AgentSHA256, d.PairingCode)
	f.Add("https://h.example.test/x'; rm -rf /; '", canonFP, agentURL, agentSHA, d.PairingCode)
	f.Add("https://h.example.test/x\n  - evil", canonFP, agentURL+"\"", agentSHA, d.PairingCode+"\n")
	f.Add("", "", "", "", "")
	canonDoc, err := cloudvm.Render(d)
	if err != nil {
		f.Fatal(err)
	}
	cp := parse(f, canonDoc)
	wantScript := mask(cp.script, d)
	f.Fuzz(func(t *testing.T, enroll, fp, agent, sha, code string) {
		in := cloudvm.UserData{EnrollURL: enroll, RootFingerprint: fp, AgentURL: agent, AgentSHA256: sha, PairingCode: code}
		doc, err := cloudvm.Render(in)
		if err != nil {
			if cloudvm.CodeOf(err) != "E698" {
				t.Fatalf("a refusal must be E698: %v", err)
			}
			return
		}
		vals := []string{in.EnrollURL, in.RootFingerprint, in.AgentURL, in.AgentSHA256, in.PairingCode}
		for i, a := range vals {
			for j, b := range vals {
				if i != j && strings.Contains(a, b) {
					return // values that contain one another make a textual mask ambiguous
				}
			}
		}
		p := parse(t, doc)
		if !reflect.DeepEqual(p.paths, cp.paths) || !reflect.DeepEqual(p.runcmd, cp.runcmd) || p.top["ssh_pwauth"] != false || p.top["disable_root"] != true || len(p.top) != len(cp.top) {
			t.Fatalf("structure changed for %q", vals)
		}
		if p.codeFile != in.PairingCode+"\n" || p.codeFileMode != "0600" || p.scriptFileMode != "0700" {
			t.Fatalf("code file %q mode %q", p.codeFile, p.codeFileMode)
		}
		if got := mask(p.script, in); got != wantScript {
			t.Fatalf("script changed shape for %q:\n%s", vals, got)
		}
		if strings.Contains(p.script, in.PairingCode) {
			t.Fatal("the code reached the script")
		}
	})
}
