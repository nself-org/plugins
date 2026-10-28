//go:build integration

package publish_test

// Purpose: D-1244. The release pipeline with the production signer: the real
// `nself-ci artifacts signer` subprocess, started through the exec funnel by the
// real signing.Client (key file, request framing, timeout), not the in-process
// handler the other pipeline tests use.
// Inputs:  a built nself-ci binary and a fake `nself` cli on PATH that serves the
// test key from a private directory (the cli.secret-resolve/v1 contract).
// Outputs: a published, verifiable release whose envelopes the subprocess signed.
// Constraints: a missing prerequisite (go, git) is t.Fatal, never t.Skip; unix only
// (the funnel's stdin FIFO); no network.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// realSigner builds nself-ci once, installs a fake cli holding the rig's key and
// returns the constructor of the production client over them.
func (p *pipeRig) realSigner() func(signing.Source) publish.Signer {
	t := p.t
	if runtime.GOOS == "windows" {
		t.Fatal("the release signer subprocess needs unix")
	}
	exe := filepath.Join(t.TempDir(), "nself-ci")
	build := exec.Command("go", "build", "-mod=vendor", "-o", exe, "../../../cmd")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	secrets := t.TempDir()
	if err := os.WriteFile(filepath.Join(secrets, signing.KeyName), p.keyValue, 0o600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := `#!/bin/sh
S='` + secrets + `'
if [ "$1 $2 $3" = "config secrets resolve" ]; then
  case "$4" in --help) exit 0;; esac
  ref="$9"; att="$7"; name="${ref#release/}"
  if [ ! -f "$S/$name" ]; then printf '{"error":{"code":"E541"}}\n'; exit 1; fi
  printf '{"data":{"schema":"cli.secret-resolve/v1","attempt_id":"%s","values":[{"ref":"%s","env_name":"%s","value":"%s","source":"age:release"}]}}\n' "$att" "$ref" "$name" "$(cat "$S/$name")"
  exit 0
fi
exit 8
`
	if err := os.WriteFile(filepath.Join(bin, "nself"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func(src signing.Source) publish.Signer {
		return &signing.Client{Runner: &keyCheckingRunner{t: t, next: &signing.FunnelRunner{Home: p.home}, key: p.keyValue}, Src: src, Exe: exe,
			Env: []string{"NSELF_CI_HOME=" + p.home, "NSELF_CI_REPO=" + p.repo}}
	}
}

// TestReleasePipelineRealSigner: publish.Run signs through the real signer
// subprocess; the release is published, exported and verifies against the
// trust line the same subprocess prints.
func TestReleasePipelineRealSigner(t *testing.T) {
	p := newPipeRig(t)
	p.signerFor = p.realSigner()
	res, st := p.run(goodRelease, true)
	if res.Exit != 0 || res.Summary.Result != "pass" {
		t.Fatalf("exit %d result %s notes %v", res.Exit, res.Summary.Result, res.Summary.Notes)
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
	info, err := d.Signer.PublicKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v := p.verifyDir(exp.Dir, info); !v.Verified || v.Exit != 0 {
		t.Fatalf("artifacts verify: %+v", v)
	}
	if p.res.calls != 0 {
		t.Fatalf("the in-process resolver ran %d times: the subprocess was bypassed", p.res.calls)
	}
	if got := string(readDir(t, exp.Dir)["artifacts/app"]); got != "app-bytes" {
		t.Fatalf("artifact bytes %q", got)
	}
}

// keyCheckingRunner observes the argv handed to the real exec funnel.
type keyCheckingRunner struct {
	t    *testing.T
	next signing.StdioRunner
	key  []byte
}

func (r *keyCheckingRunner) RunStdio(ctx context.Context, call signing.StdioCall) ([]byte, int, error) {
	assertKeyAbsent(r.t, r.key, "signer argv", strings.Join(call.Argv, "\x00"))
	return r.next.RunStdio(ctx, call)
}

func assertKeyAbsent(t *testing.T, key []byte, sink, text string) {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(string(key))
	if err != nil || len(decoded) == 0 {
		t.Fatal("invalid planted test key")
	}
	if strings.Contains(text, string(key)) || strings.Contains(text, string(decoded)) {
		t.Fatalf("planted key leaked to %s", sink)
	}
}

func TestPlantedKeyAbsentFromPublicationSinks(t *testing.T) {
	p := newPipeRig(t)
	p.signerFor = p.realSigner()
	res, st := p.run(goodRelease, true)
	if res.Exit != 0 {
		t.Fatalf("pipeline exit %d", res.Exit)
	}
	foundHookNote := false
	for _, note := range res.Summary.Notes {
		if strings.Contains(note, "release published: ") {
			foundHookNote = true
		}
	}
	if !foundHookNote {
		t.Fatal("publication hook note was not captured")
	}
	assertKeyAbsent(t, p.keyValue, "hook notes", strings.Join(res.Summary.Notes, "\n"))
	row, ok, err := st.ReleaseFor(context.Background(), res.RunID)
	if err != nil || !ok {
		t.Fatalf("release row: %v", err)
	}
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	assertKeyAbsent(t, p.keyValue, "ledger", string(raw))
	// Invoke the real artifacts sign command, including its own signer process.
	// realSigner has already built and configured the executable and fake cli.
	client := p.deps(st).Signer.(*signing.Client)
	for _, flags := range [][]string{nil, {"--json"}} {
		args := append([]string{"artifacts", "sign", res.RunID, "--project", p.repo}, flags...)
		cmd := exec.Command(client.Exe, args...)
		cmd.Env = append(os.Environ(), "NSELF_V15=1", "NSELF_CI_HOME="+p.home, "NSELF_CI_REPO="+p.repo)
		out, err := cmd.CombinedOutput()
		assertKeyAbsent(t, p.keyValue, "artifacts sign output", string(out))
		if err != nil {
			t.Fatalf("artifacts sign failed: %v", err)
		}
		if len(out) == 0 {
			t.Fatal("artifacts sign output was not captured")
		}
	}
	// Sweep every on-disk sink, including event logs, evidence, the ledger
	// (and its WAL), and release files. The fake secret store lives elsewhere.
	for name, data := range readDir(t, p.home) {
		assertKeyAbsent(t, p.keyValue, name, string(data))
	}

}
