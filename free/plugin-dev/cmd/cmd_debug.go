package main

// Purpose: `debug <name>`: build the plugin with debug flags and run it under a
// headless dlv. Port of core cmd/commands/plugin_debug.go.
// Inputs:  a plugin name or directory; --port (1024-65535) or auto from
// 2345-2399; --port-only.
// Outputs: the VS Code launch.json snippet, the build and dlv output; a binary at
// $TMPDIR/nself-debug-<name>.
// Constraints: dlv must be on PATH; the debugger listens on the given port only.

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/nself-org/nself-plugin-dev/internal/clui"
	"github.com/nself-org/nself-plugin-dev/internal/links"
)

// debugData is the --json data of `debug`: the port (and listen address) chosen,
// plus, unless --port-only, the plugin directory and the debug binary.
type debugData struct {
	Name       string `json:"name"`
	Port       int    `json:"port"`
	Address    string `json:"address"`
	PluginPath string `json:"plugin_path,omitempty"`
	Binary     string `json:"binary,omitempty"`
}

// debugStepErr is E776 for a failed debug build, a missing dlv or a dlv failure.
func debugStepErr(err error) error {
	return stepErr(err, "the debug build or the dlv debugger failed",
		"install dlv (go install github.com/go-delve/delve/cmd/dlv@latest); the tool output is on stderr")
}

func runDebug(p *parsed) (any, error) {
	name := p.args[0]
	port, err := resolveDebugPort(p.integer("port"))
	if err != nil {
		return nil, err
	}
	addr := fmt.Sprintf(":%d", port)
	if p.boolean("port-only") {
		fmt.Printf(":%d\n", port)
		return debugData{Name: name, Port: port, Address: addr}, nil
	}

	if _, err := exec.LookPath("dlv"); err != nil {
		return nil, debugStepErr(fmt.Errorf("dlv not found: install with `go install github.com/go-delve/delve/cmd/dlv@latest`"))
	}

	pluginPath, err := links.ResolvePluginPath(name)
	if err != nil {
		return nil, pluginNotFoundErr(err)
	}

	binaryPath := filepath.Join(os.TempDir(), "nself-debug-"+name)
	clui.Infof("Building %s for debug...", name)
	buildCmd := exec.Command("go", "build", "-gcflags=all=-N -l", "-o", binaryPath, "./cmd")
	buildCmd.Dir = pluginPath
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return nil, debugStepErr(fmt.Errorf("building plugin for debug: %w", err))
	}

	clui.Successf("Plugin built at %s", binaryPath)
	clui.Infof("Starting dlv on :%d...", port)
	printLaunchJSON(name, port)

	dlvCmd := exec.Command("dlv",
		"exec", binaryPath,
		"--headless",
		"--listen", fmt.Sprintf(":%d", port),
		"--api-version", "2",
		"--accept-multiclient",
	)
	dlvCmd.Dir = pluginPath
	dlvCmd.Stdout = os.Stdout
	dlvCmd.Stderr = os.Stderr
	dlvCmd.Env = append(os.Environ(), "NSELF_PLUGIN_DEV=1")

	if err := dlvCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 130 {
			return debugData{Name: name, Port: port, Address: addr, PluginPath: pluginPath, Binary: binaryPath}, nil // SIGINT
		}
		return nil, debugStepErr(fmt.Errorf("dlv exited: %w", err))
	}
	return debugData{Name: name, Port: port, Address: addr, PluginPath: pluginPath, Binary: binaryPath}, nil
}

// resolveDebugPort returns the manually specified port or the first free port in 2345-2399.
func resolveDebugPort(manual int) (int, error) {
	if manual != 0 {
		if manual < 1024 || manual > 65535 {
			return 0, usageErr(fmt.Errorf("invalid port %d: must be 1024-65535", manual), "invalid port: must be 1024-65535")
		}
		return manual, nil
	}
	for port := 2345; port <= 2399; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = ln.Close()
			return port, nil
		}
	}
	return 0, debugStepErr(fmt.Errorf("no available debug port in range 2345-2399 — use --port to specify one manually"))
}

// printLaunchJSON prints a VS Code launch.json snippet to stdout.
func printLaunchJSON(pluginName string, port int) {
	clui.Dimmed("--- VS Code launch.json snippet ---")
	fmt.Printf(`{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Attach to %s",
      "type": "go",
      "request": "attach",
      "mode": "remote",
      "remotePath": "${workspaceFolder}",
      "port": %d,
      "host": "127.0.0.1"
    }
  ]
}
`, pluginName, port)
	clui.Dimmed("---")
	clui.Dimmedf("GoLand: Run > Attach to Process > Remote... > host=127.0.0.1, port=%d", port)
	clui.Dimmed("---")
}
