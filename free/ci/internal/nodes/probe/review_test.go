package probe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestProbeIsolationCeilings(t *testing.T) {
	for _, tc := range []struct {
		name, paths, tartVersion, dockerVersion string
		want                                    model.Isolation
	}{
		{"docker", "/usr/bin/docker", "", "27.1.0", "container"},
		{"podman", "/usr/bin/podman", "", "", "container"},
		{"gvisor", "/usr/bin/runsc", "", "", "sandboxed-container"},
		{"tart", "/opt/homebrew/bin/tart", "tart 2.13.0", "", "vm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c model.Capability
			runner := func(_ context.Context, command string) (string, error) {
				switch command {
				case cmdUname:
					if tc.name == "tart" {
						return "Darwin 24.1.0 arm64", nil
					}
					return "Linux 6.1.0 x86_64", nil
				case cmdTools:
					return tc.paths, nil
				case cmdTart:
					return tc.tartVersion, nil
				case cmdDocker:
					return tc.dockerVersion, nil
				default:
					return "", errors.New("absent")
				}
			}
			if err := collect(context.Background(), &c, runner, "", time.Now(), false); err != nil {
				t.Fatal(err)
			}
			if c.Trust.Isolation.Value == nil || *c.Trust.Isolation.Value != tc.want {
				t.Fatalf("isolation ceiling = %v, want %s", c.Trust.Isolation.Value, tc.want)
			}
		})
	}
}

func TestProbeAcceleratorAbsentVersusEmpty(t *testing.T) {
	for _, tc := range []struct{ name, osName, command, output string }{
		{"linux", "linux", cmdNvidia, ""},
		{"linux no devices", "linux", cmdNvidia, "No devices were found"},
		{"darwin", "darwin", cmdDisplays, `{"SPDisplaysDataType":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, ran := range []bool{false, true} {
				var c model.Capability
				outputs := map[string]string{}
				if ran {
					outputs[tc.command] = tc.output
				}
				applyToolFacts(&c, outputs, tc.osName, time.Now())
				got := c.Resources.Accelerators
				if ran && (got.Value == nil || !reflect.DeepEqual(*got.Value, []string{}) || got.Confidence != "known") {
					t.Fatalf("successful zero-device command: %+v", got)
				}
				if !ran && (got.Value != nil || got.Confidence != "unknown") {
					t.Fatalf("absent command: %+v", got)
				}
			}
		})
	}
}

func TestProbePreservesSelfDeclaredToolchains(t *testing.T) {
	now := time.Now()
	declared := value("21", now)
	declared.Source = "self"
	c := model.Capability{}
	c.Tools.Toolchains = map[string]model.Fact[string]{"java": declared}
	original := c.Tools.Toolchains
	applyToolFacts(&c, map[string]string{cmdTools: "/usr/bin/go"}, "linux", now)
	if c.Tools.Toolchains["java"].Value == nil || *c.Tools.Toolchains["java"].Value != "21" {
		t.Fatal("self-declared toolchain lost")
	}
	c.Tools.Toolchains["java"] = value("changed", now)
	if *original["java"].Value != "21" {
		t.Fatal("toolchain map was not copied")
	}
}

func TestProbeHostLocksBounded(t *testing.T) {
	for i := 0; i < 10000; i++ {
		_ = hostLock(fmt.Sprintf("host-%d", i))
	}
	if len(hosts) > 256 {
		t.Fatalf("host locks grew to %d", len(hosts))
	}
}

func TestProbeOSReleaseMatchingQuotes(t *testing.T) {
	for _, raw := range []string{`VERSION_ID='12'`, `VERSION_ID="12"`, `VERSION_ID=12`} {
		got := parseOSRelease(raw)
		if got == nil || *got != "12" {
			t.Fatalf("%q parsed as %v", raw, got)
		}
	}
}

func TestProbeCanceledBeforeProcessStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	_, err := startLocalProcess(ctx, func() (*os.Process, error) { called = true; return nil, nil })
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("called=%v err=%v", called, err)
	}
}
