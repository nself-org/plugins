package enroll

// Purpose: the node's stored enrollment (credential.go): round trip, the sealed TLS key, modes, the
// separate KEK, the renewal point and every way the files can be corrupt or missing.
// Inputs:  a JoinResult from a real CA. Outputs: pass/fail. Constraints: state dirs under t.TempDir().

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

// joined makes a real enrollment result: a node certificate of ttl for a fresh key.
func joined(t *testing.T, ca *pki.CA, name string, ttl time.Duration) JoinResult {
	t.Helper()
	key, csr := csrOf(t)
	iss, err := ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: name, TTL: ttl})
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return JoinResult{NodeID: name, CertDER: iss.CertDER, Chain: iss.Chain, RootDER: iss.RootDER, KeyDER: der, AgentURL: "https://agent.test:3865",
		NotAfter: iss.NotAfter, RootFingerprint: ca.RootFingerprint(), CSRFingerprint: "SHA256:csr"}
}

func TestEnrollCredentialRoundTrip(t *testing.T) {
	ca := newCA(t, nil)
	res := joined(t, ca, "node-a", 0)
	state := filepath.Join(t.TempDir(), "agent")
	now := time.Unix(1_800_000_000, 0)
	if _, err := LoadCredential(state); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a node that never joined: %v", err)
	}
	if err := SaveCredential(state, res, now); err != nil {
		t.Fatal(err)
	}
	c, err := LoadCredential(state)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(res.CertDER)
	if c.Enrollment != (Enrollment{NodeID: "node-a", AgentURL: res.AgentURL, RootFingerprint: res.RootFingerprint, NotBefore: leaf.NotBefore.Unix(), NotAfter: leaf.NotAfter.Unix(), EnrolledAt: now.Unix()}) {
		t.Fatalf("enrollment %+v", c.Enrollment)
	}
	if !bytes.Equal(c.RootDER, res.RootDER) || len(c.Cert.Certificate) != len(res.Chain) || !bytes.Equal(c.Cert.Certificate[0], res.CertDER) || !bytes.Equal(c.Cert.Certificate[1], res.Chain[1]) {
		t.Fatal("certificate material changed in the round trip")
	}
	want, _ := x509.ParsePKCS8PrivateKey(res.KeyDER)
	got, ok := c.Cert.PrivateKey.(*ecdsa.PrivateKey)
	if !ok || !got.Equal(want) || !got.PublicKey.Equal(leaf.PublicKey) {
		t.Fatal("the loaded key is not the TLS client key of the certificate")
	}
	// The loaded pair builds a client config.
	if _, err = pki.TLSClientConfig(pki.ClientOptions{RootDER: c.RootDER, Cert: &c.Cert, ServerName: "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
}

func TestEnrollCredentialIsSealedAndPrivate(t *testing.T) {
	ca := newCA(t, nil)
	res := joined(t, ca, "node-s", 0)
	state := filepath.Join(t.TempDir(), "agent")
	if err := SaveCredential(state, res, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := SaveCredential(state, joined(t, ca, "node-s", 0), time.Now()); err != nil { // a second save over the first
		t.Fatal(err)
	}
	var files []string
	err := filepath.WalkDir(state, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "pki" {
				if st, _ := d.Info(); st.Mode().Perm() != 0o700 {
					t.Errorf("%s mode %v", p, st.Mode().Perm())
				}
			}
			return nil
		}
		st, _ := d.Info()
		if st.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v, want 0600", p, st.Mode().Perm())
		}
		raw, _ := os.ReadFile(p)
		if bytes.Contains(raw, []byte("PRIVATE KEY")) {
			t.Errorf("%s holds a PEM private key", p)
		}
		if bytes.Contains(raw, res.KeyDER) || bytes.Contains(raw, res.KeyDER[len(res.KeyDER)-32:]) {
			t.Errorf("%s holds the raw PKCS8 key", p)
		}
		if strings.HasPrefix(d.Name(), ".tmp-") {
			t.Errorf("temp file %s left behind", p)
		}
		files = append(files, d.Name())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 5 {
		t.Fatalf("files %v, want root.pem node.crt client.key.sealed enrollment.json kek", files)
	}
	if want := pki.KEKPath(state); filepath.Dir(want) != filepath.Join(state, "pki") || filepath.Base(want) != "kek" {
		t.Fatalf("kek path %s", want)
	}
	kek, err := os.ReadFile(pki.KEKPath(state))
	if err != nil || len(kek) != 32 {
		t.Fatalf("kek %d bytes: %v", len(kek), err)
	}
	sealed, _ := os.ReadFile(filepath.Join(state, "pki", "client.key.sealed"))
	if bytes.Contains(sealed, kek) || len(sealed) <= len(res.KeyDER) {
		t.Fatal("the sealed key looks unsealed or holds the KEK")
	}
	kek2, _ := os.ReadFile(pki.KEKPath(state))
	if !bytes.Equal(kek, kek2) {
		t.Fatal("the KEK changed")
	}
	// Replacing: the second save wins, the node id is the new one.
	c, err := LoadCredential(state)
	if err != nil || c.Enrollment.NodeID != "node-s" {
		t.Fatalf("%+v %v", c.Enrollment, err)
	}
}

func TestEnrollCredentialRenewalDue(t *testing.T) {
	c := Credential{Enrollment: Enrollment{NotBefore: 1000, NotAfter: 1000 + 3*3600}}
	if got := c.RenewalDue(); !got.Equal(time.Unix(1000+2*3600, 0)) {
		t.Fatalf("due %s", got)
	}
	ca := newCA(t, nil)
	for _, ttl := range []time.Duration{0, 30 * time.Minute} {
		state := t.TempDir()
		res := joined(t, ca, "node-r", ttl)
		if err := SaveCredential(state, res, time.Now()); err != nil {
			t.Fatal(err)
		}
		cred, err := LoadCredential(state)
		if err != nil {
			t.Fatal(err)
		}
		leaf, _ := x509.ParseCertificate(res.CertDER)
		if got := cred.RenewalDue(); !got.Equal(pki.RenewAt(leaf)) {
			t.Fatalf("ttl %s: node says %s, coordinator says %s", ttl, got, pki.RenewAt(leaf))
		}
		if got := cred.RenewalDue(); !got.After(leaf.NotBefore) || !got.Before(leaf.NotAfter) {
			t.Fatalf("due %s outside the life", got)
		}
	}
}

func TestEnrollCredentialCorruptOrMissing(t *testing.T) {
	ca := newCA(t, nil)
	res := joined(t, ca, "node-c", 0)
	fresh := func() string {
		state := filepath.Join(t.TempDir(), "agent")
		if err := SaveCredential(state, res, time.Now()); err != nil {
			t.Fatal(err)
		}
		return state
	}
	pkiFile := func(state, name string) string { return filepath.Join(state, "pki", name) }
	for _, name := range []string{"enrollment.json", "root.pem", "node.crt", "client.key.sealed", "kek"} {
		state := fresh()
		if err := os.Remove(pkiFile(state, name)); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadCredential(state); err == nil {
			t.Errorf("missing %s: no error", name)
		}
	}
	for _, tc := range []struct {
		name string
		edit func([]byte) []byte
	}{
		{"enrollment.json", func([]byte) []byte { return []byte("{not json") }},
		{"root.pem", func([]byte) []byte { return []byte("not pem") }},
		{"node.crt", func([]byte) []byte { return []byte("-----BEGIN NOTHING-----\n") }},
		{"node.crt", func([]byte) []byte { return nil }},
		{"client.key.sealed", func(b []byte) []byte { b[len(b)-1] ^= 1; return b }},
		{"client.key.sealed", func(b []byte) []byte { return b[:10] }},
		{"client.key.sealed", func([]byte) []byte { return nil }},
		{"kek", func(b []byte) []byte { b[0] ^= 1; return b }},
		{"kek", func(b []byte) []byte { return b[:16] }},
	} {
		state := fresh()
		raw, _ := os.ReadFile(pkiFile(state, tc.name))
		if err := os.WriteFile(pkiFile(state, tc.name), tc.edit(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if c, err := LoadCredential(state); err == nil {
			t.Errorf("corrupt %s: loaded %+v", tc.name, c.Enrollment)
		}
	}
	// A KEK readable by others is refused.
	state := fresh()
	if err := os.Chmod(pkiFile(state, "kek"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCredential(state); err == nil {
		t.Error("a group/world readable KEK was accepted")
	}
}

func TestEnrollCredentialSaveFailures(t *testing.T) {
	ca := newCA(t, nil)
	res := joined(t, ca, "node-f", 0)
	state := filepath.Join(t.TempDir(), "agent")
	bad := res
	bad.CertDER = []byte("garbage")
	if err := SaveCredential(state, bad, time.Now()); err == nil {
		t.Fatal("a certificate that does not parse was saved")
	}
	for _, name := range []string{"client.key.sealed", "enrollment.json", "node.crt", "root.pem"} {
		if _, err := os.Stat(filepath.Join(state, "pki", name)); err == nil {
			t.Errorf("%s written by a failed save", name)
		}
	}
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SaveCredential(file, res, time.Now()); err == nil {
		t.Fatal("a file was accepted as the state dir")
	}
	loose := filepath.Join(t.TempDir(), "agent")
	if err := os.MkdirAll(filepath.Join(loose, "pki"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pki.KEKPath(loose), bytes.Repeat([]byte{7}, 32), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveCredential(loose, res, time.Now()); err == nil {
		t.Fatal("saved under a loose KEK")
	}
}
