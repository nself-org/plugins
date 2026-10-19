package main

// Purpose: `nself ci artifacts tools install [--json]` (P7-CACHE-08, Epic
// CACHE D13): place the pinned syft at $NSELF_CI_HOME/tools/syft-<version>/syft
// so a process-level release job can inventory its artifacts. The E718
// tool_missing and tool_version remediations name this command.
// Inputs:  argv after `tools`; env NSELF_CI_HOME (default ~/.nself/ci).
// Outputs: key/value lines or one v1 JSON envelope; exit 0, 1 (usage) or 2.
// Constraints: explicit only (no job ever downloads a tool); the sha256 comes
// from the `syft` row of versions.yaml and a mismatch installs nothing
// (E718 tool_checksum); the downloaded binary is never run here; installing
// twice is a no-op.

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/sbom"
)

var (
	// toolsPin and toolsClient are replaced by tests (a fixture server).
	toolsPin    = sbom.EmbeddedPin
	toolsClient *http.Client
)

func init() {
	registerArtifactsSub("tools", artifactsTools)
}

const toolsUsage = "usage: nself ci artifacts tools install [--json]"

func artifactsTools(args []string) int {
	fs, pos, err := parseArtifacts("tools", args, nil)
	if err != nil || len(pos) != 1 || pos[0] != "install" {
		fmt.Fprintln(os.Stderr, toolsUsage)
		return 1
	}
	asJSON := fs.Lookup("json").Value.String() == "true"
	const command = "ci artifacts tools install"
	home, err := toolsHome()
	if err != nil {
		return artifactsExit(sbom.Failf(sbom.ReasonToolMissing, "%v", err), command, asJSON)
	}
	pin, err := toolsPin()
	if err != nil {
		return artifactsExit(err, command, asJSON)
	}
	res, err := sbom.Install(context.Background(), sbom.InstallOptions{Home: home, Pin: pin, Client: toolsClient})
	if err != nil {
		return artifactsExit(err, command, asJSON)
	}
	if asJSON {
		return writeArtifactsJSON(command, res)
	}
	state := "installed"
	if res.Already {
		state = "already installed"
	}
	fmt.Printf("%s %s %s\npath %s\nsha256 %s\n", sbom.ToolName, res.Version, state, res.Path, res.SHA256)
	return 0
}

// toolsHome is NSELF_CI_HOME, or the executor's default under the user home.
func toolsHome() (string, error) {
	if h := os.Getenv("NSELF_CI_HOME"); h != "" {
		return h, nil
	}
	u, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(u, ".nself", "ci"), nil
}
