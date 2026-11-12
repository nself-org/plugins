package main

// Purpose: P7-WIRE-09: buildWatcher wires the hosted tier (P7-AUTO-07) and the idle tick (P7-AUTO-20)
//          into `ci serve watch`. Every test goes through buildWatcher with the real gh funnel; gh is a
//          stub on PATH that answers the check-run reads of one commit.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/hostedtier"
)

const wire09SHA = "0123456789abcdef0123456789abcdef01234567"

// wire09Fixture makes a checkout with a GitHub origin, a CI home and a gh stub whose check-run answer
// is runsJSON, then builds the watcher with default flags.
func wire09Fixture(t *testing.T, runsJSON string) (w interface{ hosted() hostedtier.Result }, idleSet bool) {
	t.Helper()
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	home := filepath.Join(base, "home")
	bin := filepath.Join(base, "bin")
	for _, d := range []string{repo, home, bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, argv := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://github.com/acme/widgets.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, argv...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", argv, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(bin, "runs.json"), []byte(runsJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"  *annotations*) echo '[]' ;;\n" +
		"  *check-runs*) cat '" + filepath.Join(bin, "runs.json") + "' ;;\n" +
		"  *) exit 1 ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NSELF_CI_HOME", home)
	t.Setenv("NSELF_CI_REPO", repo)
	t.Chdir(repo)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	wt, closeFn, err := buildWatcher(ctx, serveWatchFlags{})
	if err != nil {
		t.Fatalf("buildWatcher: %v", err)
	}
	t.Cleanup(closeFn)
	if wt.P.Hosted == nil {
		t.Fatal("buildWatcher left the Hosted port nil: the hosted tier is off")
	}
	return hostedProbe{decide: func() hostedtier.Result {
		return wt.P.Hosted.Decide(ctx, "acme/widgets", wire09SHA, time.Now().Add(-time.Minute))
	}}, wt.P.Idle != nil
}

type hostedProbe struct{ decide func() hostedtier.Result }

func (p hostedProbe) hosted() hostedtier.Result { return p.decide() }

func wire09Run(app int64, status, conclusion string) string {
	at := time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339)
	return `{"total_count":1,"check_runs":[{"id":42,"name":"` + hostedtier.DefaultContext + `","status":"` + status +
		`","conclusion":"` + conclusion + `","started_at":"` + at + `","app":{"id":` + strconv.FormatInt(app, 10) + `}}]}`
}

func TestWire09BuildWatcherSetsHostedAndIdle(t *testing.T) {
	_, idle := wire09Fixture(t, `{"total_count":0,"check_runs":[]}`)
	if !idle {
		t.Fatal("buildWatcher left the Idle port nil: the background tick is off")
	}
}

func TestWire09HostedHandoverSkipsDuplicate(t *testing.T) {
	// A commit already green on the free Actions run: the hosted result stands, no local run.
	w, _ := wire09Fixture(t, wire09Run(hostedtier.GitHubActionsAppID, "completed", "success"))
	if r := w.hosted(); r.Decision != hostedtier.Use || r.Reason != hostedtier.ReasonSuccess {
		t.Fatalf("green Actions run: got %s/%s, want use/%s", r.Decision, r.Reason, hostedtier.ReasonSuccess)
	}
}

func TestWire09HostedFailsClosed(t *testing.T) {
	// The same name from another app is not the hosted result; an old absence hands over to local nodes.
	w, _ := wire09Fixture(t, wire09Run(999, "completed", "success"))
	if r := w.hosted(); r.Decision != hostedtier.HandOver {
		t.Fatalf("a foreign app's green run: got %s/%s, want hand_over", r.Decision, r.Reason)
	}
	// An unreadable answer from gh is an API error and hands over, never a pass.
	w, _ = wire09Fixture(t, `not json`)
	if r := w.hosted(); r.Decision != hostedtier.HandOver || r.Reason != hostedtier.ReasonAPIError {
		t.Fatalf("unreadable check runs: got %s/%s, want hand_over/%s", r.Decision, r.Reason, hostedtier.ReasonAPIError)
	}
}
