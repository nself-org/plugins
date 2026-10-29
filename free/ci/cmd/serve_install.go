package main

// Purpose: V15-only watcher service installation and exact uninstall.
// Inputs: explicit manager, scope and paths. No credential is read or rendered.
// Outputs: service files, manager argv, and unchanged/edited-file notes.
// Constraints: user scope by default; system scope requires sudo and a node agent.
// Side effects: install writes; uninstall is destructive, declared in the surface.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/svc"
)

type serveInstallFlags struct {
	launchd, systemd, user, system, dryRun bool
	binary, repo, envFile                  string
}

func newServeInstallFlagSet(name string, f *serveInstallFlags) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&f.launchd, "launchd", false, "use launchd")
	fs.BoolVar(&f.systemd, "systemd", false, "use systemd")
	fs.BoolVar(&f.user, "user", false, "install for the login user (default)")
	fs.BoolVar(&f.system, "system", false, "install a dedicated system watcher on a node-agent host (sudo)")
	fs.BoolVar(&f.dryRun, "dry-run", false, "print files and manager commands without changes")
	fs.StringVar(&f.binary, "binary", "", "absolute nself CLI path (default: nself in standard bin directories)")
	fs.StringVar(&f.repo, "repo", "", "repository checkout (default: current directory)")
	fs.StringVar(&f.envFile, "env-file", "", "existing 0600 systemd environment file, never copied or read")
	return fs
}

func init() {
	for _, action := range []string{"install", "uninstall"} {
		name := "serve " + action
		registerV15("serve "+action, subcommandEntry{V15Only: true,
			Flags:   func() *flag.FlagSet { return newServeInstallFlagSet(name, &serveInstallFlags{}) },
			Handler: func(args []string) int { return serveInstallCmd(action, args) }})
	}
}

func serveInstallCmd(action string, args []string) int {
	var f serveInstallFlags
	pos, err := parseMixed(newServeInstallFlagSet("serve "+action, &f), args)
	if err != nil || len(pos) != 0 || f.launchd == f.systemd || (f.user && f.system) {
		return usageErr("nself ci serve " + action + " (--launchd|--systemd) [--user|--system] [--dry-run] [--binary /path/to/nself] [--repo /checkout] [--env-file /path]")
	}
	c, err := serveServiceConfig(f)
	if err == nil {
		i := svc.Installer{Config: c, Out: os.Stdout, Run: func(ctx context.Context, argv ...string) error {
			_, exit, e := report.Capture(ctx, argv...)
			if e != nil {
				return e
			}
			if exit != 0 {
				return fmt.Errorf("%s exited %d", filepath.Base(argv[0]), exit)
			}
			return nil
		}}
		if action == "install" {
			err = i.Install(context.Background())
		} else {
			err = i.Uninstall(context.Background())
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

func serveServiceConfig(f serveInstallFlags) (svc.Config, error) {
	c := svc.Config{Manager: "systemd", System: f.system, DryRun: f.dryRun, Binary: f.binary, Repo: f.repo, EnvFile: f.envFile, UID: os.Getuid()}
	if f.launchd {
		c.Manager = "launchd"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return c, err
	}
	c.Home, c.CIHome = home, os.Getenv("NSELF_CI_HOME")
	if c.CIHome == "" {
		c.CIHome = filepath.Join(home, ".nself", "ci")
	}
	if c.System {
		c.Home = svc.SystemHome(c.Manager)
		c.CIHome = c.Home
	}
	if c.Repo == "" {
		c.Repo, err = os.Getwd()
		if err != nil {
			return c, err
		}
	}
	if c.Binary == "" {
		for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin"} {
			p := filepath.Join(dir, "nself")
			if fi, e := os.Stat(p); e == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0111 != 0 {
				c.Binary = p
				break
			}
		}
		if c.Binary == "" {
			return c, fmt.Errorf("pass --binary with the absolute nself CLI path")
		}
	}
	return c, nil
}
