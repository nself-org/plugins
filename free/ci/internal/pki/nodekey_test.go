package pki

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

func edPub(t *testing.T) ed25519.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub
}

func TestNodeKeyID(t *testing.T) {
	zero := make(ed25519.PublicKey, 32)
	seq := make(ed25519.PublicKey, 32)
	for i := range seq {
		seq[i] = byte(i)
	}
	if got := NodeKeyID(zero); got != "66687aadf862bd77" {
		t.Fatalf("zero key id %s", got)
	}
	if got := NodeKeyID(seq); got != "630dcd2966c43366" {
		t.Fatalf("sequence key id %s", got)
	}
}

func TestPinNodeKey(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	for _, n := range []int{0, 31, 33, 64} {
		wantReason(t, r.ca.PinNodeKey(ctx, "n1", make([]byte, n)), ReasonBadCSR)
	}
	if _, _, ok, _ := r.st.PKINodeKeyGet(ctx, "n1"); ok {
		t.Fatal("a bad key was pinned")
	}
	pub := edPub(t)
	if err := r.ca.PinNodeKey(ctx, "n1", pub); err != nil {
		t.Fatal(err)
	}
	id, public, ok, err := r.st.PKINodeKeyGet(ctx, "n1")
	if err != nil || !ok || id != NodeKeyID(pub) || public != base64.StdEncoding.EncodeToString(pub) {
		t.Fatalf("pin: %q %q %v %v", id, public, ok, err)
	}
	// re-pin replaces
	pub2 := edPub(t)
	if err = r.ca.PinNodeKey(ctx, "n1", pub2); err != nil {
		t.Fatal(err)
	}
	if id, _, _, _ = r.st.PKINodeKeyGet(ctx, "n1"); id != NodeKeyID(pub2) {
		t.Fatal("re-pin did not replace")
	}
}

func TestCheckNodeKey(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	pub, other := edPub(t), edPub(t)

	t.Run("no pin: the first hello pins", func(t *testing.T) {
		if err := r.ca.CheckNodeKey(ctx, "ssh1", pub); err != nil {
			t.Fatal(err)
		}
		if id, p, ok, _ := r.st.PKINodeKeyGet(ctx, "ssh1"); !ok || id != NodeKeyID(pub) || p != base64.StdEncoding.EncodeToString(pub) {
			t.Fatalf("pin %q %q %v", id, p, ok)
		}
	})
	t.Run("pinned key matches", func(t *testing.T) {
		if err := r.ca.CheckNodeKey(ctx, "ssh1", pub); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a different key is node_key_changed and the pin stays", func(t *testing.T) {
		err := r.ca.CheckNodeKey(ctx, "ssh1", other)
		wantReason(t, err, ReasonNodeKeyChanged)
		if strings.Contains(err.Error(), base64.StdEncoding.EncodeToString(pub)) || strings.Contains(err.Error(), base64.StdEncoding.EncodeToString(other)) {
			t.Fatal("error carries key material")
		}
		if id, _, _, _ := r.st.PKINodeKeyGet(ctx, "ssh1"); id != NodeKeyID(pub) {
			t.Fatal("pin changed")
		}
		for _, bad := range []ed25519.PublicKey{nil, make(ed25519.PublicKey, 31), append(append(ed25519.PublicKey{}, pub...), 0)} {
			wantReason(t, r.ca.CheckNodeKey(ctx, "ssh1", bad), ReasonNodeKeyChanged)
		}
	})
	t.Run("no pin and a bad key", func(t *testing.T) {
		wantReason(t, r.ca.CheckNodeKey(ctx, "fresh", make(ed25519.PublicKey, 5)), ReasonBadCSR)
	})
	t.Run("pending pin after rotate", func(t *testing.T) {
		if err := r.ca.PinNodeKey(ctx, "n9", pub); err != nil {
			t.Fatal(err)
		}
		if err := r.ca.RotateNodeKey(ctx, "cli:admin", "n9", NodeKeyID(other)); err != nil {
			t.Fatal(err)
		}
		// the old key no longer works and a stranger does not either
		wantReason(t, r.ca.CheckNodeKey(ctx, "n9", pub), ReasonNodeKeyChanged)
		wantReason(t, r.ca.CheckNodeKey(ctx, "n9", edPub(t)), ReasonNodeKeyChanged)
		if id, p, _, _ := r.st.PKINodeKeyGet(ctx, "n9"); id != NodeKeyID(other) || p != "" {
			t.Fatalf("a refused hello changed the pending pin: %q %q", id, p)
		}
		// the named key completes the pin
		if err := r.ca.CheckNodeKey(ctx, "n9", other); err != nil {
			t.Fatal(err)
		}
		if id, p, _, _ := r.st.PKINodeKeyGet(ctx, "n9"); id != NodeKeyID(other) || p != base64.StdEncoding.EncodeToString(other) {
			t.Fatalf("pin not completed: %q %q", id, p)
		}
		wantReason(t, r.ca.CheckNodeKey(ctx, "n9", pub), ReasonNodeKeyChanged)
		if err := r.ca.CheckNodeKey(ctx, "n9", other); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("pending pin never matches a key of the wrong length", func(t *testing.T) {
		short := make(ed25519.PublicKey, 31)
		if err := r.ca.RotateNodeKey(ctx, "cli:admin", "n10", NodeKeyID(short)); err != nil {
			t.Fatal(err)
		}
		wantReason(t, r.ca.CheckNodeKey(ctx, "n10", short), ReasonNodeKeyChanged)
	})
	t.Run("pending pin on a node with no earlier pin", func(t *testing.T) {
		if err := r.ca.RotateNodeKey(ctx, "cli:admin", "n11", NodeKeyID(pub)); err != nil {
			t.Fatal(err)
		}
		if err := r.ca.CheckNodeKey(ctx, "n11", pub); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("store closed", func(t *testing.T) {
		r2 := newRig(t)
		_ = r2.st.Close()
		if err := r2.ca.CheckNodeKey(ctx, "x", pub); err == nil || ReasonOf(err) != "" {
			t.Fatalf("closed store: %v", err)
		}
	})
}

func TestRotateNodeKey(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	for _, id := range []string{"", "abc", strings.Repeat("a", 15), strings.Repeat("a", 17), strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64),
		"0123456789ABCDEF", "0123456789abcdeF", "gggggggggggggggg", "0123456789abcde ", "0123456789abcdef\n", "\n0123456789abcdef", "0123456789abcdé"} {
		wantReason(t, r.ca.RotateNodeKey(ctx, "cli:admin", "n1", id), ReasonNodeKeyChanged)
	}
	if len(r.audit("pki.node_key.rotate")) != 0 {
		t.Fatal("a refused rotate was audited")
	}
	id := NodeKeyID(edPub(t))
	if err := r.ca.RotateNodeKey(ctx, "cli:admin", "n1", id); err != nil {
		t.Fatal(err)
	}
	if got := r.audit("pki.node_key.rotate"); len(got) != 1 || got[0] != [3]string{"cli:admin", "n1", "new_key_id=" + id} {
		t.Fatalf("audit %v", got)
	}
	if kid, p, ok, _ := r.st.PKINodeKeyGet(ctx, "n1"); !ok || kid != id || p != "" {
		t.Fatalf("pin %q %q %v", kid, p, ok)
	}
}

func TestCheckPinnedNodeKeyIsStrict(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	pub, other := edPub(t), edPub(t)
	// no pin: refused, and the refusal does not pin
	err := r.ca.CheckPinnedNodeKey(ctx, "n1", pub)
	wantReason(t, err, ReasonNodeKeyChanged)
	if !strings.Contains(err.Error(), "no pinned node key") {
		t.Fatalf("text %q", err)
	}
	if _, _, ok, _ := r.st.PKINodeKeyGet(ctx, "n1"); ok {
		t.Fatal("CheckPinnedNodeKey pinned a key")
	}
	// full pin: the pinned key passes, anything else is refused, nothing is written
	if err = r.ca.PinNodeKey(ctx, "n1", pub); err != nil {
		t.Fatal(err)
	}
	if err = r.ca.CheckPinnedNodeKey(ctx, "n1", pub); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []ed25519.PublicKey{other, nil, pub[:31], append(append(ed25519.PublicKey{}, pub...), 0)} {
		wantReason(t, r.ca.CheckPinnedNodeKey(ctx, "n1", bad), ReasonNodeKeyChanged)
	}
	if id, p, _, _ := r.st.PKINodeKeyGet(ctx, "n1"); id != NodeKeyID(pub) || p != base64.StdEncoding.EncodeToString(pub) {
		t.Fatal("the pin changed")
	}
	// pending pin after rotate: the named key id matches, but the check is read-only and never completes the pin
	if err = r.ca.RotateNodeKey(ctx, "cli:admin", "n2", NodeKeyID(other)); err != nil {
		t.Fatal(err)
	}
	wantReason(t, r.ca.CheckPinnedNodeKey(ctx, "n2", pub), ReasonNodeKeyChanged)
	for i := 0; i < 2; i++ {
		if err = r.ca.CheckPinnedNodeKey(ctx, "n2", other); err != nil {
			t.Fatal(err)
		}
		if id, p, ok, _ := r.st.PKINodeKeyGet(ctx, "n2"); !ok || id != NodeKeyID(other) || p != "" {
			t.Fatalf("a read-only check completed the pending pin: %q %q", id, p)
		}
	}
	if len(r.audit("pki.node_key.rotate")) != 1 {
		t.Fatal("the check wrote audit rows")
	}
	// a pending pin never matches a key of the wrong length, even when the id is the hash of that key
	short := make(ed25519.PublicKey, 31)
	if err = r.ca.RotateNodeKey(ctx, "cli:admin", "n3", NodeKeyID(short)); err != nil {
		t.Fatal(err)
	}
	wantReason(t, r.ca.CheckPinnedNodeKey(ctx, "n3", short), ReasonNodeKeyChanged)
	// a store error is returned as is, not as node_key_changed
	_ = r.st.Close()
	if err = r.ca.CheckPinnedNodeKey(ctx, "n1", pub); err == nil || ReasonOf(err) != "" {
		t.Fatalf("closed store: %v", err)
	}
}

// A pending pin that names the full SHA-256 binds all 256 bits: a key whose 16-hex id collides is refused,
// and the matching key completes the pin and the re-pairing swap (D-1258).
func TestPendingPinFullFingerprint(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	pub, other := edPub(t), edPub(t)
	if err := r.ca.PinNodeKey(ctx, "nf", other); err != nil {
		t.Fatal(err)
	}
	if err := r.ca.RotateNodeKey(ctx, "cli:admin", "nf", NodeKeyFingerprint(pub)); err != nil {
		t.Fatal(err)
	}
	if got := NodeKeyFingerprint(pub); len(got) != 64 || got[:16] != NodeKeyID(pub) {
		t.Fatalf("fingerprint %q does not extend the key id %q", got, NodeKeyID(pub))
	}
	wantReason(t, r.ca.CheckPinnedNodeKey(ctx, "nf", other), ReasonNodeKeyChanged)
	// the same 16-hex id and a different digest tail does not match
	if keyIDMatches(NodeKeyID(pub)+strings.Repeat("0", 48), pub) {
		t.Fatal("a 64-hex id matched on its first 16 characters only")
	}
	if err := r.ca.CheckPinnedNodeKey(ctx, "nf", pub); err != nil {
		t.Fatal(err)
	}
	if err := r.ca.RepinNodeKey(ctx, "nf", pub); err != nil {
		t.Fatalf("re-pairing swap against a full-digest pending pin: %v", err)
	}
	if id, p, _, _ := r.st.PKINodeKeyGet(ctx, "nf"); id != NodeKeyID(pub) || p != base64.StdEncoding.EncodeToString(pub) {
		t.Fatalf("pin not completed: %q %q", id, p)
	}
	wantReason(t, r.ca.RepinNodeKey(ctx, "nf", other), ReasonNodeKeyChanged)
}

// A small-order key is never pinned and never passes a hello (D-1259).
func TestNodeKeyPinRefusesSmallOrderKey(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	identity := make(ed25519.PublicKey, ed25519.PublicKeySize)
	identity[0] = 1
	wantReason(t, r.ca.PinNodeKey(ctx, "so1", identity), ReasonBadCSR)
	wantReason(t, r.ca.CheckNodeKey(ctx, "so2", identity), ReasonBadCSR)
	wantReason(t, r.ca.RepinNodeKey(ctx, "so3", identity), ReasonBadCSR)
	if _, _, ok, _ := r.st.PKINodeKeyGet(ctx, "so2"); ok {
		t.Fatal("a small-order key was pinned")
	}
}
