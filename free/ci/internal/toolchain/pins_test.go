package toolchain

import (
	"strings"
	"testing"
	"testing/fstest"
)

const goodTool = `name: demo
kind: tool
version: "1.2.3"
purpose: a demo tool
owner: internal/gates
probe: [demo, --version]
upstream: {type: github, ref: acme/demo}
sources:
  - {os: linux, arch: amd64, url: "https://example.org/demo.tgz", sha256: "%s"}
`

var hex64 = strings.Repeat("ab", 32)

func load(files map[string]string) ([]Pin, error) {
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys["pins/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	return LoadPins(fsys, "pins")
}

func TestLoadPinsAcceptsAGoodTool(t *testing.T) {
	pins, err := load(map[string]string{"demo.yaml": strings.Replace(goodTool, "%s", hex64, 1)})
	if err != nil || len(pins) != 1 || pins[0].Name != "demo" {
		t.Fatalf("pins %+v err %v", pins, err)
	}
}

func TestLoadPinsRejects(t *testing.T) {
	good := strings.Replace(goodTool, "%s", hex64, 1)
	cases := []struct{ name, file, body, want string }{
		{"short sha", "demo.yaml", strings.Replace(goodTool, "%s", "abc", 1), "sha256"},
		{"upper sha", "demo.yaml", strings.Replace(goodTool, "%s", strings.ToUpper(hex64), 1), "sha256"},
		{"http url", "demo.yaml", strings.Replace(good, "https://", "http://", 1), "https"},
		{"unknown key", "demo.yaml", good + "colour: red\n", "colour"},
		{"name vs file", "other.yaml", good, "file name"},
		{"no version", "demo.yaml", strings.Replace(good, "version: \"1.2.3\"\n", "", 1), "version"},
		{"no sources", "demo.yaml", strings.Split(good, "sources:")[0], "source"},
		{"bad arch", "demo.yaml", strings.Replace(good, "arch: amd64", "arch: sparc", 1), "os/arch"},
		{"bad upstream", "demo.yaml", strings.Replace(good, "type: github", "type: ftp", 1), "upstream"},
		{"tag-only base", "debian.yaml", "name: debian\nkind: base-image\nversion: trixie-slim\npurpose: p\nowner: o\nimage: docker.io/library/debian\nupstream: {type: oci, ref: x}\n", "digest"},
		{"pending with digest", "image.yaml", "name: toolchain-image\nkind: image\npurpose: p\nowner: o\nimage: docker.io/nself/ci-toolchain\nstatus: pending\ndigest: sha256:" + hex64 + "\nupstream: {type: none, ref: ''}\n", "pending"},
		{"pinned without digest", "image.yaml", "name: toolchain-image\nkind: image\npurpose: p\nowner: o\nimage: docker.io/nself/ci-toolchain\nstatus: pinned\nupstream: {type: none, ref: ''}\n", "digest"},
	}
	for _, c := range cases {
		_, err := load(map[string]string{c.file: c.body})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want mention of %q", c.name, err, c.want)
		}
		if err != nil && !strings.HasPrefix(err.Error(), CodeToolchain+":") {
			t.Errorf("%s: error lacks the %s prefix: %v", c.name, CodeToolchain, err)
		}
	}
}

func TestLoadPinsRejectsDuplicatesAndEmpty(t *testing.T) {
	if _, err := load(map[string]string{}); err == nil {
		t.Error("empty directory accepted")
	}
	a := strings.Replace(goodTool, "%s", hex64, 1)
	if _, err := load(map[string]string{"demo.yaml": a, "demo2.yaml": a}); err == nil {
		t.Error("two pins named demo accepted")
	}
}

func TestRenderIsDeterministicAndParses(t *testing.T) {
	pins := loadPinsFromRepo(t)
	a, err := Render(pins)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Render(pins)
	if string(a) != string(b) {
		t.Fatal("Render is not deterministic")
	}
	v, err := ParseVersions(a)
	if err != nil || len(v.Rows) != len(pins) {
		t.Fatalf("parse: %v rows %d want %d", err, len(v.Rows), len(pins))
	}
	if _, err := ParseVersions([]byte("version: 2\nrows: []\n")); err == nil {
		t.Error("version 2 accepted")
	}
	if _, err := ParseVersions([]byte("version: 1\nrows:\n  - name: x\n    kind: tool\n")); err == nil {
		t.Error("invalid row accepted")
	}
}
