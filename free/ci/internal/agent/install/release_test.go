package install

// Purpose: tests of the signed Release source (P7-NODE-15): a fixture release server on loopback serves a signed agent; the source
//          downloads, verifies and places it, and refuses every unsafe case before anything is placed at the final path.
// Inputs:  keys generated here; httptest servers on 127.0.0.1.
// Constraints: no real network; no real key; the "binary" is a header-only ELF.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

var linuxAMD64 = lifecycle.Platform{OS: "linux", Arch: "amd64"}

func elfBytes(m elf.Machine) []byte {
	h := make([]byte, 64)
	copy(h, "\x7fELF")
	h[4], h[5], h[6] = 2, 1, 1
	binary.LittleEndian.PutUint16(h[16:], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(h[18:], uint16(m))
	binary.LittleEndian.PutUint32(h[20:], 1)
	binary.LittleEndian.PutUint16(h[52:], 64)
	return h
}

type fixture struct {
	signer   signing.Signer
	verifier *verify.Verifier
	hits     atomic.Int32
	srv      *httptest.Server
	binary   []byte
	manifest verify.Manifest
}

// newFixture serves a release whose manifest is signed for version 1.5.0 and the bytes in body.
func newFixture(t *testing.T, body []byte) *fixture {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{signer: signing.NewEd25519Signer(signing.PurposeAgent, priv), binary: body}
	f.verifier, err = verify.New([]signing.Key{{ID: f.signer.KeyID(), Purpose: signing.PurposeAgent, Public: pub}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	f.manifest = verify.Manifest{Schema: verify.Schema, Version: "1.5.0", Target: "linux-amd64", SHA256: hex.EncodeToString(sum[:]),
		ProtocolMin: 1, ProtocolMax: 1, KeyID: f.signer.KeyID(), Alg: "ed25519"}
	in, _ := verify.SigningInput(f.manifest)
	raw, _ := f.signer.Sign(in)
	f.manifest.Sig = signing.EncodeSig(raw)
	mux := http.NewServeMux()
	mux.HandleFunc("/nself-ci-agent-linux-amd64", func(w http.ResponseWriter, _ *http.Request) {
		f.hits.Add(1)
		_, _ = w.Write(f.binary)
	})
	mux.HandleFunc("/nself-ci-agent-linux-amd64.manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		f.hits.Add(1)
		_ = json.NewEncoder(w).Encode(f.manifest)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fixture) release(dir string) Release {
	return Release{Version: "1.5.0", BaseURL: f.srv.URL, Verifier: f.verifier, Dir: dir}
}

func TestReleaseDownloadsVerifiesAndPlaces(t *testing.T) {
	f := newFixture(t, elfBytes(elf.EM_X86_64))
	dir := t.TempDir()
	path, cleanup, err := f.release(dir).Fetch(context.Background(), linuxAMD64)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup != nil {
		t.Fatal("a cache Dir keeps the binary; no cleanup expected")
	}
	if want := filepath.Join(dir, "1.5.0", "linux-amd64", "nself-ci-agent"); path != want {
		t.Fatalf("placed at %s, want %s", path, want)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("placed file: %v mode %v", err, st)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("staging leftovers beside the binary: %v", entries)
	}
}

func TestReleaseWithoutDirUsesATemporaryDirectory(t *testing.T) {
	f := newFixture(t, elfBytes(elf.EM_X86_64))
	path, cleanup, err := f.release("").Fetch(context.Background(), linuxAMD64)
	if err != nil || cleanup == nil {
		t.Fatalf("fetch: %v cleanup %v", err, cleanup != nil)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cleanup must remove the binary: %v", err)
	}
}

func TestReleaseRefusesWhatFailsVerificationAndPlacesNothing(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(f *fixture)
		reason string
	}{
		{"tampered binary", func(f *fixture) { f.binary = append([]byte(nil), f.binary...); f.binary[8] ^= 1 }, verify.ReasonChecksumMismatch},
		{"signature of another version", func(f *fixture) { f.manifest.Version = "1.4.0" }, verify.ReasonBadSignature},
		{"signature removed", func(f *fixture) { f.manifest.Sig = "" }, verify.ReasonMissingSignature},
	}
	for _, c := range cases {
		f := newFixture(t, elfBytes(elf.EM_X86_64))
		c.mutate(f)
		dir := t.TempDir()
		_, _, err := f.release(dir).Fetch(context.Background(), linuxAMD64)
		if verify.ReasonOf(err) != c.reason || lifecycle.CodeOf(err) != "E665" {
			t.Fatalf("%s: want E665 %s, got %v", c.name, c.reason, err)
		}
		if _, serr := os.Stat(filepath.Join(dir, "1.5.0", "linux-amd64", "nself-ci-agent")); !os.IsNotExist(serr) {
			t.Fatalf("%s: a refused artifact was placed", c.name)
		}
	}
}

func TestReleaseRefusesABinaryForAnotherArchitecture(t *testing.T) {
	f := newFixture(t, elfBytes(elf.EM_AARCH64)) // signed correctly, but not an amd64 image
	_, _, err := f.release(t.TempDir()).Fetch(context.Background(), linuxAMD64)
	if lifecycle.CodeOf(err) != lifecycle.CodeInvalid {
		t.Fatalf("want E651 for a wrong-architecture binary, got %v", err)
	}
}

func TestReleaseWithoutTrustedKeyFailsBeforeAnyDownload(t *testing.T) {
	f := newFixture(t, elfBytes(elf.EM_X86_64))
	empty, _ := verify.New(nil, nil)
	r := f.release(t.TempDir())
	r.Verifier = empty
	_, _, err := r.Fetch(context.Background(), linuxAMD64)
	if verify.ReasonOf(err) != verify.ReasonNoTrustedKey || !strings.Contains(err.Error(), "--agent-binary") {
		t.Fatalf("want E665 no_trusted_key naming --agent-binary, got %v", err)
	}
	if f.hits.Load() != 0 {
		t.Fatal("nothing may be downloaded without a trusted key")
	}
	// The compiled-in set is empty until P7-NODE-29: the default Verifier behaves the same.
	r.Verifier = nil
	if _, _, err = r.Fetch(context.Background(), linuxAMD64); verify.ReasonOf(err) != verify.ReasonNoTrustedKey {
		t.Fatalf("default key set: %v", err)
	}
}

func TestReleaseURLRules(t *testing.T) {
	f := newFixture(t, elfBytes(elf.EM_X86_64))
	for _, base := range []string{"http://example.com/rel", "ftp://127.0.0.1/rel", "https://" + "user" + ":" + "pw" + "@example.com/rel", "file:///tmp/rel", "https://"} {
		r := f.release(t.TempDir())
		r.BaseURL = base
		if _, _, err := r.Fetch(context.Background(), linuxAMD64); lifecycle.CodeOf(err) != lifecycle.CodeInvalid {
			t.Errorf("%q: want E651, got %v", base, err)
		}
	}
	for _, v := range []string{"", "v1.5.0", "1.5", "1.5.0/../x"} {
		r := f.release(t.TempDir())
		r.Version = v
		if _, _, err := r.Fetch(context.Background(), linuxAMD64); lifecycle.CodeOf(err) != lifecycle.CodeInvalid {
			t.Errorf("version %q: want E651, got %v", v, err)
		}
	}
}

func TestReleaseRefusesARedirectToPlainHTTPElsewhere(t *testing.T) {
	f := newFixture(t, elfBytes(elf.EM_X86_64))
	redirect := httptest.NewServer(http.RedirectHandler("http://example.com/evil", http.StatusFound))
	t.Cleanup(redirect.Close)
	r := f.release(t.TempDir())
	r.BaseURL = redirect.URL
	if _, _, err := r.Fetch(context.Background(), linuxAMD64); lifecycle.CodeOf(err) != lifecycle.CodeNoRoute {
		t.Fatalf("want E656 for a refused redirect, got %v", err)
	}
}

func TestReleaseServerErrorsAndOversizeBodies(t *testing.T) {
	missing := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(missing.Close)
	f := newFixture(t, elfBytes(elf.EM_X86_64))
	r := f.release(t.TempDir())
	r.BaseURL = missing.URL
	if _, _, err := r.Fetch(context.Background(), linuxAMD64); lifecycle.CodeOf(err) != lifecycle.CodeNoRoute {
		t.Fatalf("404: want E656, got %v", err)
	}
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		buf := make([]byte, 1<<20)
		for i := 0; i <= verify.MaxManifestBytes/len(buf)+1; i++ {
			_, _ = w.Write(buf)
		}
	}))
	t.Cleanup(big.Close)
	r.BaseURL = big.URL
	if _, _, err := r.Fetch(context.Background(), linuxAMD64); lifecycle.CodeOf(err) != lifecycle.CodeNoRoute {
		t.Fatalf("oversize manifest: want E656, got %v", err)
	}
}
