package sbom

// Purpose: the syft argv for one release artifact (Epic CACHE D13).
// Inputs:  a Target (name, workdir-relative path, kind) and the syft path.
// Outputs: Cmd with the argv and the two relative output paths.
// Constraints: pure; the artifact name becomes a file name, so it is a plain
// identifier; a path is workdir-relative (no absolute path, no ..); outputs
// land in OutDir, CycloneDX 1.6 and SPDX 2.3 side by side; syft's update
// check is switched off (no network at run time).

import (
	"path"
	"path/filepath"
	"regexp"
)

// OutDir is where SBOM files land, relative to the job workdir.
const OutDir = ".nself-ci/sbom"

// Output formats syft is asked for.
const (
	FormatCycloneDX = "cyclonedx-json@1.6"
	FormatSPDX      = "spdx-json@2.3"
)

// Source kinds of a Target.
type SourceKind string

const (
	SourceFile  SourceKind = "file"        // one file (binary, package tarball)
	SourceDir   SourceKind = "dir"         // a directory tree
	SourceImage SourceKind = "oci-archive" // an image archive
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Target is one release artifact to inventory.
type Target struct {
	Name string     // artifact name; also the output file stem
	Path string     // workdir-relative path of the artifact
	Kind SourceKind // file, dir or oci-archive
	// AllowEmpty accepts an SBOM with no components; without it syft cataloguing
	// nothing (an artifact it cannot read) is a failure, not an empty inventory.
	AllowEmpty bool
}

// Cmd is one command the Runner executes in the job workdir.
type Cmd struct {
	Argv    []string
	Env     map[string]string
	Collect []string // workdir-relative files to read back after exit 0
	// NoSource runs the command in an empty workspace (a version probe needs no source).
	NoSource bool
}

// Outputs returns the CycloneDX and SPDX file paths (workdir-relative) of name.
func Outputs(name string) (cdx, spdx string) {
	return path.Join(OutDir, name+".cdx.json"), path.Join(OutDir, name+".spdx.json")
}

// Check refuses a Target that cannot be turned into a safe command.
func (t Target) Check() error {
	if !nameRe.MatchString(t.Name) {
		return Failf(ReasonInvalid, "artifact name %q is not a plain identifier", t.Name)
	}
	if t.Path == "" || filepath.IsAbs(t.Path) || !filepath.IsLocal(t.Path) {
		return Failf(ReasonInvalid, "artifact %q path %q is not inside the workdir", t.Name, t.Path)
	}
	switch t.Kind {
	case SourceFile, SourceDir, SourceImage:
	default:
		return Failf(ReasonInvalid, "artifact %q has unknown source kind %q", t.Name, t.Kind)
	}
	return nil
}

// Argv builds the syft command for t using the syft at syftPath.
func Argv(syftPath string, t Target) (Cmd, error) {
	if err := t.Check(); err != nil {
		return Cmd{}, err
	}
	cdx, spdx := Outputs(t.Name)
	return Cmd{
		Argv: []string{syftPath, "scan", string(t.Kind) + ":" + filepath.ToSlash(filepath.Clean(t.Path)),
			"--source-name", t.Name,
			"-o", FormatCycloneDX + "=" + cdx,
			"-o", FormatSPDX + "=" + spdx,
			"-q"},
		Env:     map[string]string{"SYFT_CHECK_FOR_APP_UPDATE": "false"},
		Collect: []string{cdx, spdx},
	}, nil
}
