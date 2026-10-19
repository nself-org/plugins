package main

// Purpose: end-to-end test of the default install source (P7-NODE-15): `nodes add` without --agent-binary downloads the pinned
//          release from a local fixture server, verifies it and installs it on a simulated node; a tampered download is refused
//          and with no key compiled in the add fails naming --agent-binary.
// Inputs:  a loopback httptest release server, a generated Ed25519 key, fake ssh and scp seams, a real store.
// Constraints: no network beyond loopback; no real key; the agent "binary" is a header-only ELF.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"debug/elf"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/nodes/probe"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
	_ "modernc.org/sqlite"
)

func simELF() []byte {
	h := make([]byte, 64)
	copy(h, "\x7fELF")
	h[4], h[5], h[6] = 2, 1, 1
	binary.LittleEndian.PutUint16(h[16:], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(h[18:], uint16(elf.EM_X86_64))
	binary.LittleEndian.PutUint32(h[20:], 1)
	binary.LittleEndian.PutUint16(h[52:], 64)
	return h
}

type simNode struct {
	mu     sync.Mutex
	copied map[string][]byte // remote path -> bytes
	runs   [][]string
}

type simProber struct{ r *registry.Registry }

func (p simProber) Probe(ctx context.Context, id string) (registry.Node, error) {
	return p.r.Get(ctx, id)
}

type simSeparation struct{}

func (simSeparation) Detect(context.Context, func(context.Context, ...string) (string, error), lifecycle.Platform) lifecycle.SeparationFacts {
	return lifecycle.SeparationFacts{UIDSeparation: true, UserNS: true, Reason: "userns.ok"}
}

// simDeps wires lifecycle.Deps to an in-process node with no agent installed.
func simDeps(t *testing.T) (*lifecycle.Deps, *simNode) {
	t.Helper()
	home := t.TempDir()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	n := &simNode{copied: map[string][]byte{}}
	d := lifecycle.NewDeps(st, home, "proj-1", "cli:test")
	d.Stdin, d.Stdout, d.Stderr, d.TTY = strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, false
	d.SSHVersion = func(context.Context) (remote.Version, error) { return remote.Version{}, nil }
	blob := append([]byte{0, 0, 0, 11}, "ssh-ed25519"...)
	blob = append(blob, 0, 0, 0, 32)
	blob = append(blob, bytes.Repeat([]byte{7}, 32)...)
	sum := sha256.Sum256(blob)
	fp := "SHA256:" + b64raw(sum[:])
	key := remote.HostKey{Type: "ssh-ed25519", Fingerprint: fp, Line: "node ssh-ed25519 " + b64std(blob)}
	d.Scan = func(context.Context, remote.Target, string) ([]remote.HostKey, error) {
		return []remote.HostKey{key}, nil
	}
	d.Run = func(_ context.Context, _ remote.Target, argv ...string) (string, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		n.runs = append(n.runs, argv)
		switch {
		case argv[0] == "uname":
			return "Linux x86_64\n", nil
		case argv[0] == "test":
			if _, ok := n.copied[".nself/ci-agent/bin/nself-ci-agent"]; !ok {
				return "", os.ErrNotExist
			}
		case argv[0] == "mv" && len(argv) == 4:
			n.copied[argv[3]] = n.copied[argv[2]]
		case argv[len(argv)-1] == "version":
			return "nself-ci-agent 1.5.0 linux/amd64\n", nil
		}
		return "", nil
	}
	d.Copy = func(_ context.Context, _ remote.Target, local, remotePath string) error {
		b, rerr := os.ReadFile(local)
		n.mu.Lock()
		defer n.mu.Unlock()
		n.copied[remotePath] = b
		return rerr
	}
	d.NewProber = func(r *registry.Registry, _ string, _ remote.Version) probe.Prober { return simProber{r} }
	d.Separation = simSeparation{}
	return d, n
}

func releaseServer(t *testing.T, body []byte, tamper bool) (*httptest.Server, *verify.Verifier, *atomic.Int32) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s := signing.NewEd25519Signer(signing.PurposeAgent, priv)
	v, err := verify.New([]signing.Key{{ID: s.KeyID(), Purpose: signing.PurposeAgent, Public: pub}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	m := verify.Manifest{Schema: verify.Schema, Version: "1.5.0", Target: "linux-amd64", SHA256: hex.EncodeToString(sum[:]), ProtocolMin: 1, ProtocolMax: 1, KeyID: s.KeyID(), Alg: "ed25519"}
	in, _ := verify.SigningInput(m)
	raw, _ := s.Sign(in)
	m.Sig = signing.EncodeSig(raw)
	served := append([]byte(nil), body...)
	if tamper {
		served[10] ^= 0xff
	}
	hits := &atomic.Int32{}
	mux := http.NewServeMux()
	mux.HandleFunc("/nself-ci-agent-linux-amd64", func(w http.ResponseWriter, _ *http.Request) { hits.Add(1); _, _ = w.Write(served) })
	mux.HandleFunc("/nself-ci-agent-linux-amd64.manifest.json", func(w http.ResponseWriter, _ *http.Request) { hits.Add(1); _ = json.NewEncoder(w).Encode(m) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, v, hits
}

func pinForTest(t *testing.T, url string, v func() (*verify.Verifier, error)) {
	t.Helper()
	oldVer, oldPin := agentVerifier, agentPinnedVersion
	agentVerifier, agentPinnedVersion = v, "1.5.0"
	t.Cleanup(func() { agentVerifier, agentPinnedVersion = oldVer, oldPin })
	t.Setenv("NSELF_CI_AGENT_RELEASE_URL", url)
	t.Setenv("NSELF_CI_HOME", t.TempDir())
}

func addNoBinary(t *testing.T, d *lifecycle.Deps) error {
	t.Helper()
	fp := ""
	keys, _ := d.Scan(context.Background(), remote.Target{}, "")
	fp = keys[0].Fingerprint
	_, err := d.Add(context.Background(), lifecycle.AddOptions{Dest: "ci@sim", HostKey: fp, Trust: lifecycle.DefaultTrust()})
	return err
}

func TestNodesAddWithoutAgentBinaryInstallsTheVerifiedRelease(t *testing.T) {
	body := simELF()
	srv, v, hits := releaseServer(t, body, false)
	pinForTest(t, srv.URL, func() (*verify.Verifier, error) { return v, nil })
	d, node := simDeps(t)
	if err := addNoBinary(t, d); err != nil {
		t.Fatalf("add: %v", err)
	}
	got := node.copied[".nself/ci-agent/bin/nself-ci-agent"]
	if !bytes.Equal(got, body) {
		t.Fatalf("the node got %d bytes, want the verified release (%d)", len(got), len(body))
	}
	if hits.Load() != 2 {
		t.Fatalf("expected one manifest and one binary request, got %d", hits.Load())
	}
}

func TestNodesAddRefusesATamperedReleaseAndRegistersNothing(t *testing.T) {
	srv, v, _ := releaseServer(t, simELF(), true)
	pinForTest(t, srv.URL, func() (*verify.Verifier, error) { return v, nil })
	d, node := simDeps(t)
	err := addNoBinary(t, d)
	if lifecycle.CodeOf(err) != "E665" || verify.ReasonOf(err) != verify.ReasonChecksumMismatch {
		t.Fatalf("want E665 checksum_mismatch, got %v", err)
	}
	if len(node.copied) != 0 {
		t.Fatalf("nothing may reach the node: %v", node.copied)
	}
	if nodes, _ := d.Registry.List(context.Background()); len(nodes) != 0 {
		t.Fatalf("a failed install registers no node: %v", nodes)
	}
}

func TestNodesAddWithNoKeyCompiledInNamesAgentBinary(t *testing.T) {
	srv, _, hits := releaseServer(t, simELF(), false)
	pinForTest(t, srv.URL, verify.Default) // the committed key set: empty until P7-NODE-29
	d, node := simDeps(t)
	err := addNoBinary(t, d)
	if lifecycle.CodeOf(err) != "E665" || verify.ReasonOf(err) != verify.ReasonNoTrustedKey || !strings.Contains(err.Error(), "--agent-binary") {
		t.Fatalf("want E665 no_trusted_key naming --agent-binary, got %v", err)
	}
	if hits.Load() != 0 || len(node.copied) != 0 {
		t.Fatalf("nothing may be downloaded or installed: hits %d copied %v", hits.Load(), node.copied)
	}
}

func b64std(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
func b64raw(b []byte) string { return base64.RawStdEncoding.EncodeToString(b) }
