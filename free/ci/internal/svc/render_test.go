package svc

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

func TestUnitGoldens(t *testing.T) {
	for _, manager := range []string{"launchd", "systemd"} {
		t.Run(manager, func(t *testing.T) {
			i, _, _ := testInstaller(t, manager)
			i.Config.Binary = "/usr/bin/true"
			files, err := Files(i.Config)
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != len(i.Config.names()) {
				t.Fatal("missing unit")
			}
			for n, b := range files {
				if manager == "launchd" {
					checkPlist(t, b, strings.Contains(n, "selftest"))
				}
				if err := os.WriteFile(filepath.Join(i.Config.Home, n), b, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if manager == "launchd" {
				if p, err := exec.LookPath("/usr/bin/plutil"); err == nil {
					for n := range files {
						if out, err := exec.Command(p, "-lint", "--", filepath.Join(i.Config.Home, n)).CombinedOutput(); err != nil {
							t.Fatalf("plutil: %s: %v", out, err)
						}
					}
				}
				return
			}
			watch := string(files["nself-ci-watch.service"])
			for _, want := range []string{"Type=simple\n", "ExecStart=\"/usr/bin/true\" ci serve watch\n", "Restart=on-failure\nRestartSec=10\n", "StandardOutput=journal\nStandardError=journal\n", "WantedBy=default.target\n"} {
				if !strings.Contains(watch, want) {
					t.Errorf("watch unit missing %q: %s", want, watch)
				}
			}
			// path-valued settings take the path literally: systemd refuses a quoted WorkingDirectory=
			if want := "WorkingDirectory=" + strings.ReplaceAll(i.Config.Repo, "%", "%%") + "\n"; !strings.Contains(watch, want) {
				t.Errorf("watch unit missing %q: %s", want, watch)
			}
			self := string(files["nself-ci-selftest.service"])
			if !strings.Contains(self, "Type=oneshot\n") || !strings.Contains(self, "ci doctor --selftest --alert\n") || strings.Contains(self, "Restart=") {
				t.Fatal(self)
			}
			const timer = "[Unit]\nDescription=nSelf CI hourly selftest\n\n[Timer]\nOnCalendar=hourly\nPersistent=true\nUnit=nself-ci-selftest.service\n\n[Install]\nWantedBy=timers.target\n"
			if string(files["nself-ci-selftest.timer"]) != timer {
				t.Fatal("timer golden differs")
			}
			if p, err := exec.LookPath("/usr/bin/systemd-analyze"); err == nil {
				args := []string{"verify"}
				for _, n := range i.Config.names() {
					args = append(args, filepath.Join(i.Config.Home, n))
				}
				if out, err := exec.Command(p, args...).CombinedOutput(); err != nil {
					t.Fatalf("systemd-analyze: %s: %v", out, err)
				}
			}
		})
	}
}

func checkPlist(t *testing.T, b []byte, selftest bool) {
	t.Helper()
	d := xml.NewDecoder(bytes.NewReader(b))
	keys := map[string]bool{}
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if start, ok := tok.(xml.StartElement); ok && start.Name.Local == "key" {
			var key string
			if err := d.DecodeElement(&key, &start); err != nil {
				t.Fatal(err)
			}
			keys[key] = true
		}
	}
	for _, k := range []string{"Label", "ProgramArguments", "EnvironmentVariables", "PATH", "WorkingDirectory", "NSELF_CI_HOME", "NSELF_V15"} {
		if !keys[k] {
			t.Error("missing", k)
		}
	}
	if selftest {
		if !keys["StartInterval"] || keys["KeepAlive"] || !bytes.Contains(b, []byte("<integer>3600</integer>")) {
			t.Fatal("bad hourly plist")
		}
		for _, a := range []string{"doctor", "--selftest", "--alert"} {
			if !bytes.Contains(b, []byte("<string>"+a+"</string>")) {
				t.Fatal("missing", a)
			}
		}
	} else {
		for _, k := range []string{"RunAtLoad", "KeepAlive", "ThrottleInterval", "StandardOutPath", "StandardErrorPath"} {
			if !keys[k] {
				t.Error("missing", k)
			}
		}
		if !bytes.Contains(b, []byte("/logs/watch/watch.log</string>")) {
			t.Fatal("wrong watcher log")
		}
	}
}

func TestPathIndependentOfServiceEnv(t *testing.T) {
	i, _, _ := testInstaller(t, "systemd")
	v, err := toolchain.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	// Put a manifest tool only in its private pinned install directory.
	row, ok := v.Row("go")
	if !ok {
		t.Fatal("manifest has no go")
	}
	dir := filepath.Join(i.Config.CIHome, "tools", row.Name+"-"+row.Version, "bin")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte("#!/bin/sh\nprintf manifest-tool"), 0700); err != nil {
		t.Fatal(err)
	}
	before, err := Files(i.Config)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/hostile/nonexistent")
	after, err := Files(i.Config)
	if err != nil || !bytes.Equal(before["nself-ci-watch.service"], after["nself-ci-watch.service"]) {
		t.Fatal("service environment influenced unit", err)
	}
	path, err := GatePath(i.Config.CIHome)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after["nself-ci-watch.service"]), "PATH="+path) {
		t.Fatal("unit PATH differs")
	}
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", "go")
	cmd.Env = []string{"PATH=" + path}
	if out, err := cmd.CombinedOutput(); err != nil || string(out) != "manifest-tool" {
		t.Fatalf("gate tool failed with hostile inherited PATH: %q %v", out, err)
	}
}

func TestEscapingAndSystemIsolation(t *testing.T) {
	i, _, _ := testInstaller(t, "launchd")
	i.Config.Repo = filepath.Join(i.Config.Home, "a & b < c")
	files, err := Files(i.Config)
	if err != nil {
		t.Fatal(err)
	}
	checkPlist(t, files["org.nself.ci-watch.plist"], false)
	if !bytes.Contains(files["org.nself.ci-watch.plist"], []byte("a &amp; b &lt; c")) {
		t.Fatal("XML not escaped")
	}
	for _, manager := range []string{"launchd", "systemd"} {
		c := i.Config
		c.Manager, c.System, c.DryRun = manager, true, true
		c.Home, c.CIHome = SystemHome(manager), SystemHome(manager)
		files, err := Files(c)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range files {
			if bytes.Contains(b, []byte("nself-ci-job")) {
				t.Fatal("reused job user")
			}
		}
		c.DryRun = false
		if _, err := Files(c); err == nil {
			t.Fatal("system mutation allowed in CI")
		}
	}
}
