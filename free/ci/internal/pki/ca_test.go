package pki

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

const day = 24 * time.Hour

func TestNewRequiresStoreAndDataDir(t *testing.T) {
	r := newRig(t)
	for _, o := range []Options{{}, {Store: r.st}, {DataDir: r.dir}} {
		if _, err := New(context.Background(), o); err == nil {
			t.Fatalf("options %+v accepted", o)
		}
	}
}

func TestBootstrapHierarchy(t *testing.T) {
	r := newRig(t)
	rows, err := r.st.PKIAuthorities(context.Background())
	if err != nil || len(rows) != 4 {
		t.Fatalf("authorities: %d %v", len(rows), err)
	}
	var kinds []string
	for _, a := range rows {
		kinds = append(kinds, a.Kind)
	}
	if strings.Join(kinds, ",") != "coordinator,node,root,user" {
		t.Fatalf("kinds %v", kinds)
	}
	st, err := os.Stat(KEKPath(r.dir))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("kek: %v %v", st, err)
	}
	now := r.clk.now()
	root := r.ca.Root()
	if root.Subject.CommonName != "nself-ci root" || !root.IsCA || !root.BasicConstraintsValid || root.MaxPathLen != 1 || root.MaxPathLenZero {
		t.Fatalf("root: %+v", root.Subject)
	}
	if root.KeyUsage != x509.KeyUsageCertSign|x509.KeyUsageCRLSign || len(root.ExtKeyUsage) != 0 {
		t.Fatalf("root usage %v %v", root.KeyUsage, root.ExtKeyUsage)
	}
	if !root.NotBefore.Equal(now.Add(-120*time.Second)) || !root.NotAfter.Equal(now.Add(10*365*day)) {
		t.Fatalf("root validity %v .. %v", root.NotBefore, root.NotAfter)
	}
	if err = root.CheckSignatureFrom(root); err != nil {
		t.Fatalf("root is not self-signed: %v", err)
	}
	if pk, ok := root.PublicKey.(*ecdsa.PublicKey); !ok || pk.Curve != elliptic.P256() {
		t.Fatal("root key is not P-256")
	}
	if rows[2].NotAfter != root.NotAfter.UnixNano() {
		t.Fatalf("stored root not_after %d", rows[2].NotAfter)
	}
	wantEKU := map[Kind]x509.ExtKeyUsage{KindNode: x509.ExtKeyUsageClientAuth, KindUser: x509.ExtKeyUsageClientAuth, KindCoordinator: x509.ExtKeyUsageServerAuth}
	for k, eku := range wantEKU {
		in := r.ca.inter[k].cert
		if in.Subject.CommonName != KindCN(k) || KindCN(k) != "nself-ci "+string(k)+" CA" {
			t.Errorf("%s CN %q", k, in.Subject.CommonName)
		}
		if !in.IsCA || !in.BasicConstraintsValid || in.MaxPathLen != 0 || !in.MaxPathLenZero {
			t.Errorf("%s path length: %d zero=%v ca=%v", k, in.MaxPathLen, in.MaxPathLenZero, in.IsCA)
		}
		if len(in.ExtKeyUsage) != 1 || in.ExtKeyUsage[0] != eku {
			t.Errorf("%s EKU %v", k, in.ExtKeyUsage)
		}
		if in.KeyUsage != x509.KeyUsageCertSign|x509.KeyUsageCRLSign {
			t.Errorf("%s key usage %v", k, in.KeyUsage)
		}
		if !in.NotBefore.Equal(now.Add(-120*time.Second)) || !in.NotAfter.Equal(now.Add(5*365*day)) {
			t.Errorf("%s validity %v .. %v", k, in.NotBefore, in.NotAfter)
		}
		if err = in.CheckSignatureFrom(root); err != nil || in.Issuer.CommonName != "nself-ci root" {
			t.Errorf("%s not signed by the root: %v", k, err)
		}
		pk := r.ca.inter[k].key.PublicKey
		if pk.Curve != elliptic.P256() || !pk.Equal(in.PublicKey) {
			t.Errorf("%s key does not match its certificate", k)
		}
	}
	if !r.ca.root.key.PublicKey.Equal(root.PublicKey) {
		t.Fatal("root key does not match the certificate")
	}
	if r.ca.RootFingerprint() != CertFingerprint(r.ca.RootDER()) || !bytes.Equal(r.ca.RootDER(), root.Raw) {
		t.Fatal("root accessors disagree")
	}
	if !r.ca.Now().Equal(now) {
		t.Fatal("Now is not the injected clock")
	}
	// the sealed keys are not the plain keys
	kek, _ := LoadKEK(r.dir, false)
	for _, a := range rows {
		plain, err := Unseal(kek, a.KeySealed, "authority:"+a.Kind)
		if err != nil {
			t.Fatalf("%s: %v", a.Kind, err)
		}
		if _, err = x509.ParsePKCS8PrivateKey(plain); err != nil {
			t.Fatalf("%s plain key: %v", a.Kind, err)
		}
		if _, err = x509.ParsePKCS8PrivateKey(a.KeySealed); err == nil {
			t.Fatalf("%s stored key parses without the KEK", a.Kind)
		}
	}
}

func TestSerialsAre127BitRandom(t *testing.T) {
	limit := new(big.Int).Lsh(big.NewInt(1), 127)
	seen := map[string]bool{}
	big64 := false
	for i := 0; i < 64; i++ {
		s, err := newSerial()
		if err != nil || s.Sign() < 0 || s.Cmp(limit) >= 0 || seen[s.String()] {
			t.Fatalf("serial %v %v", s, err)
		}
		seen[s.String()] = true
		big64 = big64 || s.BitLen() > 64
	}
	if !big64 {
		t.Fatal("64 serials, none above 64 bits: not 127-bit random")
	}
}

func TestReloadGivesTheSameHierarchy(t *testing.T) {
	r := newRig(t)
	r.clk.advance(3 * day)
	ca2, err := r.reload()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ca2.RootDER(), r.ca.RootDER()) || ca2.RootFingerprint() != r.ca.RootFingerprint() {
		t.Fatal("reload changed the root")
	}
	for _, k := range []Kind{KindNode, KindUser, KindCoordinator} {
		if !bytes.Equal(ca2.inter[k].cert.Raw, r.ca.inter[k].cert.Raw) || !ca2.inter[k].key.Equal(r.ca.inter[k].key) {
			t.Fatalf("reload changed %s", k)
		}
	}
	if n := r.count(`SELECT count(*) FROM pki_authority`); n != 4 {
		t.Fatalf("reload wrote authorities: %d", n)
	}
	// a second bootstrap is a no-op that loses the race quietly
	won, err := ca2.bootstrap(context.Background())
	if err != nil || won {
		t.Fatalf("second bootstrap: %v %v", won, err)
	}
	if n := r.count(`SELECT count(*) FROM pki_authority`); n != 4 {
		t.Fatalf("second bootstrap wrote authorities: %d", n)
	}
	// without an injected clock the CA uses wall time
	ca3, err := New(context.Background(), Options{Store: r.st, DataDir: r.dir})
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(ca3.Now()); d < 0 || d > time.Minute {
		t.Fatalf("default clock off by %v", d)
	}
}

func TestLoadFailures(t *testing.T) {
	t.Run("missing KEK over an existing hierarchy", func(t *testing.T) {
		r := newRig(t)
		if err := os.Remove(KEKPath(r.dir)); err != nil {
			t.Fatal(err)
		}
		if _, err := r.reload(); !errors.Is(err, ErrNoKEK) {
			t.Fatalf("err = %v", err)
		}
		if _, err := os.Stat(KEKPath(r.dir)); !os.IsNotExist(err) {
			t.Fatal("a missing KEK was recreated over existing sealed keys")
		}
	})
	t.Run("wrong KEK", func(t *testing.T) {
		r := newRig(t)
		wrong := bytes.Repeat([]byte{9}, KEKSize)
		if err := os.WriteFile(KEKPath(r.dir), wrong, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := r.reload()
		if err == nil || !strings.Contains(err.Error(), "wrong or damaged KEK") || strings.Contains(err.Error(), string(wrong)) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("loose KEK mode", func(t *testing.T) {
		r := newRig(t)
		if err := os.Chmod(KEKPath(r.dir), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := r.reload(); err == nil || !strings.Contains(err.Error(), "accessible to others") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("sealed key moved to another authority", func(t *testing.T) {
		r := newRig(t)
		rows, _ := r.st.PKIAuthorities(context.Background())
		var node, user store.PKIAuthority
		for _, a := range rows {
			if a.Kind == "node" {
				node = a
			}
			if a.Kind == "user" {
				user = a
			}
		}
		r.exec(`UPDATE pki_authority SET key_sealed=? WHERE kind='node'`, user.KeySealed)
		r.exec(`UPDATE pki_authority SET key_sealed=? WHERE kind='user'`, node.KeySealed)
		if _, err := r.reload(); err == nil || !strings.Contains(err.Error(), "cannot unseal the node key") {
			t.Fatalf("err = %v", err)
		}
	})
	for _, kind := range []string{"root", "node", "user", "coordinator"} {
		t.Run("missing "+kind, func(t *testing.T) {
			r := newRig(t)
			r.exec(`DELETE FROM pki_authority WHERE kind=?`, kind)
			want := "pki: stored hierarchy has no " + kind + " intermediate"
			if kind == "root" {
				want = "pki: stored hierarchy has no root"
			}
			if _, err := r.reload(); err == nil || err.Error() != want {
				t.Fatalf("err = %v, want %q", err, want)
			}
		})
	}
	t.Run("garbage certificate", func(t *testing.T) {
		r := newRig(t)
		r.exec(`UPDATE pki_authority SET cert_der=x'0102' WHERE kind='root'`)
		if _, err := r.reload(); err == nil || !strings.Contains(err.Error(), "authority root:") {
			t.Fatalf("err = %v", err)
		}
	})
	damage := func(t *testing.T, plain []byte, want string) {
		r := newRig(t)
		kek, _ := LoadKEK(r.dir, false)
		sealed, err := Seal(kek, plain, "authority:root")
		if err != nil {
			t.Fatal(err)
		}
		r.exec(`UPDATE pki_authority SET key_sealed=? WHERE kind='root'`, sealed)
		if _, err = r.reload(); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, want %q", err, want)
		}
	}
	t.Run("key is not PKCS8", func(t *testing.T) { damage(t, []byte("not a key"), "authority root key:") })
	t.Run("key is not ECDSA", func(t *testing.T) {
		_, edk, _ := ed25519.GenerateKey(rand.Reader)
		der, _ := x509.MarshalPKCS8PrivateKey(edk)
		damage(t, der, "authority root key is not ECDSA")
	})
}

func TestKEKIsCreatedOnlyForAnEmptyStore(t *testing.T) {
	r := newRig(t)
	other := t.TempDir()
	// an existing hierarchy plus a data directory without a KEK is ErrNoKEK, never a new KEK
	if _, err := New(context.Background(), Options{Store: r.st, DataDir: other}); !errors.Is(err, ErrNoKEK) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(KEKPath(other)); !os.IsNotExist(err) {
		t.Fatal("KEK created next to an existing hierarchy")
	}
}
