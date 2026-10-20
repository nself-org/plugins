//go:build security

package redteam

// Purpose: TM-29, TM-33, TM-35 red team of `nself ci doctor --providers` (CI-DP001 to CI-DP014): a check
// that failed to run must not read as a pass, a hostile checkout must not turn a check green, and the
// state database that holds the provider policy must not be left readable by others.
// Inputs:  the nself-ci binary (see f_attack_helpers_test.go), a corrupt or permissive store, a planted
//          malformed .nself/ci.yaml.
// Outputs: pass = the control held; fail = the attack worked.
// Constraints: local fixtures only.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fMsg(c map[string]any) string { s, _ := c["message"].(string); return s }

// TestF12CorruptStoreIsNotGreen (TM-33, TM-35): when the state database cannot be opened, the provider
// section cannot know the health of the provider APIs or the stored authorization; it must not report
// "API: up" and an empty authorization list as passing checks.
func TestF12CorruptStoreIsNotGreen(t *testing.T) {
	if !openFinding(t, "P05") {
		return
	}
	b := fNew(t)
	if _, errOut, exit := b.run(nil, "providers", "authorize", "github-hosted", "--project", b.repo); exit != 0 {
		t.Fatalf("control: %s", errOut)
	}
	if got := b.doctor(nil)["CI-DP014"]; !strings.Contains(fMsg(got), "github-hosted") {
		t.Fatalf("control: the doctor does not show the stored authorization: %v", got)
	}
	if err := os.WriteFile(filepath.Join(b.home, "state.db"), []byte("this is not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	checks := b.doctor(nil)
	for _, id := range []string{"CI-DP011", "CI-DP014"} {
		c := checks[id]
		if c["status"] == "pass" {
			t.Errorf("%s passes with an unreadable store: %q", id, fMsg(c))
		}
	}
}

// TestF13MalformedPolicyIsNotGreen (TM-35): a checkout with an unparseable .nself/ci.yaml makes the
// policy unresolvable; CI-DP014 (hosted authorization) must then not pass, and the defaults it falls back
// to must not be reported as the operator's authorization.
func TestF13MalformedPolicyIsNotGreen(t *testing.T) {
	if !openFinding(t, "P06") {
		return
	}
	b := fNew(t)
	if _, errOut, exit := b.run(nil, "providers", "authorize", "github-hosted", "--project", b.repo); exit != 0 {
		t.Fatalf("control: %s", errOut)
	}
	if err := os.MkdirAll(filepath.Join(b.repo, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.repo, ".nself", "ci.yaml"), []byte("version: 1\npolicy:\n  trust:\n    nonsense: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checks := b.doctor(nil)
	if checks["CI-DP005"]["status"] == "pass" {
		t.Fatalf("control: the policy check should not pass: %v", checks["CI-DP005"])
	}
	if c := checks["CI-DP014"]; c["status"] == "pass" {
		t.Errorf("CI-DP014 passes although the policy could not be resolved: %q", fMsg(c))
	}
}

// TestF14StoreModes (TM-29): the state database (policy rows, audit, facts) is created owner-only and a
// directory or file that is group or world accessible is tightened or reported by the doctor.
func TestF14StoreModes(t *testing.T) {
	b := fNew(t)
	if _, errOut, exit := b.run(nil, "providers", "authorize", "gitlab", "--project", b.repo); exit != 0 {
		t.Fatalf("control: %s", errOut)
	}
	db := filepath.Join(b.home, "state.db")
	mode := func(p string) os.FileMode {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		return fi.Mode().Perm()
	}
	if m := mode(db); m&0o077 != 0 {
		t.Errorf("a fresh state.db has mode %o", m)
	}
	if m := mode(b.home); m&0o077 != 0 {
		t.Errorf("a fresh CI home has mode %o", m)
	}
	if err := os.Chmod(db, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(b.home, 0o777); err != nil {
		t.Fatal(err)
	}
	checks := b.doctor(nil)
	b.run(nil, "providers", "list")
	tightened := mode(db)&0o077 == 0 && mode(b.home)&0o077 == 0
	warned := false
	for _, c := range checks {
		m := strings.ToLower(fMsg(c))
		warned = warned || c["status"] != "pass" && (strings.Contains(m, "mode") || strings.Contains(m, "permission") || strings.Contains(m, "world"))
	}
	if !tightened && !warned {
		t.Errorf("a world-writable state.db (%o) in a world-writable home (%o) is neither tightened nor reported", mode(db), mode(b.home))
	}
}
