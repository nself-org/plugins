package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSweepIgnoresAContainerRuntimeThatIsDown proves the staging sweep does not depend on the
// host's container runtime (D-1222): macOS hosts have docker installed with the daemon stopped,
// and the sweep used to skip every orphan because it could not list containers.
func TestSweepIgnoresAContainerRuntimeThatIsDown(t *testing.T) {
	skipUnixOnly(t)
	bin := t.TempDir()
	script := "#!/bin/sh\necho 'Cannot connect to the Docker daemon' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	checkSweep(t)
}
