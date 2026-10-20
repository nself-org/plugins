//go:build integration

package runners_test

// Purpose: the image smoke (acceptance 5): the real pinned gitlab-runner image started through the
// production path.
// Inputs:  the pinned image reference, the container leg, a fixture token that exists nowhere.
// Outputs: pass or fail.
// Constraints: Docker is required; the network is none, so the runner reaches no server.

import (
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
)

// TestRunnerImageSmoke proves acceptance 5 (reworded by the lead ruling in the HOST Invalidations, because
// the pinned runner never exits non-zero on its own against an unusable token: a forbidden answer leaves it
// idle, no answer or an empty one makes it exit 0 at --wait-timeout): the pinned image, started the way the
// agent starts a lease (the agent as entrypoint, the wrapper, the stdin token, the exact RunnerCommand with
// only the wait timeout cut from 120 to 10 s) takes its token from CI_SERVER_TOKEN and its URL from --url
// ("Starting runner for <url> with token <nine>"; an empty token would be "FATAL: Missing Token", exit 1),
// reaches no job, and leaves by itself within 60 s; neither the whole token nor the nine characters it
// prints reach the output, ps or `docker inspect` once cmd/agent's partial mask is applied. The run without
// the partial mask is the positive control: the nine characters ARE printed then.
func TestRunnerImageSmoke(t *testing.T) {
	ref, err := runners.PinnedImage()
	if err != nil {
		t.Fatal(err)
	}
	needDocker(t, ref)
	const token, printed = "glrt-SmokeTok0123", "SmokeTok0" // 11 characters after the prefix: short enough that no scanner reads it as a real token
	cmd := runners.RunnerCommand("https://gitlab.invalid")
	for i, a := range cmd {
		if a == "--wait-timeout" {
			cmd[i+1] = "10"
		}
	}
	for _, sandboxed := range []bool{false, true} {
		for _, masked := range []bool{false, true} {
			var masks []string
			if masked {
				masks = []string{printed}
			}
			id := "host07-smoke-" + map[bool]string{false: "c", true: "g"}[sandboxed] + map[bool]string{false: "u", true: "m"}[masked]
			start := time.Now()
			run, ran := runLeg(t, legOptions{ID: id, Image: ref, Cred: token, Command: cmd, Sandboxed: sandboxed, Limit: 60 * time.Second, Masks: masks})
			if !ran {
				continue
			}
			took := time.Since(start)
			t.Logf("level %s masked=%v: exit %d in %s", run.Level, masked, run.Res.ExitCode, took.Round(time.Second))
			if took >= 60*time.Second || run.Res.ExitCode != 0 || run.Res.Err != nil {
				t.Errorf("level %s: want the runner to leave by itself with exit 0 within 60 s, got exit %d err %v after %s\n%s", run.Level, run.Res.ExitCode, run.Res.Err, took, run.Out)
			}
			if !strings.Contains(run.Out, "Starting runner for https://gitlab.invalid with token") || strings.Contains(run.Out, "Missing Token") {
				t.Errorf("level %s: the runner did not start with the URL and the token from the wrapper:\n%s", run.Level, run.Out)
			}
			for name, text := range map[string]string{"output": run.Out, "ps list": run.Ps, "container config": run.Inspect} {
				if strings.Contains(text, token) {
					t.Errorf("level %s: the whole token is in the %s: %s", run.Level, name, lineWith(text, token))
				}
			}
			if strings.Contains(run.Inspect, "CI_SERVER_TOKEN") || strings.Contains(run.Inspect, printed) {
				t.Errorf("level %s: the container configuration carries the token or its prefix", run.Level)
			}
			if has := strings.Contains(run.Out, printed); has == masked {
				t.Errorf("level %s masked=%v: the nine printed characters are in the output: %v (the runner prints them; the mask must remove them)\n%s", run.Level, masked, has, run.Out)
			}
		}
	}
}

// lineWith returns the first line of text that contains needle, cut to 300 bytes.
func lineWith(text, needle string) string {
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(l, needle) {
			return l[:min(len(l), 300)]
		}
	}
	return ""
}
