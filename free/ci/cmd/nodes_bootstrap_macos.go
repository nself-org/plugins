package main

// Purpose: registers the darwin bootstrap family in the `nself ci` binary (P7-NODE-31, Epic D20), so
// `nodes add --system` can install a Mac node from any coordinator. The file is named _macos, not
// _darwin: a _darwin.go suffix is a GOOS constraint, and a Linux coordinator must register the darwin
// family too (its steps run over ssh; only dscl and launchctl run on the Mac).
// Inputs:  none. Outputs: the "darwin" family in the bootstrap registry (nodes_bootstrap.go picks it with
//          bootstrap.Detect when `uname -s` answers Darwin).
// Constraints: a blank import only; the package carries no build tag.

import (
	_ "github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap/darwin"
)
