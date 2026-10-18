package run

// Purpose: install the streaming redactor as the executor's Redactor, so every
// output chunk of every attempt passes it before the tail buffer, the renderer
// or any store sees it (P7-TRUST-19; the Redactor slot is ciexec.Executor's).
// Inputs:  values added with RegisterSecretValue (secret resolution, argv and
// services.SecretSink, which this file points at it). Outputs: a fresh *redact.Filter per
// attempt, built from the process-wide registry.
// Constraints: token shapes and the workflow-command / escape neutraliser work
// with no registered value. A value registered mid-run applies to bytes not yet
// released. No value is ever logged or placed in an error.

import (
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/services"
)

// Redactions is the registry every attempt's redactor reads.
var Redactions = redact.New()

// RegisterSecretValue adds a value to mask in every encoding. It returns the
// package's E673 error for a value under redact.MinValueLen bytes.
func RegisterSecretValue(v []byte) error { return Redactions.Register(v) }

func init() {
	// Every credential the engine generates for a service is registered here
	// (D-0977). The sink has no error return; a value under MinValueLen is
	// refused by the registry and generated credentials are far longer.
	services.SecretSink = func(secret string) { _ = RegisterSecretValue([]byte(secret)) }
	RegisterRedactor(func() ciexec.Redactor { return Redactions.NewFilter() })
}
