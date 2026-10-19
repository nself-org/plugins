package bootstrap

// Purpose: TestBootstrapRegistry (the registry contract NODE-31 builds on) and the OS detection.
// Inputs:  fake families and fake node replies. Constraints: no network, no disk, no process.

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

type fakeFamily struct{ name, os string }

func (f fakeFamily) Name() string                                        { return f.name }
func (f fakeFamily) Match(os string) bool                                { return os == f.os }
func (fakeFamily) Install(context.Context, remote.Target, Options) error { return nil }
func (fakeFamily) Uninstall(context.Context, Options) error              { return nil }

func TestBootstrapRegistry(t *testing.T) {
	t.Run("the three linux families are registered", func(t *testing.T) {
		for os, want := range map[string]string{"debian": "apt", "ubuntu": "apt", "fedora": "dnf", "rocky": "dnf", "alpine": "apk"} {
			f, ok := Lookup(os)
			if !ok || f.Name() != want {
				t.Errorf("Lookup(%q) = %v, %v; want %s", os, f, ok, want)
			}
		}
	})
	t.Run("register and look up", func(t *testing.T) {
		defer unregister("fake")
		if err := RegisterFamily(fakeFamily{"fake", "plan9"}); err != nil {
			t.Fatal(err)
		}
		if f, ok := Lookup("plan9"); !ok || f.Name() != "fake" {
			t.Fatalf("Lookup(plan9) = %v, %v", f, ok)
		}
	})
	t.Run("a duplicate name is refused", func(t *testing.T) {
		defer unregister("fake")
		if err := RegisterFamily(fakeFamily{"fake", "plan9"}); err != nil {
			t.Fatal(err)
		}
		err := RegisterFamily(fakeFamily{"fake", "other"})
		if ReasonOf(err) != ReasonDuplicate {
			t.Fatalf("duplicate: %v", err)
		}
		if _, ok := Lookup("other"); ok {
			t.Fatal("the refused family became reachable")
		}
		for _, bad := range []Family{nil, fakeFamily{"", "x"}} {
			if ReasonOf(RegisterFamily(bad)) != ReasonFamilyEmpty {
				t.Errorf("%v registered", bad)
			}
		}
	})
	t.Run("an unregistered darwin falls back to the brew commands", func(t *testing.T) {
		if _, ok := Lookup("darwin"); ok {
			t.Skip("a darwin family is registered in this build")
		}
		run := func(_ context.Context, argv ...string) (string, error) { return "Darwin", nil }
		f, keys, ok, err := Detect(context.Background(), run)
		if err != nil || ok || f != nil || len(keys) != 1 || keys[0] != "darwin" {
			t.Fatalf("Detect = %v %v %v %v", f, keys, ok, err)
		}
		var buf bytes.Buffer
		PrintDarwinHint(&buf)
		if !strings.Contains(buf.String(), "brew install git") {
			t.Fatalf("hint = %q", buf.String())
		}
	})
}

func TestDetectReadsOSRelease(t *testing.T) {
	rel := "NAME=\"Ubuntu\"\nID=ubuntu\nID_LIKE=\"debian\"\n"
	run := func(_ context.Context, argv ...string) (string, error) {
		if argv[0] == "uname" {
			return "Linux", nil
		}
		return rel, nil
	}
	f, keys, ok, err := Detect(context.Background(), run)
	if err != nil || !ok || f.Name() != "apt" || strings.Join(keys, ",") != "ubuntu,debian" {
		t.Fatalf("Detect = %v %v %v %v", f, keys, ok, err)
	}
	solaris := func(_ context.Context, argv ...string) (string, error) { return "SunOS", nil }
	if _, _, _, err = Detect(context.Background(), solaris); ReasonOf(err) != ReasonUnsupported {
		t.Fatalf("SunOS: %v", err)
	}
}
