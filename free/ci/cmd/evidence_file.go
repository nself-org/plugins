package main

// Purpose: `nself ci evidence verify --file <f>`: read the document (bounded)
// and check it against the revision it names.
// Inputs:  the file path and flags. Outputs: an evidence.Verdict or an error
// with exit 11 when the file cannot be read or decoded.

import (
	"context"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
)

func verifyFile(ctx context.Context, env evidence.Env, f evidenceFlags) (evidence.Verdict, error) {
	fh, err := os.Open(f.file)
	if err != nil {
		return evidence.Verdict{}, &evidence.Error{Code: evidence.CodeRejected, Message: "cannot read the evidence file: " + err.Error(), Exit: evidence.ExitInvalid}
	}
	defer func() { _ = fh.Close() }()
	doc, err := evidence.ReadDocument(fh)
	if err != nil {
		return evidence.Verdict{}, err
	}
	return evidence.VerifyFile(ctx, env, doc, f.atRev)
}
