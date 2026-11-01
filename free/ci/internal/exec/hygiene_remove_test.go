//go:build !windows

package exec

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeRemoveDocker puts a docker on PATH that reports the attempt label on inspect and answers rm with
// rmOut on stderr and rmCode, for containers and (after `network`/`volume`) the other kinds.
func fakeRemoveDocker(t *testing.T, attempt, rmOut string, rmCode int) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in network|volume) shift ;; esac\ncase \"$1\" in\ninspect) echo " + attempt + " ;;\n" +
		"rm) echo '" + rmOut + "' >&2; exit " + strconv.Itoa(rmCode) + " ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// A second teardown that races the first finds the container's removal under way: not an error.
func TestRuntimeRemoveLabelledRemovalInProgress(t *testing.T) {
	fakeRemoveDocker(t, "a1", "Error response from daemon: removal of container abc is already in progress", 1)
	if err := RuntimeRemoveLabelled(context.Background(), "docker", KindContainer, "abc", "a1"); err != nil {
		t.Fatalf("removal in progress: %v, want nil", err)
	}
}

// Any other refusal is E613 and carries the runtime's own first line.
func TestRuntimeRemoveLabelledReportsRuntimeOutput(t *testing.T) {
	fakeRemoveDocker(t, "a1", "Error response from daemon: permission denied", 1)
	err := RuntimeRemoveLabelled(context.Background(), "docker", KindContainer, "abc", "a1")
	var e *Error
	if !asError(err, &e) || e.Code != "E613" || !strings.Contains(e.Message, "permission denied") {
		t.Fatalf("err = %v, want E613 naming the runtime's message", err)
	}
	// a network still in use is not "removal in progress" and stays an error
	fakeRemoveDocker(t, "a1", "Error response from daemon: removal of container abc is already in progress", 1)
	if err := RuntimeRemoveLabelled(context.Background(), "docker", KindNetwork, "abc", "a1"); err == nil {
		t.Fatal("network rm failure tolerated as a container removal in progress")
	}
}
