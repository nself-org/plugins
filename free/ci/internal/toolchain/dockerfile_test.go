package toolchain

// Purpose: the Dockerfile may not drift from the pins. Every FROM is pinned by
// digest, every pinned version, URL, sha256 and digest appears in it, and
// every download goes through the checksum verifier.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const dockerfilePath = "../../toolchain/Dockerfile"

func readDockerfile(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDockerfileFromLinesPinnedByDigest(t *testing.T) {
	froms := 0
	for _, line := range strings.Split(readDockerfile(t), "\n") {
		if !strings.HasPrefix(line, "FROM ") {
			continue
		}
		froms++
		if !regexp.MustCompile(`@sha256:[0-9a-f]{64}( |$)`).MatchString(line) {
			t.Errorf("FROM is not pinned by digest: %s", line)
		}
	}
	if froms < 2 {
		t.Fatalf("expected a fetch and a final stage, found %d FROM lines", froms)
	}
}

func TestDockerfileMatchesPins(t *testing.T) {
	d := readDockerfile(t)
	for _, p := range loadPinsFromRepo(t) {
		switch p.Kind {
		case KindBaseImage:
			ref := p.Image + ":" + p.Version + "@" + p.Digest
			if n := strings.Count(d, ref); n < 2 {
				t.Errorf("base image %s is used by %d stages, want every stage", ref, n)
			}
		case KindTool:
			arg := strings.ToUpper(strings.ReplaceAll(p.Name, "-", "_"))
			if !strings.Contains(d, "ARG "+arg+"_VERSION="+p.Version+"\n") {
				t.Errorf("%s version %s is not in the Dockerfile", p.Name, p.Version)
			}
			for _, s := range p.Sources {
				suffix := strings.ToUpper(s.Arch)
				// A darwin source belongs to `artifacts tools install`, not to the image.
				if s.OS != "linux" && s.OS != "any" {
					continue
				}
				if !strings.Contains(d, "ARG "+arg+"_URL_"+suffix+"="+s.URL+"\n") || !strings.Contains(d, "ARG "+arg+"_SHA256_"+suffix+"="+s.SHA256+"\n") {
					t.Errorf("%s %s/%s url or sha256 differs from the Dockerfile", p.Name, s.OS, s.Arch)
				}
			}
			if !strings.Contains(d, "fetch-verified.sh "+p.Name+" ") {
				t.Errorf("%s is not downloaded through fetch-verified.sh", p.Name)
			}
		}
	}
}

// TestDockerfileNoUnverifiedDownloads: curl and wget appear only in the verifier.
func TestDockerfileNoUnverifiedDownloads(t *testing.T) {
	for i, line := range strings.Split(readDockerfile(t), "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			continue
		}
		if strings.Contains(trim, "curl ") && !strings.Contains(trim, "apt-get") && !strings.Contains(trim, "install -y") {
			t.Errorf("Dockerfile line %d downloads outside fetch-verified.sh: %s", i+1, trim)
		}
		if strings.Contains(trim, "wget ") || strings.Contains(trim, ":latest") {
			t.Errorf("Dockerfile line %d uses wget or latest: %s", i+1, trim)
		}
	}
	b, err := os.ReadFile("../../toolchain/fetch-verified.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "sha256sum -c -") || !strings.Contains(string(b), "set -eu") {
		t.Error("fetch-verified.sh does not verify the checksum under set -eu")
	}
}
