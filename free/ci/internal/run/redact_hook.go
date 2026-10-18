package run

// Purpose: install the streaming redactor as the executor's Redactor, so every
// output chunk of every attempt passes it before the tail buffer, the renderer
// or any store sees it (P7-TRUST-19; the Redactor slot is ciexec.Executor's).
// Inputs:  values added with RegisterSecretValue (secret resolution, argv and
// credential registration call it later). Outputs: a fresh *redact.Filter per
// attempt, built from the process-wide registry.
// Constraints: token shapes and the workflow-command / escape neutraliser work
// with no registered value. A value registered mid-run applies to bytes not yet
// released. No value is ever logged or placed in an error.

import (
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// Redactions is the registry every attempt's redactor reads.
var Redactions = redact.New()

// RegisterSecretValue adds a value to mask in every encoding. It returns the
// package's E673 error for a value under redact.MinValueLen bytes.
func RegisterSecretValue(v []byte) error { return Redactions.Register(v) }

func init() {
	RegisterRedactor(func() ciexec.Redactor { return Redactions.NewFilter() })
}
