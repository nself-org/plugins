package adhoc

// Purpose: the error codes of the ad-hoc job (Epic D14, E646-E647) and the
// constructors that carry them.
// E646 there is no usable isolation for the job: Docker (or Podman) is absent
// and --isolation process was not given.
// E647 the trust class or the requested level needs an isolation level this
// host cannot deliver (an untrusted source needs the sandboxed-container
// level); nothing runs and the level is never lowered.
// Constraints: registered through model.Register, never a panic; both are
// infra codes (the host cannot provide the level), so the CLI exits 2.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// The codes this package raises.
const (
	CodeNoIsolation    = "E646"
	CodeTrustIsolation = "E647"
)

func init() {
	for _, c := range []model.Code{
		{ID: CodeNoIsolation, Class: "infra", Summary: "no isolation is available for the ad-hoc job",
			Fix: "Start Docker (or Podman) to run the job in a container, or pass --isolation process to run it on this host with host networking"},
		{ID: CodeTrustIsolation, Class: "infra", Summary: "the trust class needs an isolation level this host cannot provide",
			Fix: "Run the job on a host with the sandboxed-container level (gVisor), or drop --trust untrusted if the command is your own"},
	} {
		model.Register(c)
	}
}

func noIsolation(format string, a ...any) error {
	return &cirun.Error{Code: CodeNoIsolation, Message: fmt.Sprintf(format, a...)}
}

func trustIsolation(format string, a ...any) error {
	return &cirun.Error{Code: CodeTrustIsolation, Message: fmt.Sprintf(format, a...)}
}

// usage is a refused option combination (E627, the run package's option code).
func usage(format string, a ...any) error {
	return &cirun.Error{Code: cirun.CodeOptions, Message: fmt.Sprintf(format, a...)}
}

// UsageError is a refused option combination for callers outside the package.
func UsageError(format string, a ...any) error { return usage(format, a...) }
