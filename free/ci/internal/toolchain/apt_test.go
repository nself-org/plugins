package toolchain

// Purpose: the OS packages of the toolchain image come from one dated
// snapshot.debian.org source, pinned in pins/apt.yaml (D-0994). Two builds from
// the same base digest must install the same package bytes.
// Covers: the pin row, the Dockerfile wiring (checkDockerfile), the script that
// writes the apt sources, and a mutation proving a live-archive install fails.

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const aptScript = "../../toolchain/apt-snapshot.sh"

var aptStampRe = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z$`)

func TestAptSnapshotIsPinned(t *testing.T) {
	var apt *Pin
	for _, p := range loadPinsFromRepo(t) {
		if p.Kind == KindAptSnapshot {
			p := p
			apt = &p
		}
	}
	if apt == nil {
		t.Fatal("no apt-snapshot pin: the image installs OS packages from the live Debian archive")
	}
	if !aptStampRe.MatchString(apt.Version) {
		t.Errorf("apt-snapshot version %q is not a snapshot.debian.org timestamp", apt.Version)
	}
	v, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if row, ok := v.Row(apt.Name); !ok || row.Version != apt.Version {
		t.Errorf("versions.yaml row %+v does not carry the apt snapshot pin %+v", row, apt)
	}
}

func TestLoadPinsRejectsABadAptSnapshot(t *testing.T) {
	row := func(version string) map[string]string {
		return map[string]string{"apt.yaml": "name: apt\nkind: apt-snapshot\nversion: \"" + version + "\"\npurpose: p\nowner: o\nupstream: {type: none, ref: \"\"}\n"}
	}
	if _, err := load(row("20261005T000000Z")); err != nil {
		t.Fatalf("a valid snapshot stamp is refused: %v", err)
	}
	for _, bad := range []string{"", "latest", "2026-10-05", "20261005"} {
		if _, err := load(row(bad)); err == nil || !strings.Contains(err.Error(), "snapshot") {
			t.Errorf("stamp %q accepted or wrong error: %v", bad, err)
		}
	}
}

// TestDockerfileAptComesFromTheSnapshot: a mutated Dockerfile that runs apt-get
// against the live archive is flagged by checkDockerfile.
func TestDockerfileAptComesFromTheSnapshot(t *testing.T) {
	d, pins := readDockerfile(t), loadPinsFromRepo(t)
	live := strings.ReplaceAll(d, `apt-snapshot.sh "$APT_SNAPSHOT" && `, "")
	if live == d {
		t.Fatal("the Dockerfile does not call apt-snapshot.sh before apt-get")
	}
	found := false
	for _, p := range checkDockerfile(live, pins) {
		found = found || strings.Contains(p, "apt-get update")
	}
	if !found {
		t.Error("an apt-get update against the live archive is not reported")
	}
	stale := strings.Replace(d, "ARG APT_SNAPSHOT=", "ARG APT_SNAPSHOT=19990101T000000Z#", 1)
	found = false
	for _, p := range checkDockerfile(stale, pins) {
		found = found || strings.Contains(p, "APT_SNAPSHOT")
	}
	if !found {
		t.Error("a Dockerfile snapshot stamp that differs from the pin is not reported")
	}
}

func runAptScript(t *testing.T, stamp string) (dir string, out []byte, err error) {
	t.Helper()
	dir = t.TempDir()
	cmd := exec.Command("sh", aptScript, stamp)
	cmd.Env = append(os.Environ(), "APT_DIR="+dir)
	out, err = cmd.CombinedOutput()
	return dir, out, err
}

func TestAptSnapshotScriptWritesSignedDatedSources(t *testing.T) {
	dir, out, err := runAptScript(t, "20261005T000000Z")
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	b, err := os.ReadFile(filepath.Join(dir, "sources.list.d", "debian.sources"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, want := range []string{
		"snapshot.debian.org/archive/debian/20261005T000000Z/",
		"snapshot.debian.org/archive/debian-security/20261005T000000Z/",
		"Suites: trixie trixie-updates",
		"Suites: trixie-security",
		"Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("debian.sources lacks %q:\n%s", want, src)
		}
	}
	if strings.Contains(src, "deb.debian.org") || strings.Contains(src, "security.debian.org") {
		t.Errorf("debian.sources still names the live archive:\n%s", src)
	}
	conf, err := os.ReadFile(filepath.Join(dir, "apt.conf.d", "99snapshot"))
	if err != nil || !strings.Contains(string(conf), `Check-Valid-Until "false"`) {
		t.Errorf("apt.conf.d/99snapshot = %q, %v (a dated Release file is past its Valid-Until)", conf, err)
	}
}

func TestAptSnapshotScriptRefusesABadStamp(t *testing.T) {
	for _, bad := range []string{"", "latest", "20261005", "20261005T000000Z/../x", "x;y"} {
		dir, _, err := runAptScript(t, bad)
		if err == nil {
			t.Errorf("stamp %q accepted", bad)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "sources.list.d")); statErr == nil {
			t.Errorf("stamp %q wrote sources before failing", bad)
		}
	}
}
