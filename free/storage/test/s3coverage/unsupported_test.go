//go:build s3coverage

package s3coverage

import (
	"errors"
	"testing"

	"github.com/aws/smithy-go"
)

// unsupportedOp is a pinned expectation: the target answers this operation with wantCode, on purpose.
type unsupportedOp struct {
	wantCode string
	reason   string
}

// unsupported is the ONE table of declared gaps: target -> operation -> expectation.
// A declared op must keep failing with exactly wantCode; success or any other error fails the suite, so the
// table cannot go stale. Add an entry only with a reason and a decision from the lead.
var unsupported = map[string]map[string]unsupportedOp{
	"minio": {
		"PutBucketCors": {
			wantCode: "NotImplemented",
			reason:   "pgsty/minio RELEASE.2026-08-04 (MinIO community) has no bucket CORS API; CORS is server-wide (MINIO_API_CORS_ALLOW_ORIGIN, default *)",
		},
	},
}

// expectUnsupported reports whether the current target declares op unsupported. When it does, err must be an
// APIError with the declared code; any other outcome (including nil) fails the test.
func expectUnsupported(t *testing.T, op string, err error) bool {
	t.Helper()
	decl, ok := unsupported[env.target][op]
	if !ok {
		return false
	}
	var ae smithy.APIError
	if err == nil || !errors.As(err, &ae) || ae.ErrorCode() != decl.wantCode {
		t.Fatalf("declared-unsupported op now behaves differently: update unsupported_test.go (target %s, op %s, want %s, got %v)",
			env.target, op, decl.wantCode, err)
	}
	return true
}
