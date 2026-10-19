package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"strings"
	"testing"
)

func TestFingerprintKnownVectors(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "SHA256:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU"},
		{"abc", "SHA256:ungWv48Bz+pBQUDeXa4iI7ADYaOWF3qctBD/YfIAFa0"},
	}
	for _, c := range tests {
		if got := CertFingerprint([]byte(c.in)); got != c.want {
			t.Errorf("CertFingerprint(%q) = %s", c.in, got)
		}
		if got := SPKIFingerprint([]byte(c.in)); got != c.want {
			t.Errorf("SPKIFingerprint(%q) = %s", c.in, got)
		}
		if len(c.want) != len("SHA256:")+43 || strings.Contains(c.want, "=") {
			t.Errorf("form of %s", c.want)
		}
	}
}

func TestKeyFingerprintIsOfTheSPKI(t *testing.T) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	spki, _ := x509.MarshalPKIXPublicKey(&k.PublicKey)
	got, err := KeyFingerprint(&k.PublicKey)
	if err != nil || got != SPKIFingerprint(spki) || got != CertFingerprint(spki) {
		t.Fatalf("%s %v", got, err)
	}
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if o, _ := KeyFingerprint(&other.PublicKey); o == got {
		t.Fatal("two keys, one fingerprint")
	}
	if fp, err := KeyFingerprint("not a key"); err == nil || fp != "" {
		t.Fatalf("unsupported key: %q %v", fp, err)
	}
}

func TestEqual(t *testing.T) {
	a := CertFingerprint([]byte("a"))
	b := CertFingerprint([]byte("b"))
	tests := []struct {
		x, y string
		want bool
	}{
		{a, a, true}, {a, b, false}, {a, a + "x", false}, {a[:len(a)-1], a, false}, {"", "", true}, {"", a, false},
		{a, a[:len(a)-1] + "!", false},
	}
	for _, c := range tests {
		if got := Equal(c.x, c.y); got != c.want {
			t.Errorf("Equal(%q,%q) = %v", c.x, c.y, got)
		}
	}
}
