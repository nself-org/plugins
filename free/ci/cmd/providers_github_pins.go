package main

// Purpose: the release pins of `providers github init --agent-source release` (Epic D5): for each
// runner platform GitHub offers, the signed agent release is fetched and verified through
// internal/agent/install and internal/agent/verify (P7-NODE-15), and only then does its URL and
// sha256 go into the workflow.
// Inputs:  agentPinnedVersion and agentVerifier (agent_source.go), env NSELF_CI_AGENT_RELEASE_URL.
// Outputs: the pinned version and one githubtmpl.ReleasePin per platform.
// Constraints: no pin is built from an unverified file; with no pinned version or no trusted key the
//          error is the install package's own (E665) and nothing is written; the verified binary is
//          read only for its digest and removed; a platform whose release file is missing fails the
//          whole command (a half-pinned workflow would fail on that runner).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/agent/install"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	githubtmpl "github.com/nself-org/plugins/free/ci/templates/github"
)

// githubReleasePins is replaced by tests; the default fetches and verifies the real release.
var githubReleasePins = fetchReleasePins

var releasePlatforms = []struct {
	lifecycle.Platform
	runnerOS, runnerArch string
}{
	{lifecycle.Platform{OS: "linux", Arch: "amd64"}, "Linux", "X64"},
	{lifecycle.Platform{OS: "linux", Arch: "arm64"}, "Linux", "ARM64"},
	{lifecycle.Platform{OS: "darwin", Arch: "arm64"}, "macOS", "ARM64"},
}

func fetchReleasePins(ctx context.Context) (string, []githubtmpl.ReleasePin, error) {
	v, err := agentVerifier()
	if err != nil {
		return "", nil, err
	}
	rel := install.Release{Version: agentPinnedVersion, BaseURL: os.Getenv("NSELF_CI_AGENT_RELEASE_URL"), Verifier: v}
	base := strings.TrimRight(rel.BaseURL, "/")
	if base == "" {
		base = install.DefaultBaseURL + "/v" + rel.Version
	}
	var pins []githubtmpl.ReleasePin
	for _, p := range releasePlatforms {
		path, cleanup, err := rel.Fetch(ctx, p.Platform)
		if err != nil {
			return "", nil, err
		}
		raw, rerr := os.ReadFile(path)
		cleanup()
		if rerr != nil {
			return "", nil, fmt.Errorf("verified agent for %s cannot be read: %w", p.Platform, rerr)
		}
		sum := sha256.Sum256(raw)
		pins = append(pins, githubtmpl.ReleasePin{RunnerOS: p.runnerOS, RunnerArch: p.runnerArch,
			URL: base + "/" + install.BinaryName + "-" + p.OS + "-" + p.Arch, SHA256: hex.EncodeToString(sum[:])})
	}
	return rel.Version, pins, nil
}
