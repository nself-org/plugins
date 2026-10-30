package stats

// Purpose: P7-SCHED-20 acceptance 9, the doctor half: `ci doctor --json` run end to end (doctor.Run, the
// code behind the registered command) on a host whose only problems are the fixture store's stale lease and
// two-hour-old outbox entry reports the health section with CI-STAT warnings and exits 12.
// Inputs:  the 500-attempt fixture store; a fake host where every tool and the status poster are healthy.
// Outputs: pass or fail.
// Constraints: the host is faked through doctor.Deps, so no check outside the health section may warn or fail;
//          the section registers from init in this package, which this test binary links.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// healthyHost answers every probe the doctor makes as a working machine would.
func healthyHost(_ context.Context, argv ...string) (string, int, error) {
	switch {
	case argv[0] == "go":
		return "go version go1.24.0 darwin/arm64\n", 0, nil
	case argv[0] == "docker":
		return "27.1.0\n", 0, nil
	case argv[0] == "gh":
		return "ok\n", 0, nil
	case strings.Contains(strings.Join(argv, " "), "remote get-url"):
		return "git@github.com:example/demo.git\n", 0, nil
	case strings.Contains(strings.Join(argv, " "), "check-ignore"):
		return "", 0, nil
	}
	return "", 0, nil
}

func TestDoctorJSONHealthWarningsExitTwelve(t *testing.T) {
	st := buildFixture(t, 40)
	rows, err := st.StatsRead(context.Background(), store.StatsQuery{Now: time.Now(), Since: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Dir(rows.Path)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.test/demo\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := doctor.Deps{
		Getenv:    func(k string) string { return map[string]string{"NSELF_CI_HOME": home}[k] },
		Getwd:     func() (string, error) { return "", errors.New("no cwd") },
		UserHome:  func() (string, error) { return home, nil },
		Capture:   healthyHost,
		OpenStore: doctor.DefaultDeps().OpenStore,
	}
	var out, errb bytes.Buffer
	code := doctor.Run([]string{"--json", "--repo", repo}, &out, &errb, deps)
	if code != doctor.ExitWarnings {
		t.Fatalf("exit %d, want %d (warnings only): stdout %s stderr %s", code, doctor.ExitWarnings, out.String(), errb.String())
	}
	var env struct {
		Data struct {
			Checks []doctor.Check `json:"checks"`
		} `json:"data"`
		Meta struct {
			Warnings []string `json:"warnings"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("the report is not one JSON document: %v\n%s", err, out.String())
	}
	got := statuses(env.Data.Checks)
	for _, id := range []string{IDStaleLeases, IDOutbox} {
		if got[id] != doctor.Warn {
			t.Errorf("%s = %q in the doctor report, want warn (all: %v)", id, got[id], got)
		}
	}
	for _, c := range env.Data.Checks {
		if c.Status == doctor.Fail {
			t.Errorf("%s failed: %s", c.ID, c.Message)
		}
		if c.Status == doctor.Warn && !strings.HasPrefix(c.ID, "CI-STAT-") {
			t.Errorf("%s warned: %s; only the health section may warn on this host", c.ID, c.Message)
		}
		if c.Status == doctor.Warn && c.Remediation == "" {
			t.Errorf("%s warns without a remediation", c.ID)
		}
	}
	listed := 0
	for _, w := range env.Meta.Warnings {
		if strings.HasPrefix(w, IDStaleLeases+":") || strings.HasPrefix(w, IDOutbox+":") {
			listed++
		}
	}
	if listed != 2 {
		t.Errorf("the envelope lists %d of the two health warnings: %v", listed, env.Meta.Warnings)
	}
}
