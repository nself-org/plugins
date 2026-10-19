//go:build integration

package bootstrap

// Purpose: TestBootstrapDistros and TestBootstrapSlots, the Linux evidence of P7-NODE-06: the real ssh, scp,
// sudo and agent against Docker sshd nodes of three distros. Acceptance 4 (users, one job-shim rule per
// slot, no docker group, key and job-root isolation) and acceptance 8 (idempotent slot changes).
// Inputs:  INTEGRATION=1. Constraints: synthetic secrets only.

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
)

const shimRule = "NOPASSWD: " + AgentPath + " job-shim"

func (r *rig) lookup(ctx context.Context, want string) Family {
	r.t.Helper()
	f, keys, ok, err := Detect(ctx, r.run)
	if err != nil || !ok || f.Name() != want {
		r.t.Fatalf("Detect = %v %v %v %v, want family %s", f, keys, ok, err, want)
	}
	return f
}

// layout asserts the system layout for slots job users.
func (r *rig) layout(slots int) {
	r.t.Helper()
	for _, u := range append([]string{AgentUser}, jobUsers(slots)...) {
		r.root("id -u " + u)
		if groups := r.root("id -nG " + u); strings.Contains(" "+groups+" ", " docker ") {
			r.t.Errorf("%s is in the docker group: %s", u, groups)
		}
	}
	if members := r.root("getent group docker | cut -d: -f4 || true"); strings.Contains(members, "nself-ci") {
		r.t.Errorf("a system user is a member of the docker group: %s", members)
	}
	rules := 0
	for _, line := range strings.Split(r.root("sudo -l -U "+AgentUser), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "(") {
			continue
		}
		rules++
		ok := false
		for n := 1; n <= slots; n++ {
			ok = ok || line == "("+JobUser(n)+") "+shimRule
		}
		if !ok {
			r.t.Errorf("unexpected sudo rule for %s: %q", AgentUser, line)
		}
	}
	if rules != slots {
		r.t.Errorf("sudo -l -U %s lists %d rules, want %d", AgentUser, rules, slots)
	}
	r.root("visudo -c >/dev/null")
	for path, want := range map[string]string{AgentDir: "755 root", AgentDir + "/bin": "755 root", AgentPath: "755 root", StateDir: "700 nself-ci", WorkDir: "711 nself-ci", JobsDir: "711 root"} {
		if got := r.root("stat -c '%a %U' " + path); got != want {
			r.t.Errorf("%s is %q, want %q", path, got, want)
		}
	}
	for n := 1; n <= slots; n++ {
		if got := r.root("stat -c '%a %U' " + JobRoot(n)); got != "700 "+JobUser(n) {
			r.t.Errorf("job root %d is %q", n, got)
		}
	}
}

func jobUsers(slots int) []string {
	var out []string
	for n := 1; n <= slots; n++ {
		out = append(out, JobUser(n))
	}
	return out
}

// isolation asserts the node key and age identity are unreadable by a job user and job roots by each other.
func (r *rig) isolation() {
	r.t.Helper()
	// The agent creates its keys on first start; `serve --stdio` on a closed stdin creates them and exits.
	r.try("sudo -u " + AgentUser + " env NSELF_CI_AGENT_HOME=" + StateDir + " NSELF_CI_AGENT_WORK=" + WorkDir + " timeout 20 " + AgentPath + " serve --stdio </dev/null")
	files := strings.Fields(r.root("ls " + StateDir))
	if len(files) == 0 {
		r.t.Fatalf("the agent created no key material in %s", StateDir)
	}
	for _, f := range files {
		path := StateDir + "/" + f
		mode := strings.Fields(r.root("stat -c '%a %U' " + path))
		if len(mode) != 2 || mode[1] != AgentUser || len(mode[0]) != 3 || mode[0][1:] != "00" {
			r.t.Errorf("%s is %v: not private to the agent user", path, mode)
		}
		for _, u := range jobUsers(2) {
			if out, rc := r.try("sudo -u " + u + " cat " + path); rc == "0" || !strings.Contains(out, "Permission denied") {
				r.t.Errorf("%s read %s: rc=%s %q (want EACCES)", u, path, rc, out)
			}
		}
	}
	r.root("sudo -u " + JobUser(2) + " sh -c 'echo job2-secret > " + JobRoot(2) + "/file'")
	if out, rc := r.try("sudo -u " + JobUser(1) + " cat " + JobRoot(2) + "/file"); rc == "0" || !strings.Contains(out, "Permission denied") {
		r.t.Errorf("job-1 read job-2's job root: rc=%s %q", rc, out)
	}
	if out, rc := r.try("sudo -u " + JobUser(1) + " ls " + JobRoot(2)); rc == "0" || !strings.Contains(out, "Permission denied") {
		r.t.Errorf("job-1 listed job-2's job root: rc=%s %q", rc, out)
	}
}

// shim asserts the sudo rule starts the job shim as the job user and as nothing else.
func (r *rig) shim() {
	r.t.Helper()
	req := `printf '{"argv":["id","-un"]}\n' | sudo -u ` + AgentUser + ` sudo -n -u ` + JobUser(1) + ` ` + AgentPath + ` job-shim`
	if out, rc := r.try(req); rc != "0" || out != JobUser(1) {
		r.t.Errorf("job-shim as job-1: rc=%s %q", rc, out)
	}
	for _, denied := range []string{
		"sudo -u " + AgentUser + " sudo -n -u " + JobUser(1) + " /bin/true",
		"sudo -u " + AgentUser + " sudo -n -u root /bin/true",
		"sudo -u " + AgentUser + " sudo -n -u " + JobUser(1) + " " + AgentPath + " version",
		"sudo -u " + AgentUser + " sudo -n -u " + JobUser(9) + " " + AgentPath + " job-shim",
		"sudo -u " + AgentUser + " " + AgentPath + " job-shim </dev/null",
	} {
		if out, rc := r.try(denied); rc == "0" {
			r.t.Errorf("the agent user was allowed: %s\n%s", denied, out)
		}
	}
}

func TestBootstrapDistros(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1")
	}
	for _, d := range distros {
		t.Run(d.name, func(t *testing.T) {
			ctx := context.Background()
			r := newRig(t, d)
			fam := r.lookup(ctx, d.family)
			if err := fam.Install(ctx, r.target, r.options(2)); err != nil {
				t.Fatal(err)
			}
			r.layout(2)
			r.isolation()
			r.shim()
			if out := r.root("sudo -u " + JobUser(1) + " " + AgentPath + " version"); !strings.HasPrefix(out, "nself-ci-agent ") {
				t.Errorf("a job user cannot run the agent binary: %q", out)
			}
			if d.family == "apk" {
				if _, rc := r.try("test -e " + UnitPath); rc == "0" {
					t.Error("alpine has no systemd; no unit expected")
				}
			} else if got := r.root("cat " + UnitPath); got+"\n" != Unit() {
				t.Errorf("unit differs:\n%s", got)
			}
			if err := fam.Install(ctx, r.target, r.options(2)); err != nil { // idempotent
				t.Fatalf("second install: %v", err)
			}
			r.layout(2)
			if err := fam.Uninstall(ctx, Options{Target: r.target, Run: r.run}); err != nil {
				t.Fatal(err)
			}
			for _, gone := range []string{AgentDir, StateDir, JobsDir, SudoersFile(1)} {
				if _, rc := r.try("test -e " + gone); rc == "0" {
					t.Errorf("%s survived uninstall", gone)
				}
			}
			if _, rc := r.try("id -u " + AgentUser); rc == "0" {
				t.Error("the agent user survived uninstall")
			}
		})
	}
}

func TestBootstrapSlots(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1")
	}
	ctx := context.Background()
	r := newRig(t, distros[0])
	fam := r.lookup(ctx, "apt")
	install := func(slots int) {
		t.Helper()
		if err := fam.Install(ctx, r.target, r.options(slots)); err != nil {
			t.Fatalf("install with %d slots: %v", slots, err)
		}
	}
	uid := func(n int) string { return r.root("id -u " + JobUser(n)) }
	exists := func(n int) bool { _, rc := r.try("id -u " + JobUser(n)); return rc == "0" }
	rule := func(n int) bool { _, rc := r.try("test -f " + SudoersFile(n)); return rc == "0" }

	install(2)
	r.layout(2)
	u1, u2 := uid(1), uid(2)
	install(3) // more slots: only the missing user and rule are added
	r.layout(3)
	if uid(1) != u1 || uid(2) != u2 {
		t.Fatalf("existing users changed: %s %s -> %s %s", u1, u2, uid(1), uid(2))
	}
	r.root("sudo -u " + JobUser(3) + " sh -c 'echo keep > " + JobRoot(3) + "/keep'")
	install(2) // fewer slots: job-3 owns a file, so the user stays but its rule goes
	if !exists(3) || rule(3) || !rule(1) || !rule(2) {
		t.Fatalf("after 3->2 with a file: user3=%v rule3=%v rule1=%v rule2=%v", exists(3), rule(3), rule(1), rule(2))
	}
	if rules := strings.Count(r.root("sudo -l -U "+AgentUser), "job-shim"); rules != 2 {
		t.Fatalf("sudo -l lists %d job-shim rules, want 2", rules)
	}
	r.root("rm -f " + JobRoot(3) + "/keep")
	install(1) // job-2 and job-3 own no file now: both go
	for n := 2; n <= 3; n++ {
		if exists(n) || rule(n) {
			t.Errorf("slot %d survived 2->1 with no files: user=%v rule=%v", n, exists(n), rule(n))
		}
	}
	if !exists(1) || !rule(1) || uid(1) != u1 {
		t.Fatal("slot 1 changed")
	}
	r.layout(1)
	t.Log("slot users after the run: " + strconv.Itoa(strings.Count(r.root("getent passwd | cut -d: -f1"), "nself-ci-job")))
}
