package services

// Purpose: the error codes of the service-dependency engine (Epic D14,
// E636-E639) and the typed error that carries one.
// E636 a service could not be provisioned or torn down; E637 Docker is not
// available for a service-dependent job; E638 the nself CLI lacks the
// commands the engine calls; E639 an unknown service or component name.
// Constraints: registered through model.Register, never a panic; every error
// formats as "E###: message" so run.CodeOf finds the code.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// The codes this package raises.
const (
	CodeProvision = "E636"
	CodeNoDocker  = "E637"
	CodeNoCLI     = "E638"
	CodeUnknown   = "E639"
)

func init() {
	for _, c := range []model.Code{
		{ID: CodeProvision, Class: "infra", Summary: "service provisioning or teardown failed", Fix: "Re-run the job; list leftovers with `docker compose ls --all` and remove any nselfci- project"},
		{ID: CodeNoDocker, Class: "infra", Summary: "Docker is required for service dependencies", Fix: "Start Docker (or Colima) and re-run, or run a job that declares no services"},
		{ID: CodeNoCLI, Class: "infra", Summary: "the nself CLI does not offer the service commands", Fix: "Run `nself update`"},
		{ID: CodeUnknown, Class: "usage", Summary: "unknown service or component", Fix: "List the valid names with `nself config services list --json`"},
	} {
		model.Register(c)
	}
}

// Error is a service-engine failure with its code.
type Error struct {
	Code    string
	Message string

	portTaken bool // the CLI output said the host port is in use
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func fail(code, format string, a ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, a...)}
}
