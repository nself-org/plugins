package cas

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

// TestBuildsForWindows guards D-1005: the CAS must compile for windows/amd64
// (P7-PLUG-69) even though the store itself is unix-only at run time.
func TestBuildsForWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("already running on windows")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not on PATH")
	}
	cmd := exec.Command(goBin, "build", "-o", os.DevNull, ".")
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("GOOS=windows go build failed: %v\n%s", err, out)
	}
}
