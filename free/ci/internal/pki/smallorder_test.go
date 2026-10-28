package pki

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A small-order key verifies a crafted signature for any message; the shape check refuses it (D-1259).
func TestSmallOrderNodeKeysAreRefused(t *testing.T) {
	ff := func(lo, hi string) string { // 32 bytes: lo, 30 x ff, hi
		b := lo
		for range 30 {
			b += "ff"
		}
		return b + hi
	}
	zeros := func(first, last string) string {
		b := first
		for range 30 {
			b += "00"
		}
		return b + last
	}
	small := map[string]string{
		"identity":                  zeros("01", "00"),
		"identity, sign bit":        zeros("01", "80"),
		"order 4 (y=0)":             zeros("00", "00"),
		"order 4, sign bit":         zeros("00", "80"),
		"order 2 (y=-1)":            ff("ec", "7f"),
		"identity, non-canonical y": ff("ee", "7f"),
		"order 8":                   "26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05",
		"order 8, other sign":       "26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc85",
		"order 8, y2":               "c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a",
		"order 8, y2 other sign":    "c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac03fa",
	}
	for name, enc := range small {
		pub := mustHex(t, enc)
		if !SmallOrderKey(pub) || ValidNodeKey(pub) {
			t.Errorf("%s: %s not refused", name, enc)
		}
	}
	// the identity verifies a signature (R = identity, S = 0) over any message: the reason for the check
	sig := append(mustHex(t, zeros("01", "00")), make([]byte, 32)...)
	if !ed25519.Verify(mustHex(t, zeros("01", "00")), []byte("any message"), sig) {
		t.Fatal("the premise changed: the identity key no longer verifies a crafted signature")
	}
	base := mustHex(t, "5866666666666666666666666666666666666666666666666666666666666666") // the base point
	if SmallOrderKey(base) || !ValidNodeKey(base) {
		t.Error("the base point was refused")
	}
	for range 20 {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if !ValidNodeKey(pub) {
			t.Fatalf("a generated key was refused: %x", pub)
		}
	}
	if ValidNodeKey(make([]byte, 31)) || SmallOrderKey(nil) {
		t.Error("length")
	}
}
