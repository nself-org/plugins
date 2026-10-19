package entries

// Purpose: the error code of this package (Epic D20: E711 "cache write
// refused") and the typed refusal the store and the entries Store return.
// Inputs: none (registration at init). Outputs: CodeCacheRefused, Refused, the
// Reason* keys.
// Constraints: the file is not named codes.go on purpose. cmd/codes_registry_test.go
// requires every codes.go package to be linked into cmd (or listed in its
// notYetLinked map) and this Ticket may not edit cmd (same case as
// internal/authz/codes_authz.go). The Ticket that links entries into cmd
// (P7-CI-37) renames this file to codes.go in the same change.

import "github.com/nself-org/plugins/free/ci/internal/model"

// CodeCacheRefused is E711.
const CodeCacheRefused = "E711"

// Reason keys of E711 used by this Ticket (secret_match and path_outside at
// restore are P7-CACHE-02's).
const (
	ReasonAttemptState = "attempt_state"
	ReasonEpoch        = "epoch"
	ReasonNamespace    = "namespace"
	ReasonNotPlanned   = "not_planned"
	ReasonSizeCap      = "size_cap"
	ReasonDiskFloor    = "disk_floor"
	ReasonPathOutside  = "path_outside"
)

// Refused is E711; Reason is one of the Reason* keys.
type Refused struct{ Reason string }

func (e *Refused) Error() string { return CodeCacheRefused + " cache write refused: " + e.Reason }

// Code returns the fabric code.
func (e *Refused) Code() string { return CodeCacheRefused }

func init() {
	model.Register(model.Code{ID: CodeCacheRefused, Class: "usage", Summary: "cache write refused",
		Fix: "A cache entry is saved only by a passing attempt in finalizing, with the current epoch, into its own namespace and a planned key"})
}
