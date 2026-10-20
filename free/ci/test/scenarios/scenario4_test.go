//go:build scenarios

package scenarios

// Purpose: Epic acceptance 2, scenario 4 (heterogeneous matrix). A project that declares
// linux/amd64, linux/arm64 and darwin/arm64 expands to exactly three entries and each lands on a
// matching runner in the in-process world; with a platform missing a coordinator run waits with
// platform.none_available and a foreground PERSONAL run fails at once with E618.
// Inputs:  the planner, sched.Place and a real dispatcher over a real store.
// Outputs: assertions only.
// Constraints: no Docker, no network, no sleeps; the native legs of ci-sched-scenarios.yml run
//          TestScenario4Native on the three hosted platforms.

import (
	"context"
	"errors"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

var matrixPlatforms = []string{"darwin/arm64", "linux/amd64", "linux/arm64"}

// matrixPlan plans one job over every supported platform; w says which runners exist.
func matrixPlan(t *testing.T, support []string, w plan.World) *plan.Plan {
	t.Helper()
	yes := true
	cfg := model.PipelineConfig{Version: 1, Support: support, Jobs: map[string]model.Job{
		"build": {Kind: "test", Run: []string{"true"}, Required: &yes, Matrix: &model.JobMatrix{Platform: "support"}},
	}}
	p, err := plan.Build(plan.Input{Config: cfg, World: w})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScenario4MatrixExpandsToThreeEntries(t *testing.T) {
	p := matrixPlan(t, matrixPlatforms, plan.World{Platform: "linux/amd64", Remote: true, Runners: []string{"linux/arm64", "darwin/arm64"}})
	var got []string
	for _, j := range p.Jobs {
		if j.State != plan.StatePlanned {
			t.Fatalf("entry %s is %s (%s)", j.ID, j.State, j.Detail)
		}
		got = append(got, j.Platform)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(matrixPlatforms, ",") {
		t.Fatalf("entries %v, want exactly %v", got, matrixPlatforms)
	}
	// a platform outside the declared support never becomes an entry
	yes := true
	cfg := model.PipelineConfig{Version: 1, Support: matrixPlatforms, Jobs: map[string]model.Job{
		"build": {Kind: "test", Run: []string{"true"}, Required: &yes, Matrix: &model.JobMatrix{Platform: []string{"linux/amd64", "windows/amd64"}}},
	}}
	q, err := plan.Build(plan.Input{Config: cfg, World: plan.World{Platform: "linux/amd64", Remote: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Jobs) != 1 || q.Jobs[0].Platform != "linux/amd64" {
		t.Fatalf("an undeclared platform entered the matrix: %+v", q.Jobs)
	}
}

// TestScenario4EachEntryLandsOnAMatchingRunner admits the three entries through the real
// dispatcher and reads the placement the store recorded.
func TestScenario4EachEntryLandsOnAMatchingRunner(t *testing.T) {
	st := openStore(t)
	runners := []sched.Runner{runner("amd", "lan", "linux/amd64", 1), runner("arm", "lan", "linux/arm64", 1), runner("mac", "lan", "darwin/arm64", 1)}
	tr := &transport{}
	d := newDispatcher(t, dispatch.Config{Store: st, World: liveWorld(t, st, nil, runners...), Poll: time.Millisecond, Transport: tr, Clock: newClock().Now})
	want := map[string]string{"linux/amd64": "amd", "linux/arm64": "arm", "darwin/arm64": "mac"}
	for plat, id := range want {
		attempt := queueAttempt(t, st, "pipe", "project", "build-"+strings.ReplaceAll(plat, "/", "-"))
		j := dispatch.Job{AttemptID: attempt, Sched: job(attempt, plat)}
		j.Sched.Pipeline.Support = matrixPlatforms
		a, err := d.Submit(context.Background(), j)
		if err != nil {
			t.Fatalf("%s: %v", plat, err)
		}
		if a.RunnerID != id {
			t.Fatalf("%s landed on %s, want %s", plat, a.RunnerID, id)
		}
		if _, _, _, runnerID, _, ok, err := st.AdmittedIdentity(context.Background(), attempt); err != nil || !ok || runnerID != id {
			t.Fatalf("%s: the store records runner %q (%v, %v)", plat, runnerID, ok, err)
		}
	}
	d.Close()
	if n := len(tr.deliveries()); n != 3 {
		t.Fatalf("%d deliveries, want exactly 3", n)
	}
}

// TestScenario4MissingPlatformWaitsOnACoordinator: with darwin/arm64 absent, the coordinator's
// registry is open (a node can still enrol), so the entry waits with platform.none_available.
func TestScenario4MissingPlatformWaitsOnACoordinator(t *testing.T) {
	w := sched.World{Runners: []sched.Runner{runner("amd", "lan", "linux/amd64", 1), runner("arm", "lan", "linux/arm64", 1)}, Policy: sched.Policy{Preset: "balanced"},
		Costs: map[string]sched.Cost{}, Previous: map[string]string{}, OpenRegistry: true}
	jobs := []sched.Job{job("amd", "linux/amd64"), job("arm", "linux/arm64"), job("mac", "darwin/arm64")}
	for i := range jobs {
		jobs[i].Pipeline.Support = matrixPlatforms
	}
	res := sched.Place(w, jobs)
	if len(res.Placements) != 2 {
		t.Fatalf("placements %+v, want the two present platforms only", res.Placements)
	}
	var mac sched.Explanation
	for _, x := range res.Explanations {
		if x.JobKey == "mac" {
			mac = x
		}
	}
	if mac.Status != "waiting" || !hasCode(mac.Reasons, string(model.PlatformNoneAvailable)) {
		t.Fatalf("mac entry: status %q reasons %v", mac.Status, codes(mac.Reasons))
	}
}

// TestScenario4MissingPlatformFailsAFreshForegroundRun: a foreground PERSONAL run with a closed
// registry fails at once with E618, naming the runners it tried.
func TestScenario4MissingPlatformFailsAFreshForegroundRun(t *testing.T) {
	st := openStore(t)
	d := newDispatcher(t, dispatch.Config{Store: st, Poll: time.Millisecond, Clock: newClock().Now,
		World: liveWorld(t, st, nil, runner("local", "local", "linux/amd64", 2), runner("arm", "lan", "linux/arm64", 1))})
	attempt := queueAttempt(t, st, "pipe", "project", "mac")
	j := dispatch.Job{AttemptID: attempt, Sched: job(attempt, "darwin/arm64"), Foreground: true}
	j.Sched.Pipeline.Support = matrixPlatforms
	j.Sched.Pipeline.Foreground = true
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := d.Submit(ctx, j)
	var de *dispatch.Error
	if !errors.As(err, &de) || de.Code != "E618" {
		t.Fatalf("want E618, got %v", err)
	}
	if !strings.Contains(de.Message, string(model.PlatformMismatch)) || strings.Contains(de.Message, "\n") {
		t.Fatalf("E618 must be one line naming the platform mismatch: %q", de.Message)
	}
	if reason, _, ok, _ := st.Refusal(ctx, attempt); ok {
		t.Fatalf("a foreground refusal must not queue the job (recorded %q)", reason)
	}
}

// TestScenario4Native is the native leg: on each hosted platform the local runner is described by
// world.BuildForLocal from this machine's own platform (the path `ci plan` and a run take through
// planview.LocalCapability), and the entry for that platform places on it while the others stay off it.
// ci-sched-scenarios.yml runs it on ubuntu-24.04, ubuntu-24.04-arm and macos-15. On a host outside the
// matrix (darwin/amd64, say) it fails on purpose, so a leg can never pass without placing its own platform.
func TestScenario4Native(t *testing.T) {
	host := runtime.GOOS + "/" + runtime.GOARCH
	known := false
	for _, p := range matrixPlatforms {
		known = known || p == host
	}
	if !known {
		t.Fatalf("host platform %s is outside the scenario matrix %v", host, matrixPlatforms)
	}
	st := openStore(t)
	pol := func(context.Context, string) (sched.Policy, error) { return sched.Policy{Preset: "balanced"}, nil }
	w, _, err := world.BuildForLocal(context.Background(), world.Deps{Store: st, Policy: pol, Clock: newClock().Now},
		world.LocalInput{Project: "project", Platform: host, Capacity: ciexec.Capacity{CPUs: 8, MemoryMB: 8192}, DockerUnknown: true, Now: newClock().Now()}, sched.Pipeline{ID: "pipe", Support: matrixPlatforms})
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Runners) != 1 || w.Runners[0].ID != "local" {
		t.Fatalf("the native world must hold exactly the local runner: %+v", w.Runners)
	}
	jobs := make([]sched.Job, 0, len(matrixPlatforms))
	for _, p := range matrixPlatforms {
		j := job("build-"+strings.ReplaceAll(p, "/", "-"), p)
		j.Pipeline.Support = matrixPlatforms
		jobs = append(jobs, j)
	}
	res := sched.Place(w, jobs)
	own := "build-" + strings.ReplaceAll(host, "/", "-")
	if res.ErrorCode != "" || len(res.Placements) != 1 || res.Placements[0].RunnerID != "local" || res.Placements[0].JobKey != own {
		t.Fatalf("native placement on %s: %+v", host, res)
	}
	for _, x := range res.Explanations {
		if x.JobKey != own && x.Status == "placed" {
			t.Fatalf("%s placed on a host of another platform", x.JobKey)
		}
	}
}
