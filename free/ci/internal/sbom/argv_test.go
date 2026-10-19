package sbom_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sbom"
)

func TestArgvPerSourceKind(t *testing.T) {
	cases := []struct {
		t    sbom.Target
		want string
	}{
		{sbom.Target{Name: "cli", Path: "dist/cli", Kind: sbom.SourceFile}, "file:dist/cli"},
		{sbom.Target{Name: "web", Path: "./out/web/", Kind: sbom.SourceDir}, "dir:out/web"},
		{sbom.Target{Name: "img", Path: "img.tar", Kind: sbom.SourceImage}, "oci-archive:img.tar"},
	}
	for _, c := range cases {
		cmd, err := sbom.Argv("/home/ci/tools/syft-1.54.1/syft", c.t)
		if err != nil {
			t.Fatalf("%s: %v", c.t.Name, err)
		}
		want := []string{"/home/ci/tools/syft-1.54.1/syft", "scan", c.want, "--source-name", c.t.Name,
			"-o", "cyclonedx-json@1.6=.nself-ci/sbom/" + c.t.Name + ".cdx.json",
			"-o", "spdx-json@2.3=.nself-ci/sbom/" + c.t.Name + ".spdx.json", "-q"}
		if !reflect.DeepEqual(cmd.Argv, want) {
			t.Fatalf("%s argv\n got %q\nwant %q", c.t.Name, cmd.Argv, want)
		}
		if cmd.Env["SYFT_CHECK_FOR_APP_UPDATE"] != "false" {
			t.Fatal("the syft update check must be off")
		}
		cdx, spdx := sbom.Outputs(c.t.Name)
		if !reflect.DeepEqual(cmd.Collect, []string{cdx, spdx}) {
			t.Fatalf("collect = %q", cmd.Collect)
		}
	}
}

func TestTargetRefusesUnsafeNamesPathsAndKinds(t *testing.T) {
	bad := []sbom.Target{
		{Name: "", Path: "a", Kind: sbom.SourceFile},
		{Name: "../x", Path: "a", Kind: sbom.SourceFile},
		{Name: "a/b", Path: "a", Kind: sbom.SourceFile},
		{Name: ".hidden", Path: "a", Kind: sbom.SourceFile},
		{Name: "-flag", Path: "a", Kind: sbom.SourceFile},
		{Name: strings.Repeat("a", 129), Path: "a", Kind: sbom.SourceFile},
		{Name: "ok", Path: "", Kind: sbom.SourceFile},
		{Name: "ok", Path: "/etc/passwd", Kind: sbom.SourceFile},
		{Name: "ok", Path: "../escape", Kind: sbom.SourceDir},
		{Name: "ok", Path: "a/../../b", Kind: sbom.SourceDir},
		{Name: "ok", Path: "a", Kind: "registry"},
		{Name: "ok", Path: "a", Kind: ""},
	}
	for _, tg := range bad {
		if _, err := sbom.Argv("syft", tg); !sbom.IsReason(err, sbom.ReasonInvalid) {
			t.Errorf("%+v: got %v, want E718 invalid", tg, err)
		}
	}
}

func TestErrorShape(t *testing.T) {
	err := sbom.Failf(sbom.ReasonToolVersion, "x %d", 1)
	if err.Error() != "E718 tool_version: x 1" {
		t.Fatalf("message = %q", err)
	}
	if sbom.ReasonOf(err) != sbom.ReasonToolVersion || sbom.ReasonOf(nil) != "" {
		t.Fatal("ReasonOf")
	}
	if sbom.IsReason(err, sbom.ReasonInvalid) {
		t.Fatal("reason mismatch matched")
	}
}
