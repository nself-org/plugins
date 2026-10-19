package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

const sealRev = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// seedPassedRun stores a finished run of one plain test job and its evidence,
// under a fresh NSELF_CI_HOME, and returns the home.
func seedPassedRun(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_CI_REPO", home)
	ctx := context.Background()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(st.CreatePipeline(ctx, store.PipelineRow{ID: "run-1", Revision: sealRev, Project: home, Trigger: "local", SourceTrust: "owner",
		PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "in1", SelectionMode: "full", Status: "running"}))
	must(st.CreateJob(ctx, store.JobRow{ID: "run-1.test-1", PipelineID: "run-1", Name: "test", Kind: "test", InputDigest: "i"}))
	must(st.CreateAttempt(ctx, store.AttemptRow{ID: "run-1.test-1.a1", JobID: "run-1.test-1", N: 1}, ""))
	from := model.JobState("queued")
	for _, to := range []model.JobState{"leased", "running", "finalizing", "passed"} {
		_, epoch, err := st.AttemptStateEpoch(ctx, "run-1.test-1.a1")
		must(err)
		if to == "leased" {
			must(storetest.Admit(ctx, st, "run-1.test-1.a1"))
		} else {
			must(st.Transition(ctx, "run-1.test-1.a1", from, to, store.TransitionDetail{Epoch: epoch}))
		}
		from = to
	}
	must(st.FinishPipeline(ctx, "run-1", "passed"))
	body, _ := json.Marshal(model.Evidence{Schema: "ci.evidence/v1", Revision: sealRev, RunID: "run-1", Result: "pass", Checks: []model.Check{{ID: "t", JobID: "test", Kind: "test", Result: "pass"}}})
	must(st.RecordRunEvidence(ctx, "ev-run-1", "run-1", sealRev, "in1", body))
	return home
}

func TestArtifactsReleaseUsageAndRefusals(t *testing.T) {
	seedPassedRun(t)
	for _, args := range [][]string{{}, {"run-1"}, {"--out", "x"}, {"a", "b", "--out", "x"}} {
		if code, _, errs := capture(t, func() int { return artifactsRelease(args) }); code != 1 || !strings.Contains(errs, "usage: nself ci artifacts release") {
			t.Fatalf("%v: code %d %q", args, code, errs)
		}
	}
	// a run with no release record names its recovery command and exports nothing
	out := filepath.Join(t.TempDir(), "out")
	code, _, errs := capture(t, func() int { return artifactsRelease([]string{"run-1", "--out", out}) })
	if code != 2 || !strings.Contains(errs, "E717") || !strings.Contains(errs, "nself ci artifacts sign run-1") {
		t.Fatalf("code %d %q", code, errs)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("a refused export created the destination")
	}
	code, stdout, _ := capture(t, func() int { return artifactsRelease([]string{"run-1", "--out", out, "--json"}) })
	var env struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil || env.OK || env.Error.Code != "E717" || code != 2 {
		t.Fatalf("json %q code %d (%v)", stdout, code, err)
	}
}

func TestArtifactsSignNothingToPublishAndSources(t *testing.T) {
	home := seedPassedRun(t)
	code, _, errs := capture(t, func() int { return artifactsSign([]string{"run-1"}) })
	if code != 1 || !strings.Contains(errs, "no passing release job") {
		t.Fatalf("code %d %q", code, errs)
	}
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.ReleaseFor(context.Background(), "run-1"); ok {
		t.Fatal("a run without a release job got a release record")
	}
	_ = st.Close()
	if code, _, errs = capture(t, func() int { return artifactsSign([]string{}) }); code != 1 || !strings.Contains(errs, "usage: nself ci artifacts sign") {
		t.Fatalf("usage: code %d %q", code, errs)
	}
	// a registered source runs instead of the local signer; two at once are refused
	var ran string
	publish.RegisterSignSource("zz-cmdtest", func(fs *flag.FlagSet) { fs.String("zz-cmdtest", "", "test source") },
		func(_ context.Context, run string, fs *flag.FlagSet) (publish.Release, error) {
			ran = run + ":" + fs.Lookup("zz-cmdtest").Value.String()
			return publish.Release{RunID: run, State: publish.Published, Dir: "/d", KeyID: "k", ManifestSHA256: strings.Repeat("a", 64)}, nil
		})
	code, out, errs := capture(t, func() int { return artifactsSign([]string{"run-9", "--zz-cmdtest", "https://c.example"}) })
	if code != 0 || ran != "run-9:https://c.example" || !strings.Contains(out, "published to /d") {
		t.Fatalf("source: code %d ran %q out %q %q", code, ran, out, errs)
	}
}

func TestReleasePublisherSeamAndLookup(t *testing.T) {
	home := seedPassedRun(t)
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if _, err = cirun.ReleasePublisher(context.Background(), cirun.ReleasePublishRequest{RunID: "run-1", Store: st, Home: home, Root: home}); err != cirun.ErrNothingToPublish {
		t.Fatalf("a run without a release job: %v", err)
	}
	if _, ok, _ := publishedDir(context.Background(), "run-1"); ok {
		t.Fatal("an unpublished run has a release directory")
	}
	if err = os.MkdirAll(publish.Dir(home, "run-2"), 0o700); err != nil {
		t.Fatal(err)
	}
	if dir, ok, _ := publishedDir(context.Background(), "run-2"); !ok || dir != publish.Dir(home, "run-2") {
		t.Fatalf("lookup %q %v", dir, ok)
	}
	for _, bad := range []string{"", ".", ".."} {
		if _, ok, _ := publishedDir(context.Background(), bad); ok {
			t.Fatalf("lookup accepted %q", bad)
		}
	}
	if releaseDirOf == nil {
		t.Fatal("artifacts verify <run> has no lookup")
	}
}
