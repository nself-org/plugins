package exec

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestRetainContainerPreservesFlagValuesAndJobArguments(t *testing.T) {
	args := []string{"run", "--rm", "--label", "--rm", "--read-only", "alpine:3.19", "echo", "--rm"}
	want := []string{"run", "--label", "--rm", "--read-only", "alpine:3.19", "echo", "--rm"}
	if got := retainContainer(args, "alpine:3.19"); !reflect.DeepEqual(got, want) {
		t.Fatalf("retained command %q, want %q", got, want)
	}
}

func TestContainerStopPreservesEvidenceBeforeRemoval(t *testing.T) {
	calls := fakeRuntime(t, "", "0")
	stop := containerStopperBeforeRemove("docker", "nself-ci-final", time.Second, func() {
		f, err := os.OpenFile(filepath.Join(fakeRuntimeDir, "calls"), os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := f.WriteString("sample\n"); err != nil {
			t.Fatal(err)
		}
	})
	stop()
	stop()
	want := []string{"stop -t 0 nself-ci-final", "sample", "rm -f nself-ci-final"}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("stop, sample, removal order %q, want %q", got, want)
	}
}
