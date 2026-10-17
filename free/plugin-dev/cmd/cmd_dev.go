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

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-plugin-dev/internal/clui"
	"github.com/nself-org/nself-plugin-dev/internal/links"
)

// entrypointSafeRe matches entrypoint paths composed solely of safe characters.
var entrypointSafeRe = regexp.MustCompile(`^[a-zA-Z0-9_./-]+$`)

// devData is the --json data of `dev`, printed when the watcher (or the
// debugger it launched) exits cleanly.
type devData struct {
	Name       string `json:"name"`
	PluginPath string `json:"plugin_path"`
	Linked     bool   `json:"linked"`
	Debug      bool   `json:"debug"`
	Entrypoint string `json:"entrypoint"`
}

// devStepErr is E776 for a failed watcher or debugger launch.
func devStepErr(err error) error {
	return stepErr(err, "the dev watcher or the debugger it launched exited with an error",
		"its own output is on stderr; install air or fswatch for the watcher, dlv for --debug")
}

func runDev(p *parsed) (any, error) {
	name := p.args[0]
	noLink := p.boolean("no-link")
	debug := p.boolean("debug")
	entrypoint := p.str("entrypoint")

	pluginPath, err := links.ResolvePluginPath(name)
	if err != nil {
		return nil, pluginNotFoundErr(err)
	}

	clui.Infof("Starting %s in development mode...", name)
	clui.Dimmedf("Plugin path: %s", pluginPath)

	linked := false
	if !noLink {
		clui.Infof("Auto-linking %s...", name)
		if linkErr := links.Link(pluginPath); linkErr != nil {
			clui.Warnf("Auto-link failed (%v); continuing without link", linkErr)
		} else {
			clui.Success("Plugin linked.")
			linked = true
		}
	}

	if debug {
		self := selfBin()
		clui.Infof("Launching debugger via %s debug %s", self, name)
		debugCmd := exec.Command(self, "debug", name)
		debugCmd.Stdout = os.Stdout
		debugCmd.Stderr = os.Stderr
		if err := debugCmd.Run(); err != nil {
			return nil, devStepErr(err)
		}
		return devData{Name: name, PluginPath: pluginPath, Linked: linked, Debug: true, Entrypoint: entrypoint}, nil
	}

	if !entrypointSafeRe.MatchString(entrypoint) {
		return nil, usageErr(fmt.Errorf("plugin dev: entrypoint %q contains invalid characters (allowed: [a-zA-Z0-9_./-])", entrypoint),
			"entrypoint contains invalid characters (allowed: [a-zA-Z0-9_./-])")
	}
	cleanEntrypoint := filepath.Clean(entrypoint)
	if strings.Contains(cleanEntrypoint, "..") {
		return nil, usageErr(fmt.Errorf("plugin dev: entrypoint %q must not contain path traversal sequences", entrypoint),
			"entrypoint must not contain path traversal sequences")
	}
	resolvedEntrypoint := filepath.Join(pluginPath, cleanEntrypoint)
	if !strings.HasPrefix(resolvedEntrypoint, pluginPath+string(filepath.Separator)) &&
		resolvedEntrypoint != pluginPath {
		return nil, usageErr(fmt.Errorf("plugin dev: entrypoint %q must be within the plugin directory %q", entrypoint, pluginPath),
			"entrypoint must be within the plugin directory")
	}

	watchScript, err := findDevWatchScript()
	if err != nil {
		return nil, devStepErr(fmt.Errorf("dev-watch.sh not found: %w\nInstall the plugin SDK or run `go install github.com/air-verse/air@latest`", err))
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
			return devData{Name: name, PluginPath: pluginPath, Linked: linked, Entrypoint: cleanEntrypoint}, nil // SIGINT: clean exit
		}
		return nil, devStepErr(fmt.Errorf("dev-watch exited: %w", err))
	}
	return devData{Name: name, PluginPath: pluginPath, Linked: linked, Entrypoint: cleanEntrypoint}, nil
}

// pluginNotFoundErr is E770: no plugin by that name in cwd or the links file.
func pluginNotFoundErr(err error) error {
	return coded(err, CodeNotFound, output.ClassUser, "the plugin was not found: run from its directory or link it first",
		"run nself plugin-dev link <path>, or run the command from the plugin directory")
}
