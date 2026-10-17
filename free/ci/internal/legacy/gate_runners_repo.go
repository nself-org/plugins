package legacy

// Repo-owned gate scripts (D-0482, D-0483): the Go stack's extra gates that
// live in the repo itself, run by runGoGates with a deadline of their own.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// repoGoScripts are gate scripts a Go repo owns. Each runs, with bash from the
// repo root, only when the repo ships it, so nself ci and the repo's GitHub
// workflow apply one rule. The cli ships both (D-0482, D-0483).
var repoGoScripts = []struct{ name, path string }{
	// vendor/ must equal `go mod vendor`: catches a replaced sdk module whose
	// fix never reached the vendored copy the binary builds from.
	{"go:vendor-sync", "scripts/ci/vendor-sync-check.sh"},
	// sdk/go/remote mutation gate, driven by its allowlist header.
	{"go:sdk-remote-mutation", "scripts/ci/sdk-remote-mutation.sh"},
}

// repoScriptDeadline bounds one repo gate script. runStep has no deadline of
// its own, and the mutation gate needs the module proxy and a few minutes, so
// a hang must end as a failed gate, not a stuck run. A var so tests can shrink it.
var repoScriptDeadline = 30 * time.Minute

// runRepoGoScripts runs every repoGoScripts entry present under root. Each is
// a required gate: offline or timed out reads as failed, never as skipped.
func runRepoGoScripts(root string, timeout int, verbose bool) []GateResult {
	var gates []GateResult
	for _, s := range repoGoScripts {
		if !fileExists(filepath.Join(root, filepath.FromSlash(s.path))) {
			continue
		}
		gates = append(gates, runRepoScript(s.name, root, s.path, verbose))
	}
	return gates
}

// runRepoScript runs `bash path` in root under repoScriptDeadline. At the
// deadline the script's whole process group is killed (unix), so children
// such as gremlins or go test die with it, and its pipes are closed shortly
// after even if something still holds them.
func runRepoScript(name, root, path string, verbose bool) GateResult {
	gr, _ := runRepoScriptResult(name, root, path, verbose)
	return gr
}

// runRepoScriptResult preserves the execution error for callers that distinguish
// findings from gate errors, while runRepoScript retains its existing verdict.
func runRepoScriptResult(name, root, path string, verbose bool) (GateResult, error) {
	start := time.Now()
	gr := GateResult{Name: name}
	if _, err := exec.LookPath("bash"); err != nil {
		gr.Output = "command not found: bash (skipped)"
		gr.Passed = true
		gr.Skipped = true
		gr.Elapsed = time.Since(start)
		return gr, err
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "[nself-ci] running: bash %s\n", path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), repoScriptDeadline)
	defer cancel()
	c := exec.CommandContext(ctx, "bash", path)
	c.Dir = root
	c.WaitDelay = 10 * time.Second
	killProcessGroupOnCancel(c)
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	gr.Output = strings.TrimSpace(buf.String())
	gr.Elapsed = time.Since(start)
	gr.Passed = err == nil
	if ctx.Err() == context.DeadlineExceeded {
		gr.Passed = false
		gr.Output = strings.TrimSpace(fmt.Sprintf("timed out after %s\n%s", repoScriptDeadline, gr.Output))
	}
	return gr, err
}
