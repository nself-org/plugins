package main

// Purpose: `nself-ci-agent upgrade --version V [--manifest-url URL] [--allow-downgrade]`: the agent-side handler of the protocol
//          `upgrade` message and of an admin's manual upgrade (hq NODE D11, P7-NODE-24). It refuses while a job runs, fetches and
//          verifies the signed release, checks the protocol ceiling of the recorded coordinators, swaps its own binary
//          atomically and keeps the old one as <bin>.prev. A version equal to the kept .prev binary's is a restore. The verified
//          file is run as `<bin> version` before the swap and the installed file after it (internal/exec's seam); a binary that
//          does not start or reports another version is never left in place.
// Inputs:  --version (x.y.z), --manifest-url (the https URL of nself-ci-agent-<os>-<arch>.manifest.json; default the GitHub
//          release of the version), --allow-downgrade; the state dir (slots, coordinators.json).
// Outputs: one line on stdout; "E666: ..." or "E656: ..." on stderr. Exit 0 swapped (restart the agent to run it: the service
//          manager restarts a pull agent, the next ssh session starts the new binary), 1 refused or failed, 2 usage.
//          The coordinator's `upgrade` frame runs the same swap through upgrade_frame.go.
// Constraints: the trusted keys are the compiled-in set only; nothing is executed before it verified (the version run comes
//          after the signature and digest checks); the downgrade
//          rules and the ceiling are internal/agent/update's.

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/nself-org/plugins/free/ci/internal/agent/install"
	"github.com/nself-org/plugins/free/ci/internal/agent/update"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

// upgradeVerifier is the trusted key set; tests replace it.
var upgradeVerifier = verify.Default

// upgradeRunVersion starts a candidate binary as `<bin> version`; tests replace it.
var upgradeRunVersion = ciexec.RunAgentVersion

func init() {
	must(RegisterSubcommand("upgrade", "replace this agent with a signed release (keeps the old binary as .prev)", runUpgrade))
}

func runUpgrade(ctx context.Context, args []string, sio IO) int {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(sio.Stderr)
	version := fs.String("version", "", "release version to install (x.y.z)")
	manifestURL := fs.String("manifest-url", "", "https URL of this platform's .manifest.json (default: the GitHub release of --version)")
	allowDown := fs.Bool("allow-downgrade", false, "allow a version lower than this agent's")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || !verify.ValidVersion(*version) {
		_, _ = fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent upgrade --version X.Y.Z [--manifest-url URL] [--allow-downgrade]")
		return 2
	}
	base, err := manifestBase(*manifestURL)
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, err)
		return 2
	}
	state, err := stateDir()
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	exe, err := agentBinary()
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error: agent binary:", err)
		return 1
	}
	v, err := upgradeVerifier()
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, err)
		return 1
	}
	res, err := update.ApplyLocal(ctx, update.LocalOptions{
		StateDir: state, BinPath: exe, Current: Version, Version: *version, AllowDowngrade: *allowDown,
		Platform:   lifecycle.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Source:     update.ReleaseSource{Release: install.Release{Version: *version, BaseURL: base, Verifier: v}},
		RunVersion: upgradeRunVersion,
	})
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, err)
		return 1
	}
	if res.Noop {
		_, _ = fmt.Fprintf(sio.Stdout, "nself-ci-agent already runs %s\n", res.To)
		return 0
	}
	_, _ = fmt.Fprintf(sio.Stdout, "nself-ci-agent %s -> %s installed; the previous binary is kept as %s%s; restart the agent to run it\n", res.From, res.To, exe, update.PrevSuffix)
	return 0
}

// manifestBase turns a manifest URL into the release base it lives under; "" is the default (the GitHub release). The URL must
// name this platform's manifest (E651).
func manifestBase(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	target := verify.Target(runtime.GOOS, runtime.GOARCH)
	u, err := url.Parse(raw)
	if err != nil || path.Base(u.Path) != install.BinaryName+"-"+target+".manifest.json" {
		return "", fmt.Errorf("E651: --manifest-url must name %s-%s.manifest.json", install.BinaryName, target)
	}
	u.Path = path.Dir(u.Path)
	return u.String(), nil
}

// agentBinary is the path of the running agent binary with its symlinks resolved.
func agentBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}
