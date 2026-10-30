//go:build !windows

package container

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
)

// A killed coordinator cannot call the restricted network plan's Release. Teardown must
// discover all attempt-labelled resources from the runtime without that in-memory plan.
func TestTeardownRestrictedAttemptAfterCoordinatorKill(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$TEST_RUNTIME_CALLS"
case "$1 $2" in
 "ps -a") printf 'job burst-A1 running\nhelper burst-A1 exited\nproxy burst-A1 running\n' ;;
 "network ls") echo 'net burst-A1' ;;
 "inspect --format"|"network inspect") echo 'burst-A1' ;;
esac
`
	script = strings.ReplaceAll(script, `"$TEST_RUNTIME_CALLS"`, "'"+strings.ReplaceAll(log, "'", "'\"'\"'")+"'")
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	// A fresh profile has no Release closure left by the killed coordinator.
	p := newProfile(&fakeRunner{info: map[string]string{"docker": dockerRootless}}, false)
	if err := p.Teardown(context.Background(), isolation.Attempt{ID: "burst-A1"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	calls := string(raw)
	for _, want := range []string{"rm -f job", "rm -f helper", "rm -f proxy", "network rm net"} {
		if !strings.Contains(calls, want+"\n") {
			t.Fatalf("missing %s in %s", want, calls)
		}
	}
	if strings.Index(calls, "network rm") < strings.Index(calls, "rm -f proxy") {
		t.Fatal("network removed before proxy")
	}
}
