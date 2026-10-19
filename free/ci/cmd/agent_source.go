package main

// Purpose: makes the signed release the default way `nself ci nodes add` gets an agent when no --agent-binary is given
//          (hq NODE D11, P7-NODE-15). The Release source downloads the pinned version, verifies it through
//          internal/agent/verify and only then hands the file to the installer.
// Inputs:  agentPinnedVersion (set at release time with -ldflags "-X main.agentPinnedVersion=<x.y.z>"; empty until P7-NODE-29
//          pins the first signed release); env NSELF_CI_AGENT_RELEASE_URL (a mirror or fixture directory URL; https, or http on a
//          loopback host).
// Outputs: lifecycle.RegisterDefaultSource at init.
// Constraints: the trusted keys are the compiled-in set only; there is no flag or variable that adds a key. With no key
//          compiled in, the source fails with E665 naming --agent-binary, which is the state until P7-NODE-29.

import (
	"context"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/agent/install"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

// agentPinnedVersion is the agent release `nodes add` installs by default.
var agentPinnedVersion = ""

// agentVerifier is the trusted key set; tests replace it with a generated key.
var agentVerifier = verify.Default

func init() {
	lifecycle.RegisterDefaultSource(releaseSource)
}

// releaseSource builds the Release source from the pin and the environment.
func releaseSource() lifecycle.AgentSource {
	v, err := agentVerifier()
	if err != nil {
		return failingSource{err}
	}
	return install.Release{Version: agentPinnedVersion, BaseURL: os.Getenv("NSELF_CI_AGENT_RELEASE_URL"), Verifier: v}
}

// failingSource reports a setup error when the source is used.
type failingSource struct{ err error }

func (f failingSource) Fetch(_ context.Context, _ lifecycle.Platform) (string, func(), error) {
	return "", nil, f.err
}
