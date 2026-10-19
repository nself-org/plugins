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
// The installed file must also hash to the pin's binary_sha256 for this os/arch
// (D-1024), else tool_checksum before anything runs. The hash is taken from the
// file as read here; the runner then executes it by path, so a same-user swap
// between the check and the exec is narrowed (the directories are 0755 and
// user-owned) but not closed.

import (
	"context"
	"errors"
	"io/fs"
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

var versionOK = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+_-]*$`)

// ResolveHome returns the absolute CI home: home when set, else NSELF_CI_HOME,
// else ~/.nself/ci (what the executor uses). A relative path is refused: it
// would be looked up inside whatever directory the job runs in, where a repo
// could plant a tool.
func ResolveHome(home string) (string, error) {
	if home == "" {
		home = os.Getenv("NSELF_CI_HOME")
	}
	if home == "" {
		u, err := os.UserHomeDir()
		if err != nil {
			return "", Failf(ReasonToolMissing, "cannot find the CI home: %v", err)
		}
		home = filepath.Join(u, ".nself", "ci")
	}
	if !filepath.IsAbs(home) {
		return "", Failf(ReasonToolMissing, "the CI home %q is not an absolute path", home)
	}
	return filepath.Clean(home), nil
}

// Check refuses a pin whose version could not name a directory under tools/.
func (p Pin) Check() error {
	if !versionOK.MatchString(p.Version) {
		return Failf(ReasonToolMissing, "the %s pin version %q is not a plain version", ToolName, p.Version)
	}
	return nil
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
	res, err := r.Run(ctx, Cmd{Argv: []string{syft, "version"}, Env: map[string]string{"SYFT_CHECK_FOR_APP_UPDATE": "false"}, NoSource: true})
	if err != nil {
		if notFound(err, syft) {
			return "", Failf(ReasonToolMissing, "%s did not run: %v", ToolName, err)
		}
		// an infrastructure failure of the runner is not "the tool is missing"
		return "", Failf(ReasonInvalid, "the runner could not run %s: %v", ToolName, err)
	}
	if res.Exit == 126 || res.Exit == 127 {
		return "", Failf(ReasonToolMissing, "%s version exited %d (not found or not executable)", ToolName, res.Exit)
	}
	if res.Exit != 0 {
		return "", Failf(ReasonToolVersion, "%s version exited %d", ToolName, res.Exit)
	}
	v, ok := ParseVersion(res.Output)
	if !ok {
		return "", Failf(ReasonToolVersion, "cannot read the %s version from its output", ToolName)
	}
	return v, nil
}

// Resolve returns the syft path a job of the given mode must run.
func Resolve(ctx context.Context, r Runner, home string, mode Mode, pin Pin) (string, error) {
	if err := pin.Check(); err != nil {
		return "", err
	}
	if mode == ModeContainer {
		return checked(ctx, r, ToolName, pin, "the toolchain image carries")
	}
	h, err := ResolveHome(home)
	if err != nil {
		return "", err
	}
	want := ToolPath(h, pin.Version)
	switch err := trusted(want); {
	case err == nil:
		if err := verifyBinary(want, pin, runtime.GOOS, runtime.GOARCH); err != nil {
			return "", err
		}
		return checked(ctx, r, want, pin, "the installed")
	case !errors.Is(err, fs.ErrNotExist):
		return "", Failf(ReasonToolMissing, "%s is not a trustworthy install (%v); run `%s`", want, err, InstallCommand)
	}
	// A syft on PATH is never run here (it is not the pinned install, and running it
	// would execute a binary nothing authenticated); its presence is reported only.
	if onPath, ok := lookPath(ToolName); ok {
		return "", Failf(ReasonToolVersion, "%s on PATH (%s) is not the pinned %s %s; process-level jobs use only %s, which is not installed; run `%s`", ToolName, onPath, ToolName, pin.Version, want, InstallCommand)
	}
	return "", Failf(ReasonToolMissing, "%s %s is not installed at %s; run `%s`", ToolName, pin.Version, want, InstallCommand)
}
