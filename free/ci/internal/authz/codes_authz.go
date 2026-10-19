package authz

// Purpose: the error codes of this package (Epic D25: E683 "artifact path
// rejected", E684 "cache write denied").
// Inputs:  none (registration at init).
// Outputs: CodeArtifactPath, CodeCacheWrite.
// Constraints: the file is not named codes.go on purpose. cmd/codes_registry_test.go
// requires every codes.go package to be linked into cmd (or listed in its
// notYetLinked map), and this Ticket may not edit cmd. The first Ticket that
// links authz into cmd (P7-CACHE) renames this file to codes.go in the same
// change (handoff note in the Ticket).

import "github.com/nself-org/plugins/free/ci/internal/model"

const (
	// CodeArtifactPath is E683.
	CodeArtifactPath = "E683"
	// CodeCacheWrite is E684.
	CodeCacheWrite = "E684"
)

func init() {
	model.Register(model.Code{ID: CodeArtifactPath, Class: "usage", Summary: "artifact path rejected",
		Fix: "Pack only regular files, directories and in-tree relative symlinks, or choose an empty destination outside .git"})
	model.Register(model.Code{ID: CodeCacheWrite, Class: "usage", Summary: "cache write denied",
		Fix: "A cache is written only by a passing attempt in finalizing, into its own namespace, with the current epoch"})
}
