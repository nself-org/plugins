package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// isolationJSON runs the registered section the way `ci doctor` does and returns its checks as decoded JSON.
func isolationJSON(t *testing.T, home, goos string) map[string]map[string]any {
	t.Helper()
	old := isolationGOOS
	isolationGOOS = goos
	t.Cleanup(func() { isolationGOOS = old })
	env := doctor.Env{Root: t.TempDir(), Getenv: func(k string) string {
		if k == "NSELF_CI_HOME" {
			return home
		}
		return ""
	}}
	raw, err := json.Marshal(doctor.RunSections(context.Background(), env, nil, doctor.SectionBudget))
	if err != nil {
		t.Fatal(err)
	}
	var list []map[string]any
	if err = json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]any{}
	for _, c := range list {
		if id, _ := c["id"].(string); strings.HasPrefix(id, "isolation.") {
			out[id] = c
		}
	}
	return out
}

// seedDrillEvidence stores one run document whose drill check lists passed.
func seedDrillEvidence(t *testing.T, home, passed, platform string) {
	t.Helper()
	st, err := cirun.OpenStore(filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	if err = st.CreatePipeline(ctx, store.PipelineRow{ID: "p", Revision: "0123456789abcdef", Project: "x", Trigger: "local", SourceTrust: "owner",
		PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	excerpt, _ := json.Marshal("nested evidence\n" + passed)
	body := `{"source_trust":"owner","revision":"0123456789abcdef","runner":{"platform":"` + platform + `"},"timings":{"finished_at":"2026-10-03T03:00:00Z"},` +
		`"checks":[{"job_id":"drill-isolation-levels","kind":"drill","excerpt":` + string(excerpt) + `}]}`
	if err = st.RecordRunEvidence(ctx, "e", "p", "0123456789abcdef", "i", []byte(body)); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorIsolationSection(t *testing.T) {
	registered := false
	for _, s := range doctor.Registered() {
		registered = registered || s.Name == "isolation" && s.Selector == doctor.SelectAlways
	}
	if !registered {
		t.Fatal("the isolation section is not in the doctor registry")
	}
	home := t.TempDir()
	seedDrillEvidence(t, home, "passed: isolation.container isolation.sandboxed-container isolation.network", "linux/amd64")
	checks := isolationJSON(t, home, "linux")
	for _, id := range []string{"isolation.container", "isolation.sandboxed-container", "isolation.network", "isolation.hygiene", "isolation.vm", "isolation.ephemeral-vm"} {
		c, ok := checks[id]
		if !ok {
			t.Fatalf("no check %s in %v", id, checks)
		}
		if _, has := c["verified_at"]; !has || c["status"] == nil {
			t.Errorf("%s lacks status or verified_at: %v", id, c)
		}
	}
	for _, id := range []string{"isolation.container", "isolation.sandboxed-container", "isolation.network"} {
		if checks[id]["verified_at"] != "2026-10-03T03:00:00Z" || checks[id]["message"] != "verified 2026-10-03 on linux/amd64 (revision 0123456)" {
			t.Errorf("%s: %v", id, checks[id])
		}
	}
	if checks["isolation.hygiene"]["verified_at"] != nil || checks["isolation.hygiene"]["message"] != "unverified since never" {
		t.Errorf("hygiene never passed: %v", checks["isolation.hygiene"])
	}
	for _, id := range []string{"isolation.vm", "isolation.ephemeral-vm"} {
		if checks[id]["verified_at"] != nil || checks[id]["message"] != "unavailable (no KVM host, D-0120)" {
			t.Errorf("%s on Linux: %v", id, checks[id])
		}
	}
	if mac := isolationJSON(t, home, "darwin"); mac["isolation.ephemeral-vm"] != nil || mac["isolation.vm"]["message"] != "unverified since never" {
		t.Errorf("darwin: %v", mac)
	}
}

// A level is verified only by a stored passing check: nothing else, and no failure of the read, may say verified.
func TestDoctorIsolationNever(t *testing.T) {
	never := func(t *testing.T, name string, checks map[string]map[string]any) {
		t.Helper()
		if len(checks) == 0 {
			t.Errorf("%s: no isolation checks", name)
		}
		for id, c := range checks {
			if id == "isolation.record" {
				continue // the warning about an unreadable store carries no date
			}
			if c["verified_at"] != nil || strings.HasPrefix(c["message"].(string), "verified") {
				t.Errorf("%s: %s reads verified: %v", name, id, c)
			}
		}
	}
	missing := filepath.Join(t.TempDir(), "missing")
	never(t, "no store", isolationJSON(t, missing, "linux"))
	if _, err := os.Stat(filepath.Join(missing, "state.db")); err == nil {
		t.Error("the section must not create a store")
	}

	failed := t.TempDir()
	seedDrillEvidence(t, failed, "nested evidence: fail\n  fail isolation.container: broke", "linux/amd64")
	never(t, "only a failed check", isolationJSON(t, failed, "linux"))

	broken := t.TempDir()
	if err := os.WriteFile(filepath.Join(broken, "state.db"), []byte("this is not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	checks := isolationJSON(t, broken, "linux")
	never(t, "unreadable store", checks)
	if w := checks["isolation.record"]; w == nil || w["status"] != "warn" {
		t.Errorf("an unreadable store must warn: %v", w)
	}
}
