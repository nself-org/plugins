//go:build integration

package android

// Purpose: the Android lane end to end with a fake gradle wrapper (P7-CACHE-15
// acceptance): the required job runs, then the release job on the operator's
// own node; the release is published and verifies; the planted keystore values
// reach no sink; the same job on another node is refused the secrets and never
// runs.
// Constraints: INTEGRATION=1; a missing prerequisite is t.Fatal.

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TestAndroidReleaseFakeGradle: required jobs, then the release job; published; verified; no secret in any sink.
func TestAndroidReleaseFakeGradle(t *testing.T) {
	needIntegration(t)
	p := newRig(t)
	res, st, out := p.run(personal())
	if res.Exit != 0 || res.Summary.Result != "pass" {
		t.Fatalf("exit %d result %s notes %v", res.Exit, res.Summary.Result, res.Summary.Notes)
	}
	if _, err := os.Stat(p.marker); err != nil {
		t.Fatalf("gradle never ran: %v", err)
	}
	jobs, err := st.PipelineJobs(context.Background(), res.RunID)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("jobs %+v %v", jobs, err)
	}
	for _, j := range jobs {
		if j.Status != "passed" {
			t.Errorf("job %s is %s", j.Name, j.Status)
		}
	}
	if line := logLine(t, p.home, "signing alias="); line == "" || strings.Count(line, "REDACTED") < 3 {
		t.Fatalf("the job log shows %q: the three planted values must be redacted", line)
	}
	row, ok, err := st.ReleaseFor(context.Background(), res.RunID)
	if err != nil || !ok || row.State != store.ReleasePublished {
		t.Fatalf("release record %+v %v %v", row, ok, err)
	}
	d := p.deps(st)
	exp, err := publish.Export(context.Background(), d, res.RunID, filepath.Join(t.TempDir(), "export"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"release-manifest.dsse.json", "evidence.json", "provenance.intoto.dsse.json", "SHA256SUMS",
		"artifacts/android/app-release.apk", "sbom/android/app-release.apk.cdx.json", "sbom/android/app-release.apk.spdx.json"} {
		if _, err := os.Stat(filepath.Join(exp.Dir, want)); err != nil {
			t.Errorf("the release lacks %s", want)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(exp.Dir, "artifacts/android/app-release.apk")); string(b) != "fake-apk-bytes" {
		t.Fatalf("apk bytes %q", b)
	}
	info, err := d.Signer.PublicKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v := p.verifyDir(exp.Dir, info); !v.Verified || v.Exit != 0 {
		t.Fatalf("verify: %+v", v)
	}
	p.cliVerify(exp.Dir, info)
	var ev model.Evidence
	raw, _ := os.ReadFile(filepath.Join(exp.Dir, "evidence.json"))
	if err = json.Unmarshal(raw, &ev); err != nil || evidence.ClassOf(ev) != evidence.ClassRelease || ev.Result != "pass" {
		t.Fatalf("evidence class %v result %s (%v)", evidence.ClassOf(ev), ev.Result, err)
	}
	if hits := sinkHits(t, p.planted(), map[string]string{"run output": out, "summary": strings.Join(res.Summary.Notes, "\n")}, p.home, exp.Dir, p.repo); len(hits) != 0 {
		t.Fatalf("planted secret values reached sinks: %v", hits)
	}
	if err := exec.Command("git", "-C", p.repo, "diff", "--quiet").Run(); err != nil {
		t.Fatal("the run changed the repository")
	}
}

// TestAndroidOtherNodeRefused: the lane's secrets exist only on the operator's own node. An agent node that is
// not secrets-eligible (the sim node) is refused with E677 and the job never runs; an eligible agent node is
// refused the compat source (only the personal node may use it) and gets no value either.
func TestAndroidOtherNodeRefused(t *testing.T) {
	needIntegration(t)
	p := newRig(t)
	res, st, out := p.run(run.SecretsRuntime{Mode: secrets.ModeAgentNode, NodeID: "sim-node-2", NodeEligible: false, Runner: nselfStub{},
		Facts: func(run.AttemptInfo) secrets.Facts { return secrets.Facts{} }})
	if res.Exit == 0 || res.Summary.Result == "pass" {
		t.Fatalf("the job passed on another node: exit %d %s", res.Exit, res.Summary.Result)
	}
	if _, err := os.Stat(p.marker); err == nil {
		t.Fatal("gradle ran although the keystore was refused")
	}
	if !strings.Contains(out, "E677") {
		t.Fatalf("no E677 in the output:\n%s", out)
	}
	if _, ok, _ := st.ReleaseFor(context.Background(), res.RunID); ok {
		t.Fatal("a release was recorded for a refused job")
	}
	if hits := sinkHits(t, p.planted(), map[string]string{"run output": out}, p.home, p.repo); len(hits) != 0 {
		t.Fatalf("planted values reached sinks: %v", hits)
	}
}
