// Package errs holds the sentinel errors this plugin's operations wrap.
//
// Purpose: DR operations return errors that callers match with errors.Is, so
// the sentinels have to be identities, not messages. The four here are the ones
// the drill, promote, rollback and fence paths wrap.
//
// Constraints: a deliberate copy of four values from the CLI's internal/errs.
// That package is 1,103 lines and defines sentinels for the whole CLI; a plugin
// is a separate module and cannot import it. Copy another sentinel here if a
// later change needs one, never the package.
//
// These are distinct values from the CLI's identically-named ones. Nothing
// crosses the process boundary — the plugin runs as its own binary and reports
// through its exit code — so the two never need to compare equal.
package errs

import "errors"

var (
	// ErrDRDrillFailed is returned when a disaster-recovery drill does not pass.
	ErrDRDrillFailed = errors.New("DR drill failed")
	// ErrDRPromoteFailed is returned when promoting a standby fails.
	ErrDRPromoteFailed = errors.New("standby promotion failed")
	// ErrDRRollbackFailed is returned when rolling back a promotion fails.
	ErrDRRollbackFailed = errors.New("DR rollback failed")
	// ErrDRFenceFailed is returned when fencing the old primary fails.
	ErrDRFenceFailed = errors.New("split-brain fence failed")
)
