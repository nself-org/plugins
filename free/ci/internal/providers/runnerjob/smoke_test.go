//go:build integration

package runnerjob_test

import (
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/github/demand"
)

// TestRunnerImageSmoke proves the last acceptance line of the launch (review F6): the pinned actions-runner
// image, started through the credential wrapper with the exact command the handler gives a runner job and
// an invalid JIT config, exits non-zero within 60 seconds and its output does not contain the config value.
// It also pins the one assumption the design rests on: the runner reads ACTIONS_RUNNER_INPUT_JITCONFIG
// (its error names the base-64 decode of that input), so the credential never needs argv or a file.
func TestRunnerImageSmoke(t *testing.T) {
	needDocker(t, realImage)
	const invalid = "synthetic-invalid-jitconfig-smoke-0001"
	for _, sandboxed := range []bool{false, true} {
		id := map[bool]string{false: "host19-smoke-c", true: "host19-smoke-g"}[sandboxed]
		start := time.Now()
		run, ran := runLegWith(t, id, realImage, demand.CredentialKind, invalid, demand.RunnerCommand, sandboxed, 60*time.Second, true, nil)
		if !ran {
			continue
		}
		took := time.Since(start)
		t.Logf("level %s: exit %d in %s: %q", run.Level, run.Res.ExitCode, took.Round(time.Second), strings.TrimSpace(run.Out))
		if took >= 60*time.Second || run.Res.ExitCode == 0 && run.Res.Err == nil {
			t.Errorf("level %s: want a non-zero exit within 60 s, got exit %d err %v after %s", run.Level, run.Res.ExitCode, run.Res.Err, took)
		}
		for name, text := range map[string]string{"output": run.Out, "ps list": run.Ps, "container config": run.Inspect} {
			if strings.Contains(text, invalid) {
				t.Errorf("level %s: the config value is in the %s", run.Level, name)
			}
		}
		if !strings.Contains(run.Out, "Base-64") {
			t.Errorf("level %s: the runner did not complain about the base-64 JIT config; does it still read ACTIONS_RUNNER_INPUT_JITCONFIG?\n%s", run.Level, run.Out)
		}
	}
}
