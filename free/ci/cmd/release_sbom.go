package main

// Purpose: install the generic release SBOM hook (debt D-1271): every release
// job whose run ends with release.sbom gets both documents per artifact before
// its outputs are collected, whichever command starts the run.
// Inputs:  the pinned syft compiled into the binary (resolved on first use).
// Outputs: the hook in the registered workspace hooks, ahead of the artifacts hook.
// Constraints: the Android lane keeps its own step (android.Install); its job
// does not end with release.sbom, so the two never both apply.

import "github.com/nself-org/plugins/free/ci/internal/release/sbomhook"

func init() { sbomhook.Register(sbomhook.Hook{}) }
