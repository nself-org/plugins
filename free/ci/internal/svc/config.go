// Package svc renders and reconciles the personal watcher's service files.
package svc

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

// Config contains explicit paths; no service value is taken from PATH.
type Config struct {
	Manager                    string
	System, DryRun             bool
	Home, CIHome, Repo, Binary string
	EnvFile                    string
	UID                        int
}

// Run executes manager argv through the caller's process funnel.
type Run func(context.Context, ...string) error

// Installer owns only its fixed unit names and adjacent receipt.
type Installer struct {
	Config Config
	Run    Run
	Out    io.Writer
}

// GatePath builds a deterministic search list from manifest tools and their
// private install roots, followed by standard host tool locations.
func GatePath(home string) (string, error) {
	v, err := toolchain.Embedded()
	if err != nil {
		return "", err
	}
	dirs := []string{filepath.Join(home, "bin")}
	for _, row := range v.Rows {
		if row.Kind == toolchain.KindTool && row.Owner != "internal/nodes/provision" {
			root := filepath.Join(home, "tools", row.Name+"-"+row.Version)
			dirs = append(dirs, root, filepath.Join(root, "bin"))
		}
	}
	dirs = append(dirs, "/usr/local/go/bin", "/usr/local/rust/bin", "/usr/local/lib/pnpm", "/usr/local/python/bin", "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin")
	return strings.Join(dirs, ":"), nil
}

func (c Config) validate() error {
	if c.Manager != "launchd" && c.Manager != "systemd" {
		return fmt.Errorf("choose --launchd or --systemd")
	}
	if err := c.systemGuard(); err != nil {
		return err
	}
	for _, p := range []string{c.Home, c.CIHome, c.Repo, c.Binary} {
		if !filepath.IsAbs(p) || strings.ContainsAny(p, "\r\n\x00:") {
			return fmt.Errorf("service paths must be absolute and contain no newline or colon")
		}
	}
	if c.EnvFile != "" {
		if c.Manager != "systemd" {
			return fmt.Errorf("launchd uses the login environment; --env-file requires systemd")
		}
		fi, err := os.Lstat(c.EnvFile)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(c.EnvFile) || !fi.Mode().IsRegular() || fi.Mode().Perm() != 0600 || strings.ContainsAny(c.EnvFile, "\r\n\x00") {
			return fmt.Errorf("--env-file must name an absolute regular 0600 file")
		}
	}
	return nil
}

func (c Config) dir() string {
	if c.Manager == "launchd" {
		if c.System {
			return "/Library/LaunchDaemons"
		}
		return filepath.Join(c.Home, "Library", "LaunchAgents")
	}
	if c.System {
		return "/etc/systemd/system"
	}
	return filepath.Join(c.Home, ".config", "systemd", "user")
}

func (c Config) names() []string {
	if c.Manager == "launchd" {
		return []string{"org.nself.ci-watch.plist", "org.nself.ci-selftest.plist"}
	}
	return []string{"nself-ci-watch.service", "nself-ci-selftest.service", "nself-ci-selftest.timer"}
}

func (i Installer) writer() io.Writer {
	if i.Out == nil {
		return io.Discard
	}
	return i.Out
}
