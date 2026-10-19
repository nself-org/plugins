package gvisor

// Purpose: decide whether this host can run a job under gVisor right now.
// Inputs:  the container profile's probe (rootless, limits, runtime), the daemon's runtime
// table and one real start of the pinned probe image under runsc.
// Outputs: an isolation.Availability at level sandboxed-container, with the runsc version and
// platform as facts, or a reason.
// Constraints: a runsc binary or a runtime entry is never availability by itself; the level is
// available only when the probe container started under runsc and printed the gVisor banner.
// Any failure lowers to unavailable and never to plain container (Epic D7).

import (
	"context"
	"os"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
)

// Starter starts the probe container and asks runsc for its version.
type Starter interface {
	// Banner runs the probe image under the named runtime and returns its `dmesg` output.
	Banner(ctx context.Context, binary, runtime, image string) ([]byte, error)
	// Version returns the first line of `runsc --version`.
	Version(ctx context.Context, path string) (string, error)
}

type execStarter struct{}

func (execStarter) Banner(ctx context.Context, binary, runtime, image string) ([]byte, error) {
	return exec.RuntimeProbeRun(ctx, binary, runtime, image)
}

func (execStarter) Version(ctx context.Context, path string) (string, error) {
	return exec.RuntimeToolVersion(ctx, path)
}

func hasKVM() bool {
	_, err := os.Stat("/dev/kvm")
	return err == nil
}

// bannerSeen reports whether dmesg output came from the gVisor Sentry, which logs "Starting gVisor".
func bannerSeen(out []byte) bool {
	return strings.Contains(strings.ToLower(string(out)), "starting gvisor")
}

// unavailable builds a probe result that carries the install hint where it helps.
func unavailable(reason string, facts map[string]string) isolation.Availability {
	if facts == nil {
		facts = map[string]string{}
	}
	facts["hint"] = InstallHint
	return isolation.Availability{Reason: reason, Facts: facts}
}

func (p *Profile) probeFresh(ctx context.Context) isolation.Availability {
	base := p.base.Probe(ctx)
	if !base.Available() {
		return isolation.Availability{Reason: base.Reason}
	}
	binary := base.Facts["runtime"]
	if binary != "docker" {
		// podman keeps its OCI runtimes in containers.conf, which gives no table to read or judge
		return unavailable(isolation.ReasonRuntimeMissing, map[string]string{"runtime": binary, "detail": "gVisor is supported through docker only"})
	}
	out, err := p.opts.Runner.Query(ctx, binary, "info", "--format", "{{json .Runtimes}}")
	if err != nil {
		if exec.IsRuntimeMissing(err) {
			return unavailable(isolation.ReasonRuntimeMissing, nil)
		}
		return unavailable(isolation.ReasonProbeFailed, map[string]string{"detail": "docker info failed"})
	}
	entry, ok, err := parseEntry(out)
	if err != nil {
		return unavailable(isolation.ReasonProbeFailed, map[string]string{"detail": "docker runtime table is not JSON"})
	}
	if !ok {
		return unavailable(isolation.ReasonRuntimeMissing, map[string]string{"detail": "runsc is not registered with docker"})
	}
	platform, flag := checkArgs(entry.Args)
	if flag != "" {
		return unavailable(ReasonRuntimeFlagDenied, map[string]string{"detail": "runsc runtime argument " + flag + " is refused"})
	}
	facts := map[string]string{"platform": platform}
	if reason, why := platformReason(platform, p.opts.HasKVM()); reason != "" {
		facts["detail"] = why
		return unavailable(reason, facts)
	}
	banner, err := p.opts.Starter.Banner(ctx, binary, RuntimeName, p.opts.Image)
	if err != nil {
		facts["detail"] = "the probe container did not start under runsc"
		return unavailable(isolation.ReasonProbeFailed, facts)
	}
	if !bannerSeen(banner) {
		facts["detail"] = "the probe container started but did not print the gVisor banner"
		return unavailable(isolation.ReasonProbeFailed, facts)
	}
	version, verr := p.opts.Starter.Version(ctx, entry.Path)
	if verr != nil || version == "" {
		version = "unknown" // the version is a fact, not the proof: the start above is
	}
	out2 := map[string]string{}
	for k, v := range base.Facts {
		out2[k] = v
	}
	out2["sandbox_runtime"], out2["runsc_version"], out2["platform"], out2["banner"] = RuntimeName, version, platform, "gvisor"
	return isolation.Availability{Level: "sandboxed-container", Facts: out2}
}
