package enroll

// Purpose: the node's enrollment state under <agent state>/pki: the pinned root, the certificate chain,
//          the TLS client key (sealed), the agent URL and when to renew.
// Inputs:  the agent state dir (NSELF_CI_AGENT_HOME) and a JoinResult.
// Outputs: files root.pem, node.crt, client.key.sealed, kek (all 0600, dir 0700) and enrollment.json;
//          LoadCredential returns a tls.Certificate ready for pki.TLSClientConfig.
// Constraints: the TLS client key is written only sealed (AES-256-GCM under <state>/pki/kek, a separate
//          0600 file), never as PEM; this keeps it out of copies and backups of the key file alone, not
//          from a reader running as the same user (the KEK sits in the same directory); every file is replaced atomically (temp file, rename); the state dir
//          must be a real directory owned by the caller.

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

// Enrollment is enrollment.json.
type Enrollment struct {
	NodeID          string `json:"node_id"`
	AgentURL        string `json:"agent_url"`
	RootFingerprint string `json:"root_fingerprint"`
	NotBefore       int64  `json:"not_before"`
	NotAfter        int64  `json:"not_after"`
	EnrolledAt      int64  `json:"enrolled_at"`
}

// Credential is what an enrolled node connects with.
type Credential struct {
	Cert       tls.Certificate
	RootDER    []byte
	Enrollment Enrollment
}

func dir(state string) string { return filepath.Join(state, "pki") }

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// SaveCredential writes the enrollment of r into state.
func SaveCredential(state string, r JoinResult, now time.Time) error {
	if err := os.MkdirAll(dir(state), 0o700); err != nil {
		return err
	}
	if err := checkStateOwner(state); err != nil {
		return err
	}
	if err := securePKIDir(dir(state)); err != nil {
		return err
	}
	kek, err := pki.LoadKEK(state, true)
	if err != nil {
		return err
	}
	sealed, err := pki.Seal(kek, r.KeyDER, "client-key")
	if err != nil {
		return err
	}
	leaf, err := x509.ParseCertificate(r.CertDER)
	if err != nil {
		return err
	}
	meta, err := json.Marshal(Enrollment{NodeID: r.NodeID, AgentURL: r.AgentURL, RootFingerprint: r.RootFingerprint, NotBefore: leaf.NotBefore.Unix(), NotAfter: leaf.NotAfter.Unix(), EnrolledAt: now.Unix()})
	if err != nil {
		return err
	}
	var chain []byte
	for _, der := range r.Chain {
		chain = append(chain, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	// Fixed order, enrollment.json last: a crash before it leaves a node that reads as "never joined".
	files := []struct {
		name string
		data []byte
	}{
		{"root.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: r.RootDER})},
		{"client.key.sealed", sealed},
		{"node.crt", chain},
		{"enrollment.json", meta},
	}
	for _, f := range files {
		if err = writeAtomic(filepath.Join(dir(state), f.name), f.data); err != nil {
			return err
		}
	}
	return nil
}

// LoadCredential reads the enrollment of state; os.ErrNotExist means the node never joined.
func LoadCredential(state string) (Credential, error) {
	read := func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(dir(state), name)) }
	var c Credential
	meta, err := read("enrollment.json")
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(meta, &c.Enrollment); err != nil {
		return c, err
	}
	rootPEM, err := read("root.pem")
	if err != nil {
		return c, err
	}
	block, _ := pem.Decode(rootPEM)
	if block == nil {
		return c, errors.New("enroll: root.pem is not PEM")
	}
	c.RootDER = block.Bytes
	crt, err := read("node.crt")
	if err != nil {
		return c, err
	}
	sealed, err := read("client.key.sealed")
	if err != nil {
		return c, err
	}
	kek, err := pki.LoadKEK(state, false)
	if err != nil {
		return c, err
	}
	keyDER, err := pki.Unseal(kek, sealed, "client-key")
	if err != nil {
		return c, err
	}
	key, err := x509.ParsePKCS8PrivateKey(keyDER)
	if err != nil {
		return c, err
	}
	for rest := crt; ; {
		var b *pem.Block
		if b, rest = pem.Decode(rest); b == nil {
			break
		}
		c.Cert.Certificate = append(c.Cert.Certificate, b.Bytes)
	}
	if len(c.Cert.Certificate) == 0 {
		return c, errors.New("enroll: node.crt holds no certificate")
	}
	c.Cert.PrivateKey = key
	return c, nil
}

// RenewalDue is when the node should renew: 2/3 of the certificate's life.
func (c Credential) RenewalDue() time.Time {
	nb, na := time.Unix(c.Enrollment.NotBefore, 0), time.Unix(c.Enrollment.NotAfter, 0)
	return nb.Add(na.Sub(nb) * 2 / 3)
}
