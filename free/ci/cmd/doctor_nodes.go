package main

// Purpose: links the `nodes` section of `nself ci doctor` (P7-NODE-16, hq NODE D15) into the shipped binary and gives
// it the limits of this build. The internal/nodes/doctor package registers itself from init, so without this import
// `doctor --nodes` would report "none registered". There is no `ci nodes doctor` subcommand (X18): the section
// is reached only through `nself ci doctor --nodes [--json]`.
// Inputs:  minAgentVersion (transport_ssh.go) and agentPinnedVersion (agent_source.go, set by -ldflags at release).
// Outputs: nodesdoctor.Settings filled; no registry entry of its own.
// Constraints: Settings is read at run time, so the ldflags-set pinned version is the released one.

import (
	nodesdoctor "github.com/nself-org/plugins/free/ci/internal/nodes/doctor"
)

func init() {
	nodesdoctor.Settings = nodesdoctor.Limits{
		MinAgentVersion: minAgentVersion,
		MinProtocol:     1,
		PinnedRelease:   func() string { return agentPinnedVersion },
	}
}
