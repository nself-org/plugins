package maintenance

import "strings"

import "testing"

// The generated unit previously had no Environment=HOME. systemd starts units
// with an almost empty environment, and nself resolves plugins under
// $HOME/.nself/plugins, so the nightly timer failed to find the plugin that
// provides disk-cleanup and exited 1 every night without surfacing anything.
// The nSelf CI host filled to 100% that way on 2026-09-02.
func TestSystemdServiceUnit_SetsHome(t *testing.T) {
	unit := systemdServiceUnit("/root", "/usr/local/bin/nself")

	if !strings.Contains(unit, "Environment=HOME=/root") {
		t.Fatalf("unit must set HOME or the timer cannot find nself's plugins:\n%s", unit)
	}
	if !strings.Contains(unit, "ExecStart=/usr/local/bin/nself maintenance disk-cleanup") {
		t.Fatalf("unit lost its ExecStart:\n%s", unit)
	}
	// HOME has to be set before ExecStart runs; systemd does not care about
	// ordering within [Service], but a reader does, and an Environment= line
	// landing in [Unit] would silently do nothing.
	svc := strings.Index(unit, "[Service]")
	env := strings.Index(unit, "Environment=HOME=")
	if svc < 0 || env < svc {
		t.Fatalf("Environment= must be inside the [Service] section:\n%s", unit)
	}
}

func TestSystemdServiceUnit_UsesResolvedHome(t *testing.T) {
	unit := systemdServiceUnit("/home/ci", "/opt/nself")
	if !strings.Contains(unit, "Environment=HOME=/home/ci") {
		t.Fatalf("unit did not use the home it was given:\n%s", unit)
	}
}
