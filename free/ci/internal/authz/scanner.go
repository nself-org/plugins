package authz

// Purpose: the shape P7-CACHE's stores call to find registered secret values
// inside cache blobs and artifacts before commit. *redact.Redactor satisfies it
// (compile-checked in the test package).
// Inputs:  a byte slice. Outputs: sorted, non-overlapping matches.
// Constraints: Match is an alias of redact.Match on purpose, so the redactor's
// Scan has exactly this method set (Epic Invalidations 2026-10-18).

import "github.com/nself-org/plugins/free/ci/internal/redact"

// Match is one occurrence: Len bytes starting at Offset.
type Match = redact.Match

// ValueScanner reports occurrences of registered secret values in a blob.
// Scan must be safe for concurrent use: the cache save scans windows of one
// stream on several goroutines (D-1515).
type ValueScanner interface {
	Scan(blob []byte) []Match
}

// Clean reports whether s finds nothing in blob. A nil scanner is not clean:
// a store with no scanner must refuse, not pass.
func Clean(s ValueScanner, blob []byte) bool {
	return s != nil && len(s.Scan(blob)) == 0
}
