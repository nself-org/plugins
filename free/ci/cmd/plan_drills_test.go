package main

// Purpose: regression tests for D-1046: `ci plan` and `ci explain` expand the
// selected pipeline's drills_dir the way a real run does, so a discovered
// drill shows up as a planned job and explain finds it.
// Constraints: a real git repository built from the drill package fixture.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/planview"
)

// drillRepo copies the drill fixture (executable bits kept) into a git repo.
func drillRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	t.Setenv("NSELF_CI_HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv("NSELF_CI_REPO", "")
	src := filepath.Join("..", "internal", "drill", "testdata", "repo")
	if err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		info, err := d.Info()
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		dst := filepath.Join(repo, rel)
		if err = os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, b, info.Mode().Perm())
	}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.email=a@b", "-c", "user.name=t", "commit", "-qm", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return repo
}

func TestPlanListsDiscoveredDrillJobs(t *testing.T) {
	repo := drillRepo(t)
	resetPlanFlags()
	code, out, errs := capture(t, func() int { return runPlan([]string{"--json", "--pipeline", "drills", "--repo", repo}) })
	if code != 0 {
		t.Fatalf("plan exit %d: %s %s", code, out, errs)
	}
	var env struct {
		Data struct {
			Jobs []struct {
				ID string `json:"id"`
			} `json:"jobs"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("envelope: %v\n%s", err, out)
	}
	got := map[string]bool{}
	for _, j := range env.Data.Jobs {
		got[j.ID] = true
	}
	for _, want := range []string{"drill-pass", "drill-fail-nested", "drill-invalid"} {
		if !got[want] {
			t.Errorf("plan --pipeline drills lacks job %q: %v", want, got)
		}
	}
}

func TestExplainFindsDiscoveredDrillJob(t *testing.T) {
	repo := drillRepo(t)
	resetPlanFlags()
	code, out, errs := capture(t, func() int {
		return runExplain([]string{"drill-pass", "--json", "--pipeline", "drills", "--repo", repo})
	})
	if code != 0 {
		t.Fatalf("explain exit %d: %s %s", code, out, errs)
	}
	d, errCode := explainEnv(t, out)
	if errCode != "" || d.Source != planview.SourcePlan || len(d.Jobs) != 1 || d.Jobs[0].ID != "drill-pass" {
		t.Fatalf("drill-pass must be explained from the plan: code=%q %+v", errCode, d)
	}
}
