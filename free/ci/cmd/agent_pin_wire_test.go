package main

// Purpose: P7-WIRE-12. A release build of nself-ci carries the agent pin, so `nself ci nodes add` without --agent-binary
//          takes the signed Release source instead of failing on an empty pin.
// Inputs:  the ldflags scripts/build-tarballs.sh gives the ci plugin (plugin_ldflags), the real binary built with them,
//          a stub ssh on PATH for the add.
// Constraints: the binaries are driven by argv only; nothing in this package is called or replaced (epic rule W1).
//          No network: the node is a stub ssh and no key is compiled in yet (P7-NODE-29), so the Release source stops
//          at E665 no_trusted_key before any download.

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const wire12Pin = "9.87.65"

// wire12Ldflags returns what scripts/build-tarballs.sh passes to -ldflags for plugin at version.
func wire12Ldflags(t *testing.T, plugin, version string) string {
	t.Helper()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	c := exec.Command("bash", "-c", `source scripts/build-tarballs.sh && plugin_ldflags "$1" "$2"`, "wire12", plugin, version)
	c.Dir = root
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin_ldflags: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func wire12Build(t *testing.T, dir, name, ldflags string) string {
	t.Helper()
	bin := filepath.Join(dir, name)
	c := exec.Command("go", "build", "-trimpath", "-ldflags="+ldflags, "-o", bin, ".")
	c.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v: %s", name, err, out)
	}
	return bin
}

type wire12Envelope struct {
	Data struct {
		Checks []struct {
			ID      string `json:"id"`
			Message string `json:"message"`
		} `json:"checks"`
	} `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func wire12Run(t *testing.T, bin, cwd string, env []string, args ...string) wire12Envelope {
	t.Helper()
	c := exec.Command(bin, args...)
	c.Dir = cwd
	c.Env = append(append(os.Environ(), "NSELF_V15=1", "NSELF_CI_HOME="+t.TempDir()), env...)
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	_ = c.Run() // doctor exits 12 on warnings and add fails by design; the envelope is the result
	var e wire12Envelope
	if err := json.Unmarshal(stdout.Bytes(), &e); err != nil {
		t.Fatalf("%v: no JSON envelope (%v)\nstdout: %s\nstderr: %s", args, err, stdout.String(), stderr.String())
	}
	return e
}

// wire12UpgradeCheck is the message of CI-DN013 "agents at the pinned release", which reads the build's pin.
func wire12UpgradeCheck(t *testing.T, bin string) string {
	t.Helper()
	e := wire12Run(t, bin, t.TempDir(), nil, "doctor", "--nodes", "--json")
	for _, c := range e.Data.Checks {
		if c.ID == "CI-DN013" {
			return c.Message
		}
	}
	t.Fatalf("doctor --nodes --json has no CI-DN013 check: %+v", e)
	return ""
}

// wire12FakeNode puts a stub ssh on a PATH directory: a Linux x86_64 host with no agent installed.
// It returns the PATH entry and the SHA256 fingerprint of the host key the scan reports.
func wire12FakeNode(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	blob := append([]byte{0, 0, 0, 11}, "ssh-ed25519"...)
	blob = append(blob, 0, 0, 0, 32)
	blob = append(blob, bytes.Repeat([]byte{7}, 32)...)
	sum := sha256.Sum256(blob)
	fp := "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
	// The host-key scan runs `ssh ... -o UserKnownHostsFile=<tmp> -- <dest> true` and reads the key ssh wrote there.
	ssh := `#!/bin/sh
if [ "$1" = "-V" ]; then echo "OpenSSH_9.6p1, OpenSSL 3.0.13" >&2; exit 0; fi
kh=""
for a in "$@"; do
  case "$a" in
    UserKnownHostsFile=*) kh="${a#UserKnownHostsFile=}" ;;
    uname|*"uname "*|*"'uname'"*) echo "Linux x86_64"; exit 0 ;;
    test|"test "*|*"'test'"*) exit 1 ;;
    true) [ -n "$kh" ] && echo "192.0.2.10 ssh-ed25519 ` + base64.StdEncoding.EncodeToString(blob) + `" >> "$kh"; exit 255 ;;
  esac
done
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(ssh), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, fp
}

func TestWire12PinnedBuildReportsPin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake node is a POSIX shell script")
	}
	release := wire12Ldflags(t, "ci", wire12Pin)
	if !strings.Contains(release, "-X main.agentPinnedVersion="+wire12Pin) {
		t.Fatalf("build-tarballs.sh gives the ci plugin no agent pin: %q", release)
	}
	other := wire12Ldflags(t, "notify", wire12Pin)
	if strings.Contains(other, "agentPinnedVersion") {
		t.Fatalf("only the ci plugin carries the agent pin, notify got %q", other)
	}

	dir := t.TempDir()
	pinned := wire12Build(t, dir, "pinned", release)
	bare := wire12Build(t, dir, "bare", other)

	t.Run("the pin is in the binary and doctor reads it", func(t *testing.T) {
		if b, _ := os.ReadFile(pinned); !bytes.Contains(b, []byte(wire12Pin)) {
			t.Fatalf("the release build does not carry %s", wire12Pin)
		}
		if b, _ := os.ReadFile(bare); bytes.Contains(b, []byte(wire12Pin)) {
			t.Fatalf("the unpinned build carries %s: the probe proves nothing", wire12Pin)
		}
		if got := wire12UpgradeCheck(t, pinned); strings.Contains(got, "no agent release is pinned") {
			t.Fatalf("the release build reports no pin: %q", got)
		}
		if got := wire12UpgradeCheck(t, bare); !strings.Contains(got, "no agent release is pinned") {
			t.Fatalf("control: the unpinned build should report no pin, got %q", got)
		}
	})

	t.Run("nodes add without --agent-binary picks the Release source", func(t *testing.T) {
		path, fp := wire12FakeNode(t)
		env := []string{"PATH=" + path + string(os.PathListSeparator) + os.Getenv("PATH")}
		e := wire12Run(t, pinned, t.TempDir(), env, "nodes", "add", "ci@192.0.2.10", "--host-key", fp, "--project", "proj-1", "--json")
		if e.Error == nil {
			t.Fatalf("add succeeded with no trusted key compiled in: %+v", e)
		}
		// E665 is the Release source's verifier: the default install ran. The usage error "the agent is not installed"
		// would mean no default source was registered.
		if e.Error.Code != "E665" || !strings.Contains(e.Error.Message, "--agent-binary") || strings.Contains(e.Error.Message, "not installed on") {
			t.Fatalf("want E665 from the Release source naming --agent-binary, got %s: %s", e.Error.Code, e.Error.Message)
		}
	})
}
