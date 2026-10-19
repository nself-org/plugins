package publish_test

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/release/verify"
)

// acceptEvidenceGap runs the real verifier. The fixture has no git repository,
// so the last step (the evidence verdict at the revision) reports
// revision_absent; every step before it must have passed, which is the
// signature, manifest, checksum, evidence binding, provenance and SBOM checks.
// The integration tests run the whole chain with a real repository.
func acceptEvidenceGap(ctx context.Context, dir string, opt verify.Options) verify.Result {
	res := verify.Release(ctx, dir, opt)
	if !res.Verified && res.Reason == verify.ReasonRevisionAbsent && len(res.Passed) == 6 {
		res.Verified, res.Exit, res.Reason = true, 0, ""
	}
	return res
}
