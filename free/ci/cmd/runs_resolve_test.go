package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/job"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	resolveJob     = "run-1.publish-1a2b3c4d"
	resolveAttempt = resolveJob + ".a1"
)

// seedStuckRelease stores a run whose release attempt is in `state` under a
// fresh NSELF_CI_HOME and returns the store path.
func seedStuckRelease(t *testing.T, state string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	t.Setenv("NSELF_V15", "1")
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
	must(st.CreatePipeline(ctx, store.PipelineRow{ID: "run-1", Revision: strings.Repeat("a", 40), Project: "proj", Trigger: "local", SourceTrust: "owner",
		PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}))
	must(st.CreateJob(ctx, store.JobRow{ID: resolveJob, PipelineID: "run-1", Name: "publish", Kind: "release", InputDigest: "i"}))
	must(st.CreateAttempt(ctx, store.AttemptRow{ID: resolveAttempt, JobID: resolveJob, N: 1}, ""))
	path := map[string][]model.JobState{"needs-operator": {"leased", "lost", "needs-operator"}, "finalizing": {"leased", "running", "finalizing"}}[state]
	from := model.JobState("queued")
	for _, to := range path {
		_, epoch, err := st.AttemptStateEpoch(ctx, resolveAttempt)
		must(err)
		must(st.Transition(ctx, resolveAttempt, from, to, store.TransitionDetail{Epoch: epoch, FailureClass: "lost"}))
		from = to
	}
	return home
}

func attemptState(t *testing.T, home string) string {
	t.Helper()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	state, _, err := st.AttemptStateEpoch(context.Background(), resolveAttempt)
	if err != nil {
		t.Fatal(err)
	}
	return string(state)
}

// TestRunsResolveKeyMismatch: a key that does not match the stuck attempt is
// E716 key_mismatch, exit 1, and the attempt stays needs-operator.
func TestRunsResolveKeyMismatch(t *testing.T) {
	home := seedStuckRelease(t, "needs-operator")
	for _, flag := range []string{"--rerun", "--abandon"} {
		code, out, errs := capture(t, func() int {
			return runResolve([]string{resolveAttempt, "--idempotency-key", strings.Repeat("0", 64), flag, "--yes"})
		})
		if code != 1 || !strings.Contains(errs, "E716") || !strings.Contains(errs, job.KeyMismatch) || out != "" {
			t.Fatalf("%s: code %d stdout %q stderr %q", flag, code, out, errs)
		}
		if strings.Contains(errs, strings.Repeat("0", 64)) {
			t.Fatal("the key was echoed")
		}
	}
	if got := attemptState(t, home); got != "needs-operator" {
		t.Fatalf("state = %s", got)
	}
	// --json: one error envelope on stdout, nothing on stderr
	code, out, errs := capture(t, func() int {
		return runResolve([]string{resolveAttempt, "--idempotency-key", "nope", "--abandon", "--yes", "--json"})
	})
	var env struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || code != 1 || errs != "" || env.OK || env.Error.Code != "E716" || !strings.Contains(env.Error.Message, job.KeyMismatch) {
		t.Fatalf("json refusal: code %d stdout %q stderr %q (%v)", code, out, errs, err)
	}
}

func TestRunsResolveAbandonAndRerun(t *testing.T) {
	home := seedStuckRelease(t, "needs-operator")
	key := job.IdempotencyKey(resolveAttempt)
	code, out, errs := capture(t, func() int {
		return runResolve([]string{"--idempotency-key=" + key, "--rerun", "--yes", "--reason", "lost the network", "--json", resolveAttempt})
	})
	if code != 0 || errs != "" {
		t.Fatalf("rerun: code %d stderr %q", code, errs)
	}
	var env struct {
		Data job.Result `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.Data.Action != job.Rerun || env.Data.Next != resolveJob+".a2" {
		t.Fatalf("rerun output %q (%v)", out, err)
	}
	if got := attemptState(t, home); got != "needs-operator" {
		t.Fatalf("state after rerun approval = %s", got)
	}
	home = seedStuckRelease(t, "needs-operator")
	code, out, errs = capture(t, func() int {
		return runResolve([]string{resolveAttempt, "--idempotency-key", key, "--abandon", "--yes"})
	})
	if code != 0 || !strings.Contains(out, "failed") || errs != "" {
		t.Fatalf("abandon: code %d stdout %q stderr %q", code, out, errs)
	}
	if got := attemptState(t, home); got != "failed" {
		t.Fatalf("state after abandon = %s", got)
	}
}

func TestRunsResolveNeedsConfirmationAndValidArguments(t *testing.T) {
	home := seedStuckRelease(t, "needs-operator")
	key := job.IdempotencyKey(resolveAttempt)
	// no --yes and the operator does not confirm: refuse and change nothing
	code, _, errs := capture(t, func() int { return runResolve([]string{resolveAttempt, "--idempotency-key", key, "--abandon"}) })
	if code != 1 || !strings.Contains(errs, "nothing changed") || attemptState(t, home) != "needs-operator" {
		t.Fatalf("unconfirmed: code %d stderr %q state %s", code, errs, attemptState(t, home))
	}
	a := resolveArgs{attempt: resolveAttempt, action: job.Abandon}
	for name, c := range map[string]struct {
		in          string
		interactive bool
		yes         bool
		want        bool
	}{
		"typed yes":        {"yes\n", true, false, true},
		"typed yes padded": {"  yes  \n", true, false, true},
		"typed y":          {"y\n", true, false, false},
		"typed nothing":    {"", true, false, false},
		"no terminal":      {"yes\n", false, false, false},
		"--yes":            {"", false, true, true},
	} {
		a.yes = c.yes
		var got bool
		capture(t, func() int { got = confirm(a, strings.NewReader(c.in), c.interactive); return 0 })
		if got != c.want {
			t.Errorf("%s: confirm = %v", name, got)
		}
	}
	for name, args := range map[string][]string{
		"no attempt":      {"--idempotency-key", key, "--abandon"},
		"no key":          {resolveAttempt, "--abandon"},
		"no action":       {resolveAttempt, "--idempotency-key", key},
		"both actions":    {resolveAttempt, "--idempotency-key", key, "--abandon", "--rerun"},
		"two attempts":    {resolveAttempt, "other", "--idempotency-key", key, "--abandon"},
		"unknown flag":    {resolveAttempt, "--idempotency-key", key, "--abandon", "--bogus"},
		"key no value":    {resolveAttempt, "--abandon", "--idempotency-key"},
		"reason no value": {resolveAttempt, "--idempotency-key", key, "--abandon", "--reason"},
	} {
		code, _, errs = capture(t, func() int { return runResolve(args) })
		if code != 1 || !strings.Contains(errs, "usage: nself ci runs resolve") {
			t.Errorf("%s: code %d stderr %q", name, code, errs)
		}
	}
	if attemptState(t, home) != "needs-operator" {
		t.Fatal("a refused invocation changed the attempt")
	}
	// --force is a real flag (D-1078): it abandons a finalizing attempt of a live coordinator
	parsed, err := parseResolve([]string{resolveAttempt, "--idempotency-key", key, "--abandon", "--force"})
	if err != nil || !parsed.force {
		t.Fatalf("--force: %+v, %v", parsed, err)
	}
}

func TestRunsResolveIsAReleaseOnlyRunsSubcommand(t *testing.T) {
	if _, ok := runsSubs["resolve"]; !ok {
		t.Fatal("`runs resolve` is not registered")
	}
	home := seedStuckRelease(t, "finalizing")
	code, _, errs := capture(t, func() int {
		return runResolve([]string{resolveAttempt, "--idempotency-key", job.IdempotencyKey(resolveAttempt), "--rerun", "--yes"})
	})
	if code != 1 || !strings.Contains(errs, job.NotResolvable) || attemptState(t, home) != "finalizing" {
		t.Fatalf("code %d stderr %q state %s", code, errs, attemptState(t, home))
	}
}
