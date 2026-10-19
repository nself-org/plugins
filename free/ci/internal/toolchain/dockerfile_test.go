package toolchain

// Purpose: the Dockerfile may not drift from the pins. Every FROM is pinned by
// digest, every pinned version, URL, sha256 and digest appears in it exactly
// once as an ARG, every ARG maps to a pin source, and every download goes
// through the checksum verifier. checkDockerfile is a function so a mutated
// copy proves the check can fail.

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

const dockerfilePath = "../../toolchain/Dockerfile"

var (
	fromDigestRe = regexp.MustCompile(`@sha256:[0-9a-f]{64}( |$)`)
	argRe        = regexp.MustCompile(`(?m)^ARG ([A-Z0-9_]+)=(.*)$`)
)

func readDockerfile(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// checkDockerfile returns every way d disagrees with pins.
func checkDockerfile(d string, pins []Pin) []string {
	var out []string
	froms := 0
	for _, line := range strings.Split(d, "\n") {
		if strings.HasPrefix(line, "FROM ") {
			froms++
			if !fromDigestRe.MatchString(line) {
				out = append(out, "FROM is not pinned by digest: "+line)
			}
		}
	}
	if froms < 2 {
		out = append(out, fmt.Sprintf("expected a fetch and a final stage, found %d FROM lines", froms))
	}
	args := map[string]string{}
	for _, m := range argRe.FindAllStringSubmatch(d, -1) {
		if _, dup := args[m[1]]; dup {
			out = append(out, "ARG "+m[1]+" is declared twice (the later default wins)")
		}
		args[m[1]] = m[2]
	}
	want := map[string]string{}
	for _, p := range pins {
		switch p.Kind {
		case KindBaseImage:
			ref := p.Image + ":" + p.Version + "@" + p.Digest
			if n := strings.Count(d, ref); n < 2 {
				out = append(out, fmt.Sprintf("base image %s is used by %d stages, want every stage", ref, n))
			}
		case KindAptSnapshot:
			want["APT_SNAPSHOT"] = p.Version
		case KindTool:
			name := strings.ToUpper(strings.ReplaceAll(p.Name, "-", "_"))
			want[name+"_VERSION"] = p.Version
			for _, s := range p.Sources {
				if s.OS != "linux" && s.OS != "any" { // darwin sources belong to `artifacts tools install`
					continue
				}
				want[name+"_URL_"+strings.ToUpper(s.Arch)] = s.URL
				want[name+"_SHA256_"+strings.ToUpper(s.Arch)] = s.SHA256
			}
			if !strings.Contains(d, "fetch-verified.sh "+p.Name+" ") {
				out = append(out, p.Name+" is not downloaded through fetch-verified.sh")
			}
		}
	}
	out = append(out, checkAptSnapshot(d)...)
	for k, v := range want {
		if got, ok := args[k]; !ok || got != v {
			out = append(out, fmt.Sprintf("ARG %s is %q, the pins say %q", k, got, v))
		}
	}
	for k := range args {
		if _, ok := want[k]; !ok && k != "BUILDARCH" && k != "TARGETARCH" {
			out = append(out, "ARG "+k+" has no pin source")
		}
	}
	return out
}

// checkAptSnapshot: every RUN that runs apt-get update first points apt at the pinned snapshot.
func checkAptSnapshot(d string) []string {
	var out []string
	for _, run := range strings.Split(strings.ReplaceAll(d, " \\\n", " "), "\n") {
		if !strings.HasPrefix(run, "RUN ") || !strings.Contains(run, "apt-get update") {
			continue
		}
		snap := strings.Index(run, `apt-snapshot.sh "$APT_SNAPSHOT"`)
		if snap < 0 || snap > strings.Index(run, "apt-get update") {
			out = append(out, "apt-get update runs against the live Debian archive, not the pinned snapshot: "+run)
		}
	}
	return out
}

func TestDockerfileMatchesPins(t *testing.T) {
	for _, problem := range checkDockerfile(readDockerfile(t), loadPinsFromRepo(t)) {
		t.Error(problem)
	}
}

// TestDockerfileCheckCanFail: each kind of drift is caught on a mutated copy.
func TestDockerfileCheckCanFail(t *testing.T) {
	d, pins := readDockerfile(t), loadPinsFromRepo(t)
	cases := []struct{ name, text, want string }{
		{"duplicate ARG", d + "\nARG SYFT_URL_AMD64=https://example.org/other.tgz\n", "declared twice"},
		{"orphan ARG", d + "\nARG GHOST_URL_AMD64=https://example.org/ghost.tgz\n", "no pin source"},
		{"changed sha", strings.Replace(d, "ARG GITLEAKS_SHA256_AMD64=55", "ARG GITLEAKS_SHA256_AMD64=66", 1), "GITLEAKS_SHA256_AMD64"},
		{"unpinned FROM", strings.Replace(d, "FROM docker.io/library/debian:trixie-slim@", "FROM docker.io/library/debian:trixie-slim #", 1), "not pinned by digest"},
		{"unverified download", strings.Replace(d, "fetch-verified.sh trivy ", "curl -O trivy ", 1), "trivy is not downloaded"},
	}
	for _, c := range cases {
		if c.text == d {
			t.Fatalf("%s: the mutation changed nothing", c.name)
		}
		found := false
		for _, p := range checkDockerfile(c.text, pins) {
			found = found || strings.Contains(p, c.want)
		}
		if !found {
			t.Errorf("%s: drift not detected (want a problem mentioning %q)", c.name, c.want)
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
