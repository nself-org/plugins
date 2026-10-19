package sbom

// Purpose: find the pinned syft and prove its version (Epic CACHE D13).
// Inputs:  the `syft` row of versions.yaml (Pin), NSELF_CI_HOME, a Runner.
// Outputs: the path to run; E718 tool_missing / tool_version otherwise.
// Constraints: a process-level job uses ONLY $NSELF_CI_HOME/tools/syft-<version>/syft
// (placed by the explicit install command, never at run time); a container job
// uses the syft the toolchain image carries, on PATH; either way the probed
// version must equal the pin. A process-level job never executes a PATH syft:
// when the pinned install is absent and a PATH syft exists, the job fails
// tool_version naming the install command; with none it fails tool_missing.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

// ToolName is the row name in versions.yaml and the binary name.
const ToolName = "syft"

// Pin is the syft row: the version every SBOM must name and the checked sources.
type Pin struct {
	Version string
	Sources []toolchain.Source
}

// PinFrom reads the syft row of a versions table.
func PinFrom(v toolchain.Versions) (Pin, error) {
	row, ok := v.Row(ToolName)
	if !ok || row.Version == "" {
		return Pin{}, Failf(ReasonToolMissing, "versions.yaml has no %s row", ToolName)
	}
	return Pin{Version: row.Version, Sources: row.Sources}, nil
}

// EmbeddedPin is the pin compiled into the binary.
func EmbeddedPin() (Pin, error) {
	v, err := toolchain.Embedded()
	if err != nil {
		return Pin{}, err
	}
	return PinFrom(v)
}

// Source returns the download source for an os/arch.
func (p Pin) Source(goos, goarch string) (toolchain.Source, bool) {
	for _, s := range p.Sources {
		if s.OS == goos && s.Arch == goarch {
			return s, true
		}
	}
	return toolchain.Source{}, false
}

// HostSource returns the source for the running os/arch.
func (p Pin) HostSource() (toolchain.Source, bool) { return p.Source(runtime.GOOS, runtime.GOARCH) }

// Mode says where a job's syft comes from.
type Mode int

const (
	// ModeProcess is a process-level job: only the installed syft counts.
	ModeProcess Mode = iota
	// ModeContainer is a container job: the image's syft on PATH counts.
	ModeContainer
)

// ToolPath is where `artifacts tools install` places syft for version.
func ToolPath(home, version string) string {
	return filepath.Join(home, "tools", ToolName+"-"+version, ToolName)
}

var versionRe = regexp.MustCompile(`(?m)^Version:\s+v?(\S+)\s*$`)

// ParseVersion extracts the version from `syft version` output.
func ParseVersion(out string) (string, bool) {
	m := versionRe.FindStringSubmatch(out)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// Probe runs `<syft> version` and returns the version it prints.
func Probe(ctx context.Context, r Runner, syft string) (string, error) {
	if r == nil {
		return "", Failf(ReasonToolMissing, "no runner to probe %s with", ToolName)
	}
	res, err := r.Run(ctx, Cmd{Argv: []string{syft, "version"}, Env: map[string]string{"SYFT_CHECK_FOR_APP_UPDATE": "false"}})
	if err != nil {
		return "", Failf(ReasonToolMissing, "%s did not run: %v", ToolName, err)
	}
	if res.Exit != 0 {
		return "", Failf(ReasonToolMissing, "%s version exited %d", ToolName, res.Exit)
	}
	v, ok := ParseVersion(res.Output)
	if !ok {
		return "", Failf(ReasonToolVersion, "cannot read the %s version from its output", ToolName)
	}
	return v, nil
}

// Resolve returns the syft path a job of the given mode must run.
func Resolve(ctx context.Context, r Runner, home string, mode Mode, pin Pin) (string, error) {
	if mode == ModeContainer {
		return checked(ctx, r, ToolName, pin, "the toolchain image carries")
	}
	want := ToolPath(home, pin.Version)
	if isExecutable(want) {
		return checked(ctx, r, want, pin, "the installed")
	}
	// A syft on PATH is never run here (it is not the pinned install, and running it
	// would execute a binary nothing authenticated); its presence is reported only.
	if onPath, ok := lookPath(ToolName); ok {
		return "", Failf(ReasonToolVersion, "%s on PATH (%s) is not the pinned %s %s; process-level jobs use only %s, which is not installed; run `%s`", ToolName, onPath, ToolName, pin.Version, want, InstallCommand)
	}
	return "", Failf(ReasonToolMissing, "%s %s is not installed at %s; run `%s`", ToolName, pin.Version, want, InstallCommand)
}

func checked(ctx context.Context, r Runner, syft string, pin Pin, what string) (string, error) {
	v, err := Probe(ctx, r, syft)
	if err != nil {
		if IsReason(err, ReasonToolMissing) {
			return "", Failf(ReasonToolMissing, "%s %s syft %s did not run (%v); run `%s`", ToolName, what, pin.Version, err, InstallCommand)
		}
		return "", err
	}
	if v != pin.Version {
		return "", Failf(ReasonToolVersion, "%s is %s but the pin is %s; run `%s` to place the pinned %s", what+" "+ToolName, v, pin.Version, InstallCommand, ToolName)
	}
	return syft, nil
}

func isExecutable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular() && st.Mode()&0111 != 0
}

// lookPath searches PATH like a shell (os/exec is the exec funnel's alone).
func lookPath(name string) (string, bool) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		if p := filepath.Join(dir, name); isExecutable(p) {
			return p, true
		}
	}
	return "", false
}
