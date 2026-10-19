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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
		return &signing.Client{Runner: &signing.FunnelRunner{Home: p.home}, Src: src, Exe: exe,
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
