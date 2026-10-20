package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/model"
	nodesdoctor "github.com/nself-org/plugins/free/ci/internal/nodes/doctor"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type nodesReport struct {
	Data struct {
		Checks []doctor.Check `json:"checks"`
	} `json:"data"`
}

// runNodesDoctor runs `ci doctor --nodes --json` against a CI home and returns the exit code, the raw envelope and the report.
func runNodesDoctor(t *testing.T, home string) (int, []byte, nodesReport) {
	t.Helper()
	d := doctor.DefaultDeps()
	d.Getenv = func(k string) string { return map[string]string{"NSELF_CI_HOME": home}[k] }
	d.Capture = func(context.Context, ...string) (string, int, error) { return "", 127, nil }
	var out, errOut bytes.Buffer
	code := doctor.Run([]string{"--json", "--nodes", "--repo", t.TempDir()}, &out, &errOut, d)
	var rep nodesReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("not an envelope: %v\n%s%s", err, out.String(), errOut.String())
	}
	return code, out.Bytes(), rep
}

func nodesChecks(rep nodesReport, id string) []doctor.Check {
	var out []doctor.Check
	for _, c := range rep.Data.Checks {
		if c.ID == id {
			out = append(out, c)
		}
	}
	return out
}

// nodesOnly keeps the checks of the nodes section (the local checks depend on the machine running the test).
func nodesOnly(rep nodesReport) []doctor.Check {
	var out []doctor.Check
	for _, c := range rep.Data.Checks {
		if strings.HasPrefix(c.ID, "CI-DN") {
			out = append(out, c)
		}
	}
	return out
}

// seedBrokenSSHNode registers an ssh node at a closed local port with no pinned host key.
func seedBrokenSSHNode(t *testing.T, home string) {
	t.Helper()
	b, err := os.ReadFile("../internal/model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name, c.Identity.Provider, c.Identity.Transport = "01J0000000000000000000FIX1", "fixture", "ssh", "ssh"
	c.Identity.SSH = &model.CapabilitySSH{Hostname: "127.0.0.1", Port: 1, User: "ci"}
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if _, err = registry.New(st).Register(context.Background(), c, "operator"); err != nil {
		t.Fatal(err)
	}
}

func TestNodesSection(t *testing.T) {
	t.Setenv("NSELF_V15", "1")

	t.Run("the nodes section is registered and no ci nodes doctor command exists", func(t *testing.T) {
		found := false
		for _, s := range doctor.Registered() {
			found = found || s.Name == nodesdoctor.SectionName && s.Selector == doctor.SelectNodes
		}
		if !found {
			t.Fatal("the nodes section is not in the doctor registry (the blank import is missing)")
		}
		for key := range subcommands {
			if strings.Contains(key, "nodes doctor") {
				t.Errorf("subcommand %q exists", key)
			}
		}
		for key := range v15Subcommands {
			if strings.Contains(key, "nodes doctor") {
				t.Errorf("v1.5 subcommand %q exists", key)
			}
		}
	})

	t.Run("doctor --nodes --json lists CI-DN001..CI-DN015 with remediations in a valid v1 envelope", func(t *testing.T) {
		_, raw, rep := runNodesDoctor(t, t.TempDir())
		if err := output.CheckEnvelope(raw); err != nil {
			t.Fatalf("envelope: %v\n%s", err, raw)
		}
		for i := 1; i <= 15; i++ {
			id := fmt.Sprintf("CI-DN%03d", i)
			rows := nodesChecks(rep, id)
			if len(rows) == 0 {
				t.Errorf("%s is not listed", id)
			}
			for _, c := range rows {
				if c.Remediation == "" {
					t.Errorf("%s has no remediation", id)
				}
			}
		}
		if got := len(nodesChecks(rep, "CI-D090")); got != 0 {
			t.Errorf("the 'none registered' placeholder is still listed")
		}
	})

	t.Run("a healthy empty fleet is quiet and a broken node fires its ids", func(t *testing.T) {
		clean := t.TempDir()
		_, _, rep := runNodesDoctor(t, clean)
		if code := doctor.ExitCode(nodesOnly(rep)); code != doctor.ExitClean {
			t.Fatalf("empty fleet gives nodes exit %d: %+v", code, nodesOnly(rep))
		}
		if _, err := os.Stat(filepath.Join(clean, "state.db")); !os.IsNotExist(err) {
			t.Fatal("doctor created a state database")
		}
		broken := t.TempDir()
		seedBrokenSSHNode(t, broken)
		_, _, rep = runNodesDoctor(t, broken)
		if code := doctor.ExitCode(nodesOnly(rep)); code != doctor.ExitFailed {
			t.Fatalf("broken fleet gives nodes exit %d", code)
		}
		for _, id := range []string{"CI-DN001", "CI-DN002"} {
			rows := nodesChecks(rep, id)
			if len(rows) != 1 || rows[0].Status != doctor.Fail || rows[0].Subject != "01J0000000000000000000FIX1" {
				t.Errorf("%s = %+v, want one failure for the node", id, rows)
			}
		}
	})
}
