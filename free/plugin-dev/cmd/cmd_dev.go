package main

// Purpose: `dev <name>`: link the plugin and run its hot-reload watcher. Port of
// core cmd/commands/plugin_dev.go.
// Inputs:  a plugin name or directory; --no-link, --debug, --entrypoint.
// Outputs: the watcher's output; ~/.nself/plugin-links.json (auto-link);
// /tmp/nself-dev-watch.sh when no SDK devkit script exists (as core).
// Constraints: the entrypoint must match [a-zA-Z0-9_./-], hold no "..", and stay
// inside the plugin directory; the watcher runs with NSELF_PLUGIN_DEV=1 and the
// plugin directory as cwd, as in core. `--debug` re-executes this binary as
// `debug <name>` (core re-executes itself as `plugin debug <name>`).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nself-org/nself-plugin-dev/internal/clui"
	"github.com/nself-org/nself-plugin-dev/internal/links"
)

// entrypointSafeRe matches entrypoint paths composed solely of safe characters.
var entrypointSafeRe = regexp.MustCompile(`^[a-zA-Z0-9_./-]+$`)

func runDev(p *parsed) error {
	name := p.args[0]
	noLink := p.boolean("no-link")
	debug := p.boolean("debug")
	entrypoint := p.str("entrypoint")

	pluginPath, err := links.ResolvePluginPath(name)
	if err != nil {
		return err
	}

	clui.Infof("Starting %s in development mode...", name)
	clui.Dimmedf("Plugin path: %s", pluginPath)

	if !noLink {
		clui.Infof("Auto-linking %s...", name)
		if linkErr := links.Link(pluginPath); linkErr != nil {
			clui.Warnf("Auto-link failed (%v); continuing without link", linkErr)
		} else {
			clui.Success("Plugin linked.")
		}
	}

	if debug {
		self := selfBin()
		clui.Infof("Launching debugger via %s debug %s", self, name)
		debugCmd := exec.Command(self, "debug", name)
		debugCmd.Stdout = os.Stdout
		debugCmd.Stderr = os.Stderr
		return debugCmd.Run()
	}

	if !entrypointSafeRe.MatchString(entrypoint) {
		return fmt.Errorf("plugin dev: entrypoint %q contains invalid characters (allowed: [a-zA-Z0-9_./-])", entrypoint)
	}
	cleanEntrypoint := filepath.Clean(entrypoint)
	if strings.Contains(cleanEntrypoint, "..") {
		return fmt.Errorf("plugin dev: entrypoint %q must not contain path traversal sequences", entrypoint)
	}
	resolvedEntrypoint := filepath.Join(pluginPath, cleanEntrypoint)
	if !strings.HasPrefix(resolvedEntrypoint, pluginPath+string(filepath.Separator)) &&
		resolvedEntrypoint != pluginPath {
		return fmt.Errorf("plugin dev: entrypoint %q must be within the plugin directory %q", entrypoint, pluginPath)
	}

	watchScript, err := findDevWatchScript()
	if err != nil {
		return fmt.Errorf("dev-watch.sh not found: %w\nInstall the plugin SDK or run `go install github.com/air-verse/air@latest`", err)
	}

	clui.Dimmedf("Using watch script: %s", watchScript)
	clui.Dimmedf("Entrypoint: %s", cleanEntrypoint)
	clui.Info("Watching for changes — press Ctrl+C to stop.")

	devCmd := exec.Command("bash", watchScript, cleanEntrypoint)
	devCmd.Dir = pluginPath
	devCmd.Stdout = os.Stdout
	devCmd.Stderr = os.Stderr
	devCmd.Env = append(os.Environ(), "NSELF_PLUGIN_DEV=1")

	if err := devCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 130 {
			return nil // SIGINT: clean exit
		}
		return fmt.Errorf("dev-watch exited: %w", err)
	}
	return nil
}
