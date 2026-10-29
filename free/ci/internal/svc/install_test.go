package svc

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testInstaller(t *testing.T, manager string) (Installer, *bytes.Buffer, *[][]string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("CI", "1")
	t.Setenv("HOME", home)
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"launchctl", "systemctl"} {
		if err := os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	c := Config{Manager: manager, Home: home, CIHome: filepath.Join(home, "ci"), Repo: home, Binary: "/usr/local/bin/nself", UID: 501}
	var out bytes.Buffer
	calls := new([][]string)
	i := Installer{Config: c, Out: &out, Run: func(_ context.Context, argv ...string) error {
		*calls = append(*calls, append([]string{}, argv...))
		return nil
	}}
	return i, &out, calls
}

func TestInstallIdempotent(t *testing.T) {
	for _, manager := range []string{"launchd", "systemd"} {
		t.Run(manager, func(t *testing.T) {
			i, out, calls := testInstaller(t, manager)
			if err := i.Install(context.Background()); err != nil {
				t.Fatal(err)
			}
			old := time.Unix(123456, 0)
			paths := append(i.Config.names(), filepath.Base(i.Config.receiptPath()))
			for _, n := range paths {
				if err := os.Chtimes(filepath.Join(i.Config.dir(), n), old, old); err != nil {
					t.Fatal(err)
				}
			}
			before := len(*calls)
			out.Reset()
			if err := i.Install(context.Background()); err != nil {
				t.Fatal(err)
			}
			if out.String() != "unchanged\n" || before != len(*calls) {
				t.Fatalf("second install: %q, calls %v", out, *calls)
			}
			for _, n := range paths {
				fi, err := os.Stat(filepath.Join(i.Config.dir(), n))
				if err != nil || !fi.ModTime().Equal(old) {
					t.Fatalf("rewrote %s: %v", n, err)
				}
			}
		})
	}
}

func TestUninstallExact(t *testing.T) {
	for _, manager := range []string{"launchd", "systemd"} {
		t.Run(manager, func(t *testing.T) {
			i, out, calls := testInstaller(t, manager)
			if err := i.Install(context.Background()); err != nil {
				t.Fatal(err)
			}
			name := i.Config.names()[0]
			path := filepath.Join(i.Config.dir(), name)
			if err := os.WriteFile(path, []byte("user edit"), 0644); err != nil {
				t.Fatal(err)
			}
			other := filepath.Join(i.Config.dir(), "other.service")
			if err := os.WriteFile(other, []byte("unrelated"), 0644); err != nil {
				t.Fatal(err)
			}
			*calls = nil
			if err := i.Uninstall(context.Background()); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "leaving user-edited") {
				t.Fatal("no note")
			}
			for _, p := range []string{path, other} {
				if _, err := os.Stat(p); err != nil {
					t.Fatal(err)
				}
			}
			for _, n := range i.Config.names()[1:] {
				if _, err := os.Stat(filepath.Join(i.Config.dir(), n)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("left %s: %v", n, err)
				}
			}
			for _, argv := range *calls {
				if strings.Contains(strings.Join(argv, " "), strings.TrimSuffix(name, ".plist")) {
					t.Fatalf("touched edited job: %v", argv)
				}
			}
		})
	}
	i, _, _ := testInstaller(t, "systemd")
	if err := i.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := i.Uninstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(i.Config.dir())
	if err != nil || len(entries) != 0 {
		t.Fatalf("left files: %v, %v", entries, err)
	}
	if err := i.Uninstall(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestNeverTouchesRealManager(t *testing.T) {
	i, _, calls := testInstaller(t, "systemd")
	t.Setenv("PATH", "/usr/bin:/bin")
	if err := i.Install(context.Background()); err == nil {
		t.Fatal("accepted real manager in CI")
	}
	if len(*calls) != 0 {
		t.Fatal("ran real manager")
	}
	if _, err := os.Stat(i.Config.dir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("wrote files before rejecting manager")
	}
	t.Setenv("PATH", filepath.Join(i.Config.Home, "bin"))
	if err := os.Remove(filepath.Join(i.Config.Home, "bin", "systemctl")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/usr/bin/true", filepath.Join(i.Config.Home, "bin", "systemctl")); err != nil {
		t.Fatal(err)
	}
	if _, err := ManagerPath(i.Config); err == nil {
		t.Fatal("allowed symlink escape")
	}
}

func TestDryRunAndRefusal(t *testing.T) {
	i, out, calls := testInstaller(t, "launchd")
	i.Config.DryRun = true
	if err := i.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 || !strings.Contains(out.String(), "bootstrap gui/501") || !strings.Contains(out.String(), "<plist") {
		t.Fatalf("bad dry-run: %s", out)
	}
	if _, err := os.Stat(i.Config.dir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dry-run wrote")
	}
	i.Config.DryRun = false
	if err := os.MkdirAll(i.Config.dir(), 0700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(i.Config.dir(), i.Config.names()[1])
	if err := os.WriteFile(p, []byte("unowned"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := i.Install(context.Background()); err == nil {
		t.Fatal("overwrote unowned")
	}
	if len(*calls) != 0 {
		t.Fatal("called manager on refusal")
	}
}

func TestActivationRetry(t *testing.T) {
	i, _, calls := testInstaller(t, "systemd")
	run := i.Run
	i.Run = func(context.Context, ...string) error { return errors.New("manager unavailable") }
	if err := i.Install(context.Background()); err == nil {
		t.Fatal("lost manager error")
	}
	i.Run = run
	if err := i.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	r, err := i.Config.receipt()
	if err != nil || !r.Active || len(*calls) == 0 {
		t.Fatalf("did not retry: %+v %v", r, err)
	}
	files, err := Files(i.Config)
	if err != nil {
		t.Fatal(err)
	}
	for n, want := range files {
		got, err := os.ReadFile(filepath.Join(i.Config.dir(), n))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal(n, err)
		}
	}
}
