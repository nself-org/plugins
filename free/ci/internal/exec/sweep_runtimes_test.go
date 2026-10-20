//go:build !windows

package exec

// Purpose: the orphan sweep reaches every installed container runtime, not only docker (D-0965).
// Constraints: fake runtime scripts on a PATH that holds nothing else; no real daemon is touched.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSweepRuntime installs a runtime script named binary into dir. It answers info with {}, ps with
// listing (exit psCode) and logs every call to <dir>/<binary>.calls.
func fakeSweepRuntime(t *testing.T, dir, binary, listing string, psCode string) {
	t.Helper()
	script := "#!/bin/sh\necho \"$*\" >> \"${0%/*}/" + binary + ".calls\"\n" +
		"case \"$1\" in\n" +
		"info) echo '{}';;\n" +
		"ps) printf '%s\\n' '" + listing + "'; exit " + psCode + ";;\n" +
		"esac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, binary), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
}

type sweepStale map[string]bool

func (m sweepStale) Stale(_ context.Context, id string) (bool, error) { return m[id], nil }

func sweepCalls(t *testing.T, dir, binary string) []string {
	t.Helper()
	data, _ := os.ReadFile(filepath.Join(dir, binary+".calls"))
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestSweepReachesEveryRuntime(t *testing.T) {
	type rt struct{ listing, psCode string } // an absent key means the binary is not installed
	cases := []struct {
		name        string
		runtimes    map[string]rt
		wantRemoved map[string][]string // binary -> ids removed
		wantErr     bool
	}{
		{"podman only", map[string]rt{"podman": {"p1 dead", "0"}}, map[string][]string{"podman": {"p1"}}, false},
		{"docker only", map[string]rt{"docker": {"d1 dead", "0"}}, map[string][]string{"docker": {"d1"}}, false},
		{"both runtimes", map[string]rt{"docker": {"d1 dead", "0"}, "podman": {"p1 dead", "0"}}, map[string][]string{"docker": {"d1"}, "podman": {"p1"}}, false},
		{"live and current containers stay", map[string]rt{"podman": {"p1 alive\np2 now\np3 dead", "0"}}, map[string][]string{"podman": {"p3"}}, false},
		{"docker listing fails, podman is still swept", map[string]rt{"docker": {"", "1"}, "podman": {"p1 dead", "0"}}, map[string][]string{"podman": {"p1"}}, true},
		{"no runtime installed", map[string]rt{}, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ResetRuntimeInfoCache()
			dir := t.TempDir()
			t.Setenv("PATH", dir)
			for binary, r := range tc.runtimes {
				fakeSweepRuntime(t, dir, binary, r.listing, r.psCode)
			}
			e := &Executor{Stale: sweepStale{"dead": true, "alive": false}}
			err := e.sweepContainers(context.Background(), "now")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}
			for binary := range tc.runtimes {
				var removed []string
				for _, call := range sweepCalls(t, dir, binary) {
					if id, ok := strings.CutPrefix(call, "rm -f "); ok {
						removed = append(removed, id)
					}
				}
				if got, want := strings.Join(removed, ","), strings.Join(tc.wantRemoved[binary], ","); got != want {
					t.Errorf("%s removed %q, want %q", binary, got, want)
				}
			}
		})
	}
}
