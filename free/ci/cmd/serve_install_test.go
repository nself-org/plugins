package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeInstallRegistration(t *testing.T) {
	s := loadSpec(t)
	for _, action := range []string{"install", "uninstall"} {
		name := "serve " + action
		entry, ok := v15Subcommands[name]
		if !ok || !entry.V15Only {
			t.Fatal("missing V15 entry", name)
		}
		if _, legacy := subcommands[name]; legacy {
			t.Fatal("changed legacy dispatch")
		}
		found := false
		for _, r := range s.Commands {
			if r.Path != name {
				continue
			}
			found = true
			if problems := checkFlags(name, r, entry.Flags(), true); len(problems) != 0 {
				t.Fatal(problems)
			}
			want := "write"
			if action == "uninstall" {
				want = "destructive"
			}
			if r.SideEffect != want {
				t.Fatal("wrong side effect")
			}
		}
		if !found {
			t.Fatal("missing surface row")
		}
	}
}

func TestServeInstallFakeManagers(t *testing.T) {
	for _, manager := range []string{"launchd", "systemd"} {
		t.Run(manager, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("CI", "1")
			t.Setenv("NSELF_CI_HOME", filepath.Join(home, "ci"))
			bin := filepath.Join(home, "bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			log := filepath.Join(home, "calls")
			name := "systemctl"
			if manager == "launchd" {
				name = "launchctl"
			}
			script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + log + "'\n"
			if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			// The process funnel uses sh to launch argv. Keep only that launcher
			// and the fake manager available on PATH.
			if err := os.Symlink("/bin/sh", filepath.Join(bin, "sh")); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			args := []string{"--" + manager, "--binary", "/usr/local/bin/nself", "--repo", home}
			if code := serveInstallCmd("install", args); code != 0 {
				t.Fatalf("install exit %d", code)
			}
			before, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if code := serveInstallCmd("install", args); code != 0 {
				t.Fatalf("repeat exit %d", code)
			}
			after, err := os.ReadFile(log)
			if err != nil || string(before) != string(after) {
				t.Fatal("repeat invoked manager", err)
			}
			if code := serveInstallCmd("uninstall", args); code != 0 {
				t.Fatalf("uninstall exit %d", code)
			}
			after, err = os.ReadFile(log)
			if err != nil || !strings.Contains(string(after), "ci-watch") {
				t.Fatal("fake manager not executed", err)
			}
		})
	}
}

func TestServeInstallBadFlags(t *testing.T) {
	for _, args := range [][]string{nil, {"--launchd", "--systemd"}, {"--launchd", "--user", "--system"}, {"--systemd", "unexpected"}} {
		if code := serveInstallCmd("install", args); code != 1 {
			t.Fatalf("%v: got %d", args, code)
		}
	}
}
