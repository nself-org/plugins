// Package contentsafety provides content scanning interfaces for promotion execution.
//
// Contract: publish.scanner.v1 (provides)
package contentsafety

import (
	"context"
	"errors"
)

// ErrScanFailed is returned when a scan cannot be completed.
// Implementations must return this or wrap it on any failure to enforce
// fail-closed semantics: if the scanner cannot confirm content is safe,
// the content must be treated as unsafe.
var ErrScanFailed = errors.New("content scan failed")

// ScanResult holds the outcome of a content scan.
type ScanResult struct {
	// Safe indicates whether the scanned content passed all safety checks.
	Safe bool

	// Reason provides additional context when content is deemed unsafe.
	Reason string
}

// Scanner defines the hook interface that a promotion executor invokes to
// check content before publishing. Implementations must follow fail-closed
// semantics: any error, timeout, or inability to reach a verdict must result
// in the content being rejected. Callers are required to pass a context with
// a deadline or timeout; implementations must respect context cancellation.
type Scanner interface {
	// Scan evaluates the provided content for safety.
	// The context must carry a timeout. If the scan cannot complete
	// successfully, an error wrapping ErrScanFailed must be returned.
	Scan(ctx context.Context, content []byte) (ScanResult, error)
}
