package darwin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOwnershipScanFindsDeepOwnedFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a", "b", "c", "d", "e", "f", "owned")
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	script := boundedOwnershipScan("'"+dir+"'", fmt.Sprintf("-type f -uid %d", os.Getuid()), 5)
	out, err := exec.Command("sh", "-c", script).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != file {
		t.Fatalf("deep ownership scan = %q, %v; want %q", out, err, file)
	}
}

func TestMountedOwnershipScanPreservesNonstandardMountpoint(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "custom mount")
	file := filepath.Join(dir, "a", "b", "c", "d", "e", "owned")
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	listing := "echo 'devfs on /missing-dev (devfs, local)'; echo 'map on /missing-autofs (autofs, automounted)'; echo '/dev/test on " + dir + " (apfs, local)'"
	script := mountedOwnershipScan(listing, fmt.Sprintf("-type f -uid %d", os.Getuid()), 30)
	out, err := exec.Command("sh", "-c", script).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != file {
		t.Fatalf("mounted ownership scan = %q, %v; want %q", out, err, file)
	}
	for _, command := range []string{"false", "echo malformed", "echo '/dev/test on /truncated (apfs'", "echo '/dev/test on /empty ()'", "echo '/dev/test on /path on /ambiguous (apfs, local)'"} {
		out, err := exec.Command("sh", "-c", mountedOwnershipScan(command, "-uid 450", 30)).CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != "scan-incomplete" {
			t.Fatalf("invalid mount listing = %q, %v", out, err)
		}
	}
}

// Exercise the generated shell with controlled find results, without scanning the host.
func TestOwnershipScanFailsClosed(t *testing.T) {
	for _, tc := range []struct{ name, find, want string }{
		{"empty", "return 0", ""},
		{"deep file", "echo /volume/a/b/c/d/e/f/owned", "/volume/a/b/c/d/e/f/owned"},
		{"unreadable", "return 1", "scan-incomplete"},
		{"timeout", "exec sleep 60", "scan-incomplete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			budget := 30
			if tc.name == "timeout" {
				budget = 5
			}
			script := "set -eu\nfind() { " + tc.find + "; }\n" + boundedOwnershipScan("/", "-uid 450", budget)
			out, err := exec.Command("sh", "-c", script).CombinedOutput()
			if err != nil || strings.TrimSpace(string(out)) != tc.want {
				t.Fatalf("scan = %q, %v; want %q", out, err, tc.want)
			}
			if time.Since(start) > 30*time.Second {
				t.Fatal("ownership scan exceeded its time cap")
			}
		})
	}
}

func TestOwnershipScanCoversAllVolumesWithoutDepthLimit(t *testing.T) {
	for _, script := range []string{usersScript(1), removeSlotScript(1)} {
		mustContain(t, "ownership scan", script, "mounts=$(/sbin/mount)", "-xdev", "-print -quit", "+ 30", "scan-incomplete")
		if strings.Contains(script, "-maxdepth") {
			t.Fatal("ownership scan ignores deeply nested files")
		}
	}
}

// A blocked stat/lookup before the watchdog would bypass the budget, so no directory test may run
// outside the bounded find: the validation belongs to find itself.
func TestOwnershipScanDoesNotProbeMetadataOutsideWatchdog(t *testing.T) {
	listing := "echo '/dev/test on /blocked (apfs, local)'"
	for name, script := range map[string]string{
		"mounted": mountedOwnershipScan(listing, "-uid 450", 30),
		"bounded": boundedOwnershipScan("/blocked", "-uid 450", 30),
	} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command("sh", "-xc", "find() { return 1; }\n"+script)
			out, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(out), "scan-incomplete") {
				t.Fatalf("scan = %q, %v; want scan-incomplete", out, err)
			}
			for _, probe := range []string{"'[' -d", "'[' -e", "test -d", "test -e"} {
				if strings.Contains(string(out), "+ "+probe) {
					t.Fatalf("scan probed metadata outside the watchdog (%q):\n%s", probe, out)
				}
			}
		})
	}
}

func TestMountedOwnershipScanFailsClosedOnMissingMountpoint(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	listing := "echo '/dev/test on " + missing + " (apfs, local)'"
	out, err := exec.Command("sh", "-c", mountedOwnershipScan(listing, "-uid 450", 30)).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "scan-incomplete" {
		t.Fatalf("missing mountpoint = %q, %v", out, err)
	}
}
