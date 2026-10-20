package update

// Purpose: shared fixtures of the update tests: real stub agent binaries (built once per version), a signed release server on
//          loopback that the real install.Release downloads from, and a sim node whose "ssh" runs the commands in a temp home.
// Inputs:  the go toolchain on PATH (the stubs are built with it); keys generated here.
// Constraints: no network beyond 127.0.0.1; no real key; every directory is under t.TempDir or the TestMain dir.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/plugins/free/ci/internal/agent/install"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/provenance"
)

var hostPlatform = lifecycle.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}

// stubSrc is the stub agent: `version` prints the release line (exit 1 when <state>/broken-<v> exists), `slots` prints the slot
// table line with <state>/busy as the used count.
const stubSrc = `package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var v = "0.0.0"

func main() {
	state := os.Getenv("STUB_STATE")
	if state == "" {
		state = filepath.Join(".nself", "ci-agent")
	}
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		if _, err := os.Stat(filepath.Join(state, "broken-"+v)); err == nil {
			os.Exit(1)
		}
		fmt.Printf("nself-ci-agent %s %s/%s\n", v, runtime.GOOS, runtime.GOARCH)
	case "slots":
		b, _ := os.ReadFile(filepath.Join(state, "busy"))
		n := strings.TrimSpace(string(b))
		if n == "" {
			n = "0"
		}
		fmt.Printf("cap 2 used %s\n", n)
	}
}
`

var (
	stubDir  string
	stubMu   sync.Mutex
	stubDone = map[string]string{}
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "update-stubs-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	stubDir = dir
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// stubBinary builds (once) and returns the path of the stub that reports version v.
func stubBinary(t testing.TB, v string) string {
	t.Helper()
	stubMu.Lock()
	defer stubMu.Unlock()
	if p, ok := stubDone[v]; ok {
		return p
	}
	src := filepath.Join(stubDir, "src-"+v)
	if err := os.MkdirAll(src, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"main.go": stubSrc, "go.mod": "module stub\n\ngo 1.21\n"} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(stubDir, "agent-"+v)
	cmd := exec.Command("go", "build", "-ldflags", "-X main.v="+v, "-o", out, ".")
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "GOFLAGS=", "CGO_ENABLED=0", "GOWORK=off")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build stub %s: %v\n%s", v, err, b)
	}
	stubDone[v] = out
	return out
}

// release is a signed release server for one version of the stub.
type release struct {
	URL      string
	Verifier *verify.Verifier
	Version  string
}

// newRelease serves stub version v signed with a fresh key, speaking protocol pmin..pmax.
func newRelease(t testing.TB, v string, pmin, pmax int) *release {
	t.Helper()
	body, err := os.ReadFile(stubBinary(t, v))
	if err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer := signing.NewEd25519Signer(signing.PurposeAgent, priv)
	ver, err := verify.New([]signing.Key{{ID: signer.KeyID(), Purpose: signing.PurposeAgent, Public: pub}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	m := verify.Manifest{Schema: verify.Schema, Version: v, Target: verify.Target(hostPlatform.OS, hostPlatform.Arch), SHA256: hex.EncodeToString(sum[:]),
		ProtocolMin: pmin, ProtocolMax: pmax, KeyID: signer.KeyID(), Alg: "ed25519"}
	in, _ := verify.SigningInput(m)
	raw, _ := signer.Sign(in)
	m.Sig = signing.EncodeSig(raw)
	name := verify.BinaryName(m.Target)
	digest := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	cdx, spdx := []byte(`{"bomFormat":"CycloneDX","specVersion":"1.6","metadata":{"component":{"name":"`+name+`"}}}`), []byte(`{"spdxVersion":"SPDX-2.3","name":"`+name+`"}`)
	st, err := provenance.BuildAgent(provenance.AgentFacts{Repo: "nself-org/plugins", Ref: "refs/tags/v" + v, Revision: strings.Repeat("a", 40), RunID: "1",
		Subjects: []provenance.FileDigest{{Name: name, SHA256: m.SHA256}},
		SBOMs:    []provenance.FileDigest{{Name: name + verify.SuffixCycloneDX, SHA256: digest(cdx)}, {Name: name + verify.SuffixSPDX, SHA256: digest(spdx)}}})
	if err != nil {
		t.Fatal(err)
	}
	env, err := signing.SignEnvelope(signer, provenance.PayloadType, st)
	if err != nil {
		t.Fatal(err)
	}
	prov, _ := json.Marshal(env)
	mj, _ := json.Marshal(m)
	files := map[string][]byte{"": body, ".manifest.json": mj, verify.SuffixCycloneDX: cdx, verify.SuffixSPDX: spdx, verify.SuffixProvenance: prov}
	mux := http.NewServeMux()
	for suffix, b := range files {
		mux.HandleFunc("/"+name+suffix, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(b) })
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &release{URL: srv.URL, Verifier: ver, Version: v}
}

func (r *release) source() ReleaseSource {
	return ReleaseSource{Release: releaseOf(r)}
}

// sim is a node home on this machine: commands run with it as the working directory, like an ssh session in the user's home.
type sim struct {
	t    testing.TB
	Home string
}

func newSim(t testing.TB, installed string) *sim {
	t.Helper()
	s := &sim{t: t, Home: t.TempDir()}
	if err := os.MkdirAll(filepath.Join(s.Home, agentDir, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	s.install(installed)
	return s
}

func (s *sim) state() string { return filepath.Join(s.Home, agentDir) }
func (s *sim) bin() string   { return filepath.Join(s.Home, agentBin) }

func (s *sim) install(v string) {
	b, err := os.ReadFile(stubBinary(s.t, v))
	if err != nil {
		s.t.Fatal(err)
	}
	_ = os.Remove(s.bin())
	if err := os.WriteFile(s.bin(), b, 0o755); err != nil {
		s.t.Fatal(err)
	}
}

func (s *sim) flag(name, content string) {
	if err := os.WriteFile(filepath.Join(s.state(), name), []byte(content), 0o600); err != nil {
		s.t.Fatal(err)
	}
}

func (s *sim) clear(name string) { _ = os.Remove(filepath.Join(s.state(), name)) }

// version runs the installed stub like the engine's hello does.
func (s *sim) version(ctx context.Context) (string, error) {
	out, err := s.Run(ctx, remote.Target{}, agentBin, "version")
	if err != nil {
		return "", err
	}
	v, _ := ParseVersionLine(out)
	return v, nil
}

// Run executes argv in the sim home (the remote command line, one argument per element).
func (s *sim) Run(ctx context.Context, _ remote.Target, argv ...string) (string, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = s.Home
	out, err := cmd.Output()
	return string(out), err
}

// Copy puts a local file at remotePath under the sim home.
func (s *sim) Copy(_ context.Context, _ remote.Target, local, remotePath string) error {
	in, err := os.Open(local)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(filepath.Join(s.Home, remotePath), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func releaseOf(r *release) install.Release {
	return install.Release{Version: r.Version, BaseURL: r.URL, Verifier: r.Verifier}
}
