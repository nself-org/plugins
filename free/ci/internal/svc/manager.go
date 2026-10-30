package svc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ManagerPath resolves a fixed system manager. Under CI=1 only an executable
// beneath the temporary login HOME is accepted, with no fallback to the host.
func ManagerPath(c Config) (string, error) {
	name := "systemctl"
	if c.Manager == "launchd" {
		name = "launchctl"
	}
	dirs := []string{"/bin", "/usr/bin"}
	if os.Getenv("CI") == "1" {
		dirs = filepath.SplitList(os.Getenv("PATH"))
	}
	for _, dir := range dirs {
		if !filepath.IsAbs(dir) {
			continue
		}
		p, err := filepath.EvalSymlinks(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if os.Getenv("CI") == "1" {
			home, herr := filepath.EvalSymlinks(c.Home)
			rel, rerr := filepath.Rel(home, p)
			if herr != nil || rerr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
		}
		fi, err := os.Stat(p)
		if err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("no permitted %s executable (CI=1 requires a fake beneath HOME)", name)
}

func (i Installer) command(ctx context.Context, manager string, args ...string) error {
	fmt.Fprintf(i.writer(), "%s %s\n", manager, strings.Join(args, " "))
	if i.Config.DryRun {
		return nil
	}
	if i.Run == nil {
		return fmt.Errorf("no service manager runner")
	}
	return i.Run(ctx, append([]string{manager}, args...)...)
}

func (i Installer) activate(ctx context.Context, manager string, names []string) error {
	c := i.Config
	if c.Manager == "systemd" {
		prefix := []string{}
		if !c.System {
			prefix = append(prefix, "--user")
		}
		if err := i.command(ctx, manager, append(prefix, "daemon-reload")...); err != nil {
			return err
		}
		for _, n := range names {
			if n == "nself-ci-selftest.service" {
				continue
			}
			if err := i.command(ctx, manager, append(prefix, "enable", "--now", n)...); err != nil {
				return err
			}
			if err := i.command(ctx, manager, append(prefix, "restart", n)...); err != nil {
				return err
			}
		}
		return nil
	}
	for _, n := range names {
		// A newly installed or changed job may already be loaded. Probe first;
		// only boot it out when launchd confirms it exists.
		label := strings.TrimSuffix(n, ".plist")
		if !c.DryRun && i.Run(ctx, manager, "print", c.domain()+"/"+label) == nil {
			if err := i.command(ctx, manager, "bootout", c.domain()+"/"+label); err != nil {
				return err
			}
		}
		if err := i.command(ctx, manager, "bootstrap", c.domain(), filepath.Join(c.dir(), n)); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) domain() string {
	if c.System {
		return "system"
	}
	return "gui/" + strconv.Itoa(c.UID)
}

func (i Installer) deactivate(ctx context.Context, manager, name string) error {
	c := i.Config
	if c.Manager == "launchd" {
		job := c.domain() + "/" + strings.TrimSuffix(name, ".plist")
		if !c.DryRun && i.Run(ctx, manager, "print", job) != nil {
			return nil
		}
		return i.command(ctx, manager, "bootout", job)
	}
	args := []string{}
	if !c.System {
		args = append(args, "--user")
	}
	return i.command(ctx, manager, append(args, "disable", "--now", name)...)
}
