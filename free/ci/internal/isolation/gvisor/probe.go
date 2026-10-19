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
	"regexp"
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

// execStarter starts through internal/exec, the only package that may start a process.
type execStarter struct{}

func (execStarter) Banner(ctx context.Context, binary, runtime, image string) ([]byte, error) {
	return exec.RuntimeProbeRun(ctx, binary, runtime, image)
}

func (execStarter) Version(ctx context.Context, path string) (string, error) {
	return exec.RuntimeToolVersion(ctx, path)
}

// hasKVM reports whether /dev/kvm exists (the kvm platform needs it).
func hasKVM() bool {
	_, err := os.Stat("/dev/kvm")
	return err == nil
}

// banner is gVisor's first log line, optionally behind a "[ time ]" stamp. It must be a whole line:
// a host kernel log line that merely mentions a process called "Starting gVisor" is not it.
var bannerLine = regexp.MustCompile(`(?m)^(\[[^\]]*\]\s*)?Starting gVisor\.\.\.\s*$`)

// bannerSeen reports whether dmesg output came from the gVisor Sentry.
func bannerSeen(out []byte) bool { return bannerLine.Match(out) }

// unavailable builds a probe result that carries the install hint where it helps.
func unavailable(reason string, facts map[string]string) isolation.Availability {
	if facts == nil {
		facts = map[string]string{}
	}
	facts["hint"] = InstallHint
	return isolation.Availability{Reason: reason, Facts: facts}
}

// inspected is what the cheap checks found: the runtime to start, runsc's entry and platform.
type inspected struct {
	binary, platform string
	entry            runtimeEntry
	base             map[string]string
}

// key identifies the runtime configuration a start result belongs to: a daemon reload that
// changes the runsc entry makes the cached start stale at once.
func (in inspected) key() string {
	return in.binary + "|" + in.entry.Path + "|" + strings.Join(in.entry.Args, "\x00")
}

// inspect runs the checks that cost one query each and run on every call (the container
// profile's probe, the runtime table, the deny list and the platform). ok is false when the
// level is unavailable; av then says why.
func (p *Profile) inspect(ctx context.Context) (in inspected, av isolation.Availability, ok bool) {
	base := p.base.Probe(ctx)
	if !base.Available() {
		return in, isolation.Availability{Reason: base.Reason}, false
	}
	in.binary, in.base = base.Facts["runtime"], base.Facts
	if in.binary != "docker" {
		// podman keeps its OCI runtimes in containers.conf, which gives no table to read or judge
		return in, unavailable(isolation.ReasonRuntimeMissing, map[string]string{"runtime": in.binary, "detail": "gVisor is supported through docker only"}), false
	}
	out, err := p.opts.Runner.Query(ctx, in.binary, "info", "--format", "{{json .Runtimes}}")
	if err != nil {
		if exec.IsRuntimeMissing(err) {
			return in, unavailable(isolation.ReasonRuntimeMissing, nil), false
		}
		return in, unavailable(isolation.ReasonProbeFailed, map[string]string{"detail": "docker info failed"}), false
	}
	entry, found, foreign, err := parseEntry(out)
	if err != nil {
		return in, unavailable(isolation.ReasonProbeFailed, map[string]string{"detail": "docker runtime table is not JSON"}), false
	}
	if !found {
		return in, unavailable(isolation.ReasonRuntimeMissing, map[string]string{"detail": "runsc is not registered with docker"}), false
	}
	if foreign != "" {
		return in, unavailable(ReasonRuntimeFlagDenied, map[string]string{"detail": "the runsc runtime entry has a key (" + foreign + ") whose settings this profile cannot read"}), false
	}
	if entry.Path != "" && !isRunsc(entry.Path) {
		return in, unavailable(isolation.ReasonProbeFailed, map[string]string{"detail": "the runtime named runsc points at " + entry.Path + ", which is not the runsc binary"}), false
	}
	platform, flag := checkArgs(entry.Args)
	if flag != "" {
		return in, unavailable(ReasonRuntimeFlagDenied, map[string]string{"detail": "runsc runtime argument " + flag + " is refused"}), false
	}
	if reason, why := platformReason(platform, p.opts.HasKVM()); reason != "" {
		return in, unavailable(reason, map[string]string{"platform": platform, "detail": why}), false
	}
	in.entry, in.platform = entry, platform
	return in, isolation.Availability{}, true
}

// start proves the level by really starting the probe image under runsc and reading the banner.
func (p *Profile) start(ctx context.Context, in inspected) isolation.Availability {
	facts := map[string]string{"platform": in.platform}
	dmesg, err := p.opts.Starter.Banner(ctx, in.binary, RuntimeName, p.opts.Image)
	if err != nil {
		facts["detail"] = "the probe container did not start under runsc"
		return unavailable(isolation.ReasonProbeFailed, facts)
	}
	if !bannerSeen(dmesg) {
		facts["detail"] = "the probe container started but did not print the gVisor banner"
		return unavailable(isolation.ReasonProbeFailed, facts)
	}
	version := "unknown" // the version is a fact, not the proof: the start above is
	if v, verr := p.opts.Starter.Version(ctx, in.entry.Path); verr == nil && v != "" {
		version = v
	}
	out := map[string]string{}
	for k, v := range in.base {
		out[k] = v
	}
	out["sandbox_runtime"], out["runsc_version"], out["platform"], out["banner"] = RuntimeName, version, in.platform, "gvisor"
	return isolation.Availability{Level: "sandboxed-container", Facts: out}
}

// clone copies the facts map, so a caller that edits its result never touches the cache.
func clone(av isolation.Availability) isolation.Availability {
	if av.Facts != nil {
		facts := make(map[string]string, len(av.Facts))
		for k, v := range av.Facts {
			facts[k] = v
		}
		av.Facts = facts
	}
	return av
}
