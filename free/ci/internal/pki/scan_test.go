package pki

// Purpose: the filesystem scan of the ticket: after issue, renew, rotate, token minting and revoke, no
//          unsealed private key is on disk (data dir, SQLite file, WAL), the KEK file is 0600 and its
//          bytes appear nowhere else, and no error text carries key material.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// scanFile reports what looks like an unsealed private key in b, and every needle found.
func scanFile(b []byte, needles map[string][]byte) (found []string) {
	if bytes.Contains(b, []byte("PRIVATE KEY")) {
		found = append(found, "PEM PRIVATE KEY marker")
	}
	for i := 0; i+2 < len(b); i++ {
		if b[i] != 0x30 {
			continue
		}
		l, hdr := int(b[i+1]), 2
		switch {
		case l == 0x81 && i+3 <= len(b):
			l, hdr = int(b[i+2]), 3
		case l == 0x82 && i+4 <= len(b):
			l, hdr = int(b[i+2])<<8|int(b[i+3]), 4
		case l >= 0x80:
			continue
		}
		if end := i + hdr + l; end <= len(b) {
			if _, err := x509.ParsePKCS8PrivateKey(b[i:end]); err == nil {
				found = append(found, "PKCS8 key at offset "+itoa(i))
			}
		}
	}
	for name, n := range needles {
		if len(n) > 0 && bytes.Contains(b, n) {
			found = append(found, name)
		}
	}
	return found
}

func itoa(i int) string { return strconv.Itoa(i) }

// scanDir scans every file under dir except skip and returns "file: finding" lines.
func scanDir(t *testing.T, dir, skip string, needles map[string][]byte) (out []string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == skip {
			return err
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		for _, f := range scanFile(b, needles) {
			out = append(out, filepath.Base(p)+": "+f)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestScannerFindsKeys(t *testing.T) { // the scan has teeth: a positive control per kind
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	noise := bytes.Repeat([]byte{0x30, 0x01, 0xff}, 40)
	cases := map[string][]byte{
		"raw PKCS8 inside noise": append(append(append([]byte{}, noise...), der...), noise...),
		"PEM block":              pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("x")}),
		"PEM PKCS8 block":        pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}),
		"needle":                 append([]byte("xx"), key.D.Bytes()...),
	}
	for name, b := range cases {
		if got := scanFile(b, map[string][]byte{"D": key.D.Bytes()}); len(got) == 0 {
			t.Errorf("%s: not detected", name)
		}
	}
	if got := scanFile(append(noise, []byte("a harmless certificate request")...), nil); len(got) != 0 {
		t.Errorf("false positive: %v", got)
	}
}

func TestNoUnsealedPrivateKeyOnDisk(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	r.addNode("n1")
	kek, _ := LoadKEK(r.dir, false)

	// issue, renew, rotate, tokens, user bundle, server leaf, revoke
	n := r.issueNode("n1", 0)
	r.clk.advance(5 * day)
	csr, renewedKey := newCSR(t)
	if _, err := r.ca.Renew(ctx, n.Cert, csr); err != nil {
		t.Fatal(err)
	}
	b, _, _ := r.issueUser("alice", 0)
	server := r.serverCert([]string{"ci.example.com"}, loopback, 0)
	tk := newTokens(t, r)
	if _, err := tk.Mint(claims(0), time.Hour); err != nil {
		t.Fatal(err)
	}
	pub := edPub(t)
	if err := r.ca.PinNodeKey(ctx, "n1", pub); err != nil {
		t.Fatal(err)
	}
	if err := r.ca.RotateNodeKey(ctx, "cli:admin", "n1", NodeKeyDigest(edPub(t))); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ca.Revoke(ctx, "cli:admin", "n1", "done"); err != nil {
		t.Fatal(err)
	}

	needles := map[string][]byte{
		"KEK (raw)": kek, "KEK (hex)": []byte(hex.EncodeToString(kek)), "KEK (base64)": []byte(base64.StdEncoding.EncodeToString(kek)),
		"node key scalar":     n.Key.D.FillBytes(make([]byte, 32)),
		"renewed key scalar":  renewedKey.D.FillBytes(make([]byte, 32)),
		"server key scalar":   server.PrivateKey.(*ecdsa.PrivateKey).D.FillBytes(make([]byte, 32)),
		"token signing seed":  tk.priv.Seed(),
		"token private key":   []byte(tk.priv),
		"user bundle PEM key": b.KeyPEM,
	}
	needles["root key scalar"] = r.ca.root.key.D.FillBytes(make([]byte, 32))
	for k, a := range r.ca.inter {
		needles[string(k)+" intermediate key scalar"] = a.key.D.FillBytes(make([]byte, 32))
		der, _ := x509.MarshalPKCS8PrivateKey(a.key)
		needles[string(k)+" intermediate PKCS8"] = der
	}
	nodeKeyDER, _ := x509.MarshalPKCS8PrivateKey(n.Key)
	needles["node key PKCS8"] = nodeKeyDER
	kekPath := KEKPath(r.dir)

	check := func(label string) {
		t.Helper()
		if got := scanDir(t, r.dir, kekPath, needles); len(got) != 0 {
			t.Fatalf("%s: unsealed key material on disk: %v", label, got)
		}
		// the KEK file: 0600, exactly the KEK, and nothing else in the data dir is a key file
		st, err := os.Stat(kekPath)
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("%s: kek file %v %v", label, st, err)
		}
		raw, _ := os.ReadFile(kekPath)
		if !bytes.Equal(raw, kek) {
			t.Fatalf("%s: kek file content changed", label)
		}
		var files []string
		_ = filepath.WalkDir(r.dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				rel, _ := filepath.Rel(r.dir, p)
				files = append(files, rel)
			}
			return nil
		})
		sort.Strings(files)
		for _, f := range files {
			switch f {
			case "state.db", "state.db-wal", "state.db-shm", filepath.Join("pki", "kek"):
			default:
				t.Fatalf("%s: unexpected file in the data dir: %s (all: %v)", label, f, files)
			}
		}
	}
	check("live store (db + wal)")
	// the sealed material is really in the database: the scan is not looking at an empty file
	db, _ := os.ReadFile(r.dbPath)
	wal, _ := os.ReadFile(r.dbPath + "-wal")
	if len(db)+len(wal) < 4096 {
		t.Fatalf("database files too small to be a real scan: %d", len(db)+len(wal))
	}
	_ = r.st.Close() // checkpoints the WAL into the main file
	check("closed store")
	if raw, _ := os.ReadFile(r.dbPath); len(raw) < 4096 {
		t.Fatal("main database file is empty after close")
	}
}

func TestErrorsNeverCarryKeyMaterial(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	kek, _ := LoadKEK(r.dir, false)
	tk := newTokens(t, r)
	good, _ := tk.Mint(claims(0), time.Hour)
	pub, other := edPub(t), edPub(t)
	_ = r.ca.PinNodeKey(ctx, "n1", pub)
	csr, key := newCSR(t)
	secrets := [][]byte{kek, tk.priv.Seed(), []byte(tk.priv), key.D.Bytes(), csr, []byte(good), pub, other,
		r.ca.root.key.D.Bytes(), []byte(base64.StdEncoding.EncodeToString(pub)), []byte(base64.StdEncoding.EncodeToString(other))}
	var errs []error
	add := func(_ any, err error) { errs = append(errs, err) }
	errs = append(errs, r.ca.CheckNodeKey(ctx, "n1", other), r.ca.PinNodeKey(ctx, "n1", pub[:5]), r.ca.RotateNodeKey(ctx, "a", "n1", "short"))
	add(tk.Verify(good[:len(good)-4] + "AAAA"))
	add(tk.Verify("a.b"))
	add(tk.Mint(claims(1), time.Hour))
	add(r.ca.IssueNode(ctx, NodeRequest{CSR: csr, NodeID: "BAD ID", TTL: 8 * day}))
	add(r.ca.IssueNode(ctx, NodeRequest{CSR: csr[:len(csr)-3], NodeID: "n1"}))
	add(r.ca.Renew(ctx, &x509.Certificate{}, csr))
	add(CSRKeyFingerprint(csr[:10]))
	add(LoadKEK(t.TempDir(), false))
	// wrong KEK
	if err := os.WriteFile(KEKPath(r.dir), bytes.Repeat([]byte{3}, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	add(r.reload())
	// damaged token key
	r.exec(`UPDATE pki_token_key SET key_sealed=x'0102'`)
	add(r.ca.Tokens(ctx))
	for _, e := range errs {
		if e == nil {
			t.Fatal("an expected failure succeeded")
		}
		txt := e.Error()
		for i, s := range secrets {
			if len(s) >= 4 && (strings.Contains(txt, string(s)) || strings.Contains(txt, hex.EncodeToString(s)) || strings.Contains(txt, base64.StdEncoding.EncodeToString(s)) || strings.Contains(txt, base64.RawURLEncoding.EncodeToString(s))) {
				t.Errorf("error %q carries secret #%d", txt, i)
			}
		}
	}
}
