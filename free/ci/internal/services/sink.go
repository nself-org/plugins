package services

// Purpose: the registration point for the redactor. Every credential the
// engine generates is passed to SecretSink so job output and logs can mask it.
// Inputs:  set by the redaction package at init when it exists (P7-TRUST);
// until then the engine masks its own CLI output through the Funnel.
// Outputs: none. Constraints: set before any run; never log the value.

// SecretSink receives each generated credential. Nil means no redactor is
// registered.
var SecretSink func(secret string)
