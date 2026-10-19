package lifecycle

import (
	"debug/elf"
	"encoding/json"
	"strings"
	"testing"
)

// run1 executes one subcommand through Run against the kit's Deps.
func (k *kit) run1(sub string, args ...string) (code int, out, errs string) {
	return runWith(k.d, sub, args...)
}

type envelope struct {
	SchemaVersion string          `json:"schema_version"`
	Command       string          `json:"command"`
	Data          json.RawMessage `json:"data"`
	Error         *struct {
		Code     string `json:"code"`
		ExitCode int    `json:"exit_code"`
	} `json:"error"`
}

func decode(t *testing.T, out string) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal([]byte(out), &e); err != nil || e.SchemaVersion == "" || e.Command == "" {
		t.Fatalf("not a v1 envelope: %v %q", err, out)
	}
	return e
}

func TestAddThroughRunEmitsEnvelope(t *testing.T) {
	k := newKit(t)
	bin := elfFixture(t, elf.EM_X86_64)
	code, out, errs := k.run1("add", "ci@cli1", "--host-key", fpA, "--agent-binary", bin, "--json")
	if code != 0 || errs != "" {
		t.Fatalf("code=%d err=%q out=%q", code, errs, out)
	}
	e := decode(t, out)
	if e.Command != "ci nodes add" || e.Error != nil {
		t.Fatalf("envelope = %+v", e)
	}
	var d struct {
		Node struct {
			ID                 string   `json:"id"`
			Accepts            []string `json:"accepts"`
			AuthorizedProjects []string `json:"authorized_projects"`
		} `json:"node"`
		Fingerprints []string `json:"host_key_fingerprints"`
	}
	if err := json.Unmarshal(e.Data, &d); err != nil || d.Node.ID != "cli1" || len(d.Fingerprints) != 1 || d.Fingerprints[0] != fpA {
		t.Fatalf("data = %s (%v)", e.Data, err)
	}
	code, out, _ = k.run1("list", "--json")
	var l struct {
		Nodes []struct {
			ID string `json:"id"`
		} `json:"nodes"`
	}
	if e = decode(t, out); code != 0 || json.Unmarshal(e.Data, &l) != nil || len(l.Nodes) != 1 || l.Nodes[0].ID != "cli1" {
		t.Fatalf("list code=%d out=%s", code, out)
	}
	if code, out, _ = k.run1("show", "cli1", "--json"); code != 0 || decode(t, out).Command != "ci nodes show" {
		t.Fatalf("show: %d %s", code, out)
	}
}

func TestRunExitClassesAndErrorEnvelopes(t *testing.T) {
	k := newKit(t)
	for name, tc := range map[string]struct {
		sub  string
		args []string
		exit int
		code string
	}{
		"no tty, no host key": {"add", []string{"ci@x1", "--json"}, 3, "E655"},
		"unknown node":        {"show", []string{"ghost", "--json"}, 1, "E650"},
		"missing argument":    {"revoke", []string{"--json"}, 1, "E651"},
		"rotate no key":       {"rotate", []string{"ghost", "--json"}, 1, "E650"},
	} {
		code, out, _ := k.run1(tc.sub, tc.args...)
		e := decode(t, out)
		if code != tc.exit || e.Error == nil || e.Error.Code != tc.code || e.Error.ExitCode != tc.exit {
			t.Errorf("%s: code=%d envelope=%s", name, code, out)
		}
	}
	k.addNode("cli2")
	if code, _, _ := k.run1("revoke", "cli2"); code != 0 {
		t.Fatalf("revoke code %d", code)
	}
	if code, out, _ := k.run1("drain", "cli2", "--json"); code != 3 || decode(t, out).Error.Code != "E657" {
		t.Fatalf("drain revoked: %d %s", code, out)
	}
	if code, _, errs := k.run1("add", "ci@x2"); code != 3 || !strings.Contains(errs, "E655") {
		t.Fatalf("human error path: %d %q", code, errs)
	}
	if code, _, _ := k.run1("add", "--bogus"); code != 1 {
		t.Fatalf("bad flag exit = %d", code)
	}
}

func TestHumanOutputAndSecretCanary(t *testing.T) {
	k := newKit(t)
	k.addNode("can1")
	var all strings.Builder
	for _, c := range [][]string{{"list"}, {"show", "can1"}, {"test", "can1"}, {"drain", "can1"}, {"enable", "can1"}, {"revoke", "can1"}} {
		sub := c[0]
		for _, j := range []bool{false, true} {
			args := c[1:]
			if j {
				args = append(append([]string{}, args...), "--json")
			}
			_, out, errs := k.run1(sub, args...)
			all.WriteString(out + errs)
		}
	}
	all.WriteString(k.out.String() + k.auditText())
	for _, s := range secretFixtures {
		if strings.Contains(all.String(), s) {
			t.Fatalf("secret fixture %q reached an output or the audit log", s)
		}
	}
	if strings.Contains(all.String(), "PRIVATE KEY") || strings.Contains(all.String(), "ssh-ed25519 AAAA") {
		t.Fatal("key material in output")
	}
}

func TestSecretsInHostAndErrorsNeverEcho(t *testing.T) {
	k := newKit(t)
	k.failRun["uname"] = errString("ssh: " + secretFixtures[0] + "\nsecond line " + secretFixtures[1])
	_, out, errs := k.run1("add", "ci@leak1", "--host-key", fpA, "--agent-binary", elfFixture(t, elf.EM_X86_64), "--json")
	if strings.Contains(out+errs, secretFixtures[1]) {
		t.Fatalf("second error line leaked: %s", out)
	}
	if k.auditCount("") != 0 {
		t.Fatal("audit row for a failed add")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestFlagSetsAndUsage(t *testing.T) {
	for _, sub := range []string{"add", "list", "show", "test", "drain", "enable", "revoke", "remove", "rotate"} {
		fs := FlagSet(sub)
		if fs.Lookup("json") == nil {
			t.Errorf("%s has no --json", sub)
		}
		if usageLine(sub) == "" {
			t.Errorf("%s has no usage line", sub)
		}
	}
	for _, f := range []string{"accepts", "secret-classes", "privacy-zone", "ownership", "host-key", "agent-binary", "name"} {
		if FlagSet("add").Lookup(f) == nil {
			t.Errorf("add has no --%s", f)
		}
	}
	if FlagSet("rotate").Lookup("host-key") == nil {
		t.Error("rotate has no --host-key")
	}
	if got := splitList(" owner, internal ,,"); len(got) != 2 || got[1] != "internal" {
		t.Errorf("splitList = %v", got)
	}
	for _, c := range []struct {
		code string
		exit int
	}{{"E655", 3}, {"E657", 3}, {"E656", 2}, {"E607", 2}, {"E650", 1}, {"E651", 1}} {
		if exitFor(c.code) != c.exit {
			t.Errorf("exitFor(%s) = %d", c.code, exitFor(c.code))
		}
	}
}

func TestCodeOfAndTypedErrors(t *testing.T) {
	if CodeOf(nil) != "" || CodeOf(errBoom) != "" || CodeOf(errString("E650: node not found")) != "E650" {
		t.Fatal("CodeOf")
	}
	if e := usage("x %d", 1); e.Code != CodeInvalid || e.Error() != "E651: x 1" {
		t.Fatalf("usage error = %v", e)
	}
	if got := oneLine(errString("a\x00b\nsecond")); got != "ab" {
		t.Fatalf("oneLine = %q", got)
	}
}
