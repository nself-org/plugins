// Package links is the plugin shadow registry (~/.nself/plugin-links.json) and
// the plugin-directory resolution the author commands share.
//
// Purpose: temporary copy of cli cmd/commands/plugin_link.go (registry file,
// name resolution, path validation) and plugin_dev.go resolvePluginPath, kept
// identical so `plugin-dev link|unlink|dev|debug|test` have the same file
// effects as core. Ports must not import cli internals (CANON D11); deleted at
// P7-SHIP-09.
// Inputs:  the HOME directory, the working directory and the registry file.
// Outputs: the links map; absolute plugin paths; errors worded as core words them.
// Constraints: the registry file is written 0600 under a 0750 ~/.nself; no
// path outside HOME/.nself is ever written; standard library only.
package links

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Path returns the path of the plugin shadow registry JSON file.
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", ".nself", "plugin-links.json")
	}
	return filepath.Join(home, ".nself", "plugin-links.json")
}

// Load reads plugin-links.json, returning an empty map on a missing file.
func Load() (map[string]string, error) {
	data, err := os.ReadFile(Path())
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading plugin-links.json: %w", err)
	}
	var links map[string]string
	if err := json.Unmarshal(data, &links); err != nil {
		return nil, fmt.Errorf("parsing plugin-links.json: %w", err)
	}
	return links, nil
}

// Save writes the links map to plugin-links.json (0600 permissions).
func Save(links map[string]string) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("creating .nself dir: %w", err)
	}
	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling plugin-links: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing plugin-links.json: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("chmod plugin-links.json: %w", err)
	}
	return nil
}

// Link registers localPath in the registry under its plugin name (the dev
// auto-link path).
func Link(localPath string) error {
	name, err := ResolveName(localPath)
	if err != nil {
		return err
	}
	links, err := Load()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return fmt.Errorf("resolving absolute path: %w", err)
	}
	links[name] = abs
	return Save(links)
}

// ResolveName reads plugin.yaml to find the plugin's declared name, falling
// back to the directory basename.
func ResolveName(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "name:") {
				name := strings.TrimSpace(strings.TrimPrefix(line, "name:"))
				name = strings.Trim(name, `"'`)
				if name != "" {
					return name, nil
				}
			}
		}
	}
	name := filepath.Base(dir)
	if name == "" || name == "." {
		return "", fmt.Errorf("cannot determine plugin name from path %q", dir)
	}
	return name, nil
}

// ValidatePath resolves the path, checks it is a directory that holds a
// plugin.yaml, and returns its absolute form.
func ValidatePath(rawPath string) (string, error) {
	abs, err := filepath.Abs(rawPath)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("path %q does not exist: %w", abs, err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("path %q is not a directory", abs)
	}
	if _, err := os.Stat(filepath.Join(abs, "plugin.yaml")); err != nil {
		return "", fmt.Errorf("path %q does not contain plugin.yaml — not a valid plugin directory", abs)
	}
	return abs, nil
}

// ResolvePluginPath returns the absolute path of the named plugin on disk:
// (1) cwd/<name>, (2) cwd itself when it holds a plugin.yaml, (3) the registry.
func ResolvePluginPath(name string) (string, error) {
	cwd, err := os.Getwd()
	if err == nil {
		candidate := filepath.Join(cwd, name)
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate, nil
		}
		if _, err := os.Stat(filepath.Join(cwd, "plugin.yaml")); err == nil {
			return cwd, nil
		}
	}
	links, err := Load()
	if err == nil {
		if path, ok := links[name]; ok {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}
	return "", fmt.Errorf("plugin %q not found: run from plugin directory or link it first with `nself plugin link`", name)
}
