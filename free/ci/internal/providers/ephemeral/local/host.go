package local

// Purpose: build the Provider for the host the process runs on, from the isolation profiles
//          the run package registered (container and gVisor on Linux, Tart `vm` on Apple silicon).
// Inputs:  an isolation.Registry (default isolation.Default) and the rest of Options.
// Outputs: a Provider over the matching Backend, or an E698 *Error: a Linux host with no container
//          profile, a macOS host without the Tart profile, any other OS.
// Constraints: a Linux host with docker or podman and no runsc gets container-level slots (the
//          gVisor probe decides at Probe time, never here); there is no fallback below container and no
//          second container or VM implementation.

import (
	"runtime"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// NewHost returns the provider for this host. reg may be nil (isolation.Default).
func NewHost(reg *isolation.Registry, o Options) (*Provider, error) {
	return newFor(runtime.GOOS, reg, o)
}

func newFor(goos string, reg *isolation.Registry, o Options) (*Provider, error) {
	if reg == nil {
		reg = isolation.Default
	}
	switch goos {
	case "linux":
		c := profile(reg, containerLevel)
		if c == nil {
			return nil, unavailable(ReasonNoAdapter, nil, "the container isolation profile is not registered")
		}
		o.Backend = NewContainerBackend(c, profile(reg, sandboxLevel), nil)
	case "darwin":
		v := profile(reg, vmLevel)
		if v == nil {
			return nil, unavailable(ReasonNoTart, nil, "the Tart vm isolation profile is not registered")
		}
		o.Backend = NewTartBackend(v)
	default:
		return nil, unavailable(ReasonUnsupport, nil, "no local sandbox on %s", goos)
	}
	return New(o)
}

// profile returns the registered profile of exactly this declared level, or nil.
func profile(reg *isolation.Registry, level model.Isolation) isolation.Profile {
	for _, p := range reg.Profiles() {
		if p.Level() == level {
			return p
		}
	}
	return nil
}
