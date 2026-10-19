package pki

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func claims(exp int64) TokenClaims {
	return TokenClaims{JTI: "j1", JobID: "job-1", AttemptID: "att-1", Revision: "rev-1", RunnerID: "n1", Epoch: 7, Exp: exp}
}

func newTokens(t *testing.T, r *rig) *Tokens {
	t.Helper()
	tk, err := r.ca.Tokens(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

func TestTokenMintVerifyRoundTrip(t *testing.T) {
	r := newRig(t)
	tk := newTokens(t, r)
	now := r.clk.now().Unix()
	tok, err := tk.Mint(claims(0), 30*time.Minute) // zero exp takes the cap
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 || strings.ContainsAny(tok, "=+/ ") {
		t.Fatalf("token form: %q", tok)
	}
	kid, _ := base64.RawURLEncoding.DecodeString(parts[0])
	if string(kid) != tk.active || len(kid) != 16 {
		t.Fatalf("kid %q", kid)
	}
	got, err := tk.Verify(tok)
	want := claims(now + int64((30*time.Minute + 5*time.Minute).Seconds()))
	if err != nil || got != want {
		t.Fatalf("verify: %+v %v\nwant %+v", got, err, want)
	}
	// the same token verifies through a freshly loaded Tokens (keys come from the store)
	got2, err := newTokens(t, r).Verify(tok)
	if err != nil || got2 != want {
		t.Fatalf("reloaded: %+v %v", got2, err)
	}
	if n := r.count(`SELECT count(*) FROM pki_token_key`); n != 1 {
		t.Fatalf("%d token keys", n)
	}
}

func TestTokenMintCapsExp(t *testing.T) {
	r := newRig(t)
	tk := newTokens(t, r)
	now := r.clk.now().Unix()
	limit := now + int64((10*time.Minute + TokenSlack).Seconds())
	if TokenSlack != 5*time.Minute {
		t.Fatal("slack changed")
	}
	tests := []struct {
		name string
		exp  int64
		ok   bool
		want int64
	}{
		{"zero takes the cap", 0, true, limit},
		{"exactly the cap", limit, true, limit},
		{"one second past the cap", limit + 1, false, 0},
		{"far past the cap", limit + 3600, false, 0},
		{"one second from now", now + 1, true, now + 1},
		{"now", now, false, 0},
		{"the past", now - 1, false, 0},
		{"negative", -5, false, 0},
	}
	for _, c := range tests {
		tok, err := tk.Mint(claims(c.exp), 10*time.Minute)
		if !c.ok {
			wantReason(t, err, ReasonTokenExpired)
			if tok != "" {
				t.Errorf("%s: token minted with an error", c.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		cl, err := tk.Verify(tok)
		if err != nil || cl.Exp != c.want {
			t.Errorf("%s: exp %d (%v), want %d", c.name, cl.Exp, err, c.want)
		}
	}
	// zero job timeout caps at the slack alone
	tok, err := tk.Mint(claims(0), 0)
	cl, _ := tk.Verify(tok)
	if err != nil || cl.Exp != now+300 {
		t.Fatalf("zero timeout: %d %v", cl.Exp, err)
	}
	// the cap moves with the clock
	r.clk.advance(time.Hour)
	tok, err = tk.Mint(claims(0), 10*time.Minute)
	cl, _ = tk.Verify(tok)
	if err != nil || cl.Exp != limit+3600 {
		t.Fatalf("after an hour: %d %v", cl.Exp, err)
	}
}

func TestTokenVerifyRejections(t *testing.T) {
	r := newRig(t)
	tk := newTokens(t, r)
	good, err := tk.Mint(claims(0), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(good, ".")
	enc := base64.RawURLEncoding.EncodeToString
	signed := func(kid, body string) string { // a token signed by the real key over arbitrary parts
		head := enc([]byte(kid)) + "." + enc([]byte(body))
		return head + "." + enc(ed25519.Sign(tk.priv, []byte(head)))
	}
	otherKey, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	_ = otherKey
	forged := func(kid string) string {
		head := enc([]byte(kid)) + "." + parts[1]
		return head + "." + enc(ed25519.Sign(otherPriv, []byte(head)))
	}
	flip := func(s string) string {
		b := []byte(s)
		if b[0] == 'A' {
			b[0] = 'B'
		} else {
			b[0] = 'A'
		}
		return string(b)
	}
	body2, _ := json.Marshal(claims(r.clk.now().Unix() + 100))
	tests := []struct {
		name   string
		token  string
		reason string
	}{
		{"empty", "", ReasonTokenMalformed},
		{"one part", "abc", ReasonTokenMalformed},
		{"two parts", parts[0] + "." + parts[1], ReasonTokenMalformed},
		{"four parts", good + ".x", ReasonTokenMalformed},
		{"kid not base64", "!!!." + parts[1] + "." + parts[2], ReasonTokenMalformed},
		{"body not base64", parts[0] + ".!!!." + parts[2], ReasonTokenMalformed},
		{"signature not base64", parts[0] + "." + parts[1] + ".!!!", ReasonTokenMalformed},
		{"padded base64", parts[0] + "=." + parts[1] + "." + parts[2], ReasonTokenMalformed},
		{"unknown key id", signed("0000000000000000", string(body2)), ReasonTokenUnknownKey},
		{"unknown key id, foreign signer", forged("ffffffffffffffff"), ReasonTokenUnknownKey},
		{"empty key id", signed("", string(body2)), ReasonTokenUnknownKey},
		{"known key id, signature by another key", forged(tk.active), ReasonTokenSignature},
		{"altered body", parts[0] + "." + enc(body2) + "." + parts[2], ReasonTokenSignature},
		{"altered signature", parts[0] + "." + parts[1] + "." + flip(parts[2]), ReasonTokenSignature},
		{"empty signature", parts[0] + "." + parts[1] + ".", ReasonTokenSignature},
		{"body that is not JSON", signed(tk.active, "not json"), ReasonTokenMalformed},
		{"body that is a JSON string", signed(tk.active, `"x"`), ReasonTokenMalformed},
	}
	for _, c := range tests {
		cl, err := tk.Verify(c.token)
		wantReason(t, err, c.reason)
		if cl != (TokenClaims{}) {
			t.Errorf("%s: claims returned with an error: %+v", c.name, cl)
		}
		if strings.Contains(err.Error(), c.token) && c.token != "" {
			t.Errorf("%s: error echoes the token", c.name)
		}
	}
	if _, err = tk.Verify(good); err != nil {
		t.Fatal(err)
	}
}

func TestTokenVerifyExpiry(t *testing.T) {
	r := newRig(t)
	tk := newTokens(t, r)
	tok, err := tk.Mint(claims(0), 0) // exp = now + 300
	if err != nil {
		t.Fatal(err)
	}
	exp := r.clk.now().Unix() + 300
	r.clk.set(time.Unix(exp-1, 0))
	if cl, err := tk.Verify(tok); err != nil || cl.Exp != exp {
		t.Fatalf("one second before: %+v %v", cl, err)
	}
	for _, at := range []int64{exp, exp + 1, exp + 3600} {
		r.clk.set(time.Unix(at, 0))
		cl, err := tk.Verify(tok)
		wantReason(t, err, ReasonTokenExpired)
		if cl != (TokenClaims{}) {
			t.Fatalf("claims of an expired token: %+v", cl)
		}
	}
}

func TestTokensKeyLoading(t *testing.T) {
	ctx := context.Background()
	kek := func(r *rig) []byte { k, _ := LoadKEK(r.dir, false); return k }
	putKey := func(r *rig, pub ed25519.PublicKey, sealed []byte) string {
		kid := NodeKeyID(pub)
		if err := r.st.PKITokenKeyPut(ctx, kid, base64.StdEncoding.EncodeToString(pub), sealed); err != nil {
			t.Fatal(err)
		}
		return kid
	}
	t.Run("rotation: newest active key signs, retired keys still verify", func(t *testing.T) {
		r := newRig(t)
		oldTok := newTokens(t, r)
		oldToken, err := oldTok.Mint(claims(0), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		oldKID := oldTok.active
		r.clk.advance(time.Second)
		pub, seed, _ := ed25519.GenerateKey(rand.Reader)
		sealed, _ := Seal(kek(r), seed.Seed(), "token-key")
		newKID := putKey(r, pub, sealed)
		r.exec(`UPDATE pki_token_key SET retired_at=? WHERE kid=?`, r.clk.now().UnixNano(), oldKID)
		tk := newTokens(t, r)
		if tk.active != newKID || len(tk.pubs) != 2 {
			t.Fatalf("active %s, %d keys", tk.active, len(tk.pubs))
		}
		if _, err = tk.Verify(oldToken); err != nil {
			t.Fatalf("token of a retired key: %v", err)
		}
		fresh, err := tk.Mint(claims(0), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if k, _ := base64.RawURLEncoding.DecodeString(strings.Split(fresh, ".")[0]); string(k) != newKID {
			t.Fatalf("signed with %s", k)
		}
	})
	t.Run("two active keys: the newest signs", func(t *testing.T) {
		r := newRig(t)
		first := newTokens(t, r).active
		r.clk.advance(time.Second)
		pub, seed, _ := ed25519.GenerateKey(rand.Reader)
		sealed, _ := Seal(kek(r), seed.Seed(), "token-key")
		newest := putKey(r, pub, sealed)
		tk := newTokens(t, r)
		if tk.active != newest || newest == first {
			t.Fatalf("active %s", tk.active)
		}
	})
	t.Run("a retired key's blob is never unsealed", func(t *testing.T) {
		r := newRig(t)
		first := newTokens(t, r).active
		r.exec(`UPDATE pki_token_key SET key_sealed=x'00', retired_at=? WHERE kid=?`, r.clk.now().UnixNano(), first)
		r.clk.advance(time.Second)
		pub, seed, _ := ed25519.GenerateKey(rand.Reader)
		sealed, _ := Seal(kek(r), seed.Seed(), "token-key")
		putKey(r, pub, sealed)
		if _, err := r.ca.Tokens(ctx); err != nil {
			t.Fatal(err)
		}
	})
	damaged := []struct {
		name string
		do   func(r *rig)
	}{
		{"public key not base64", func(r *rig) { r.exec(`UPDATE pki_token_key SET public='***'`) }},
		{"public key wrong length", func(r *rig) {
			r.exec(`UPDATE pki_token_key SET public=?`, base64.StdEncoding.EncodeToString([]byte("short")))
		}},
		{"sealed seed damaged", func(r *rig) { r.exec(`UPDATE pki_token_key SET key_sealed=x'0102030405060708090a0b0c0d0e0f101112'`) }},
		{"sealed seed of the wrong length", func(r *rig) {
			s, _ := Seal(kek(r), []byte("only 31 bytes............!!!!!!"), "token-key")
			r.exec(`UPDATE pki_token_key SET key_sealed=?`, s)
		}},
		{"sealed for another purpose", func(r *rig) {
			s, _ := Seal(kek(r), make([]byte, ed25519.SeedSize), "authority:root")
			r.exec(`UPDATE pki_token_key SET key_sealed=?`, s)
		}},
	}
	for _, c := range damaged {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			newTokens(t, r)
			c.do(r)
			tk, err := r.ca.Tokens(ctx)
			wantReason(t, err, ReasonTokenMalformed)
			if tk != nil {
				t.Fatal("Tokens returned with an error")
			}
		})
	}
	t.Run("store closed", func(t *testing.T) {
		r := newRig(t)
		_ = r.st.Close()
		if _, err := r.ca.Tokens(ctx); err == nil {
			t.Fatal("Tokens on a closed store succeeded")
		}
	})
}

func TestMintWithoutAnActiveKey(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	first := newTokens(t, r)
	old, err := first.Mint(claims(0), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	r.exec(`UPDATE pki_token_key SET retired_at=?`, r.clk.now().UnixNano())
	tk, err := r.ca.Tokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tk.active != "" || tk.priv != nil {
		t.Fatal("a retired key became active")
	}
	tok, err := tk.Mint(claims(0), time.Hour)
	wantReason(t, err, ReasonTokenUnknownKey)
	if tok != "" {
		t.Fatal("a token was minted without an active key")
	}
	// tokens of the retired key still verify
	if _, err = tk.Verify(old); err != nil {
		t.Fatalf("token of a retired key: %v", err)
	}
	// the cap check is not reached: the key check comes first, even for a bad exp
	_, err = tk.Mint(claims(1), time.Hour)
	wantReason(t, err, ReasonTokenUnknownKey)
}

// TestTokenHasOneCanonicalString: the base64 decoder is strict, so a segment whose unused tail bits
// are altered (same decoded bytes) is refused and exactly one string verifies per mint.
func TestTokenHasOneCanonicalString(t *testing.T) {
	r := newRig(t)
	tk := newTokens(t, r)
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	// noncanonical changes the last character's unused low bits; "" when the segment has none
	noncanonical := func(seg string) string {
		spare := (6 * len(seg)) % 8
		if spare == 0 {
			return ""
		}
		last := strings.IndexByte(alphabet, seg[len(seg)-1])
		alt := last ^ 1 // flips the lowest bit, which is a spare bit
		if alt == last {
			t.Fatal("no change")
		}
		return seg[:len(seg)-1] + string(alphabet[alt])
	}
	relaxed := base64.RawURLEncoding
	segments := map[string]bool{}
	for i := 0; i < 8; i++ { // vary the claims so body lengths differ mod 3
		cl := claims(0)
		cl.JTI = strings.Repeat("x", i)
		tok, err := tk.Mint(cl, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(tok, ".")
		for idx, name := range []string{"kid", "body", "signature"} {
			alt := noncanonical(parts[idx])
			if alt == "" {
				continue
			}
			segments[name] = true
			a, _ := relaxed.DecodeString(parts[idx])
			b, err := relaxed.DecodeString(alt)
			if err != nil || string(a) != string(b) {
				t.Fatalf("%s variant is not the same bytes under a lax decoder", name)
			}
			bad := append([]string{}, parts...)
			bad[idx] = alt
			cl2, err := tk.Verify(strings.Join(bad, "."))
			wantReason(t, err, ReasonTokenMalformed)
			if cl2 != (TokenClaims{}) {
				t.Fatalf("%s: claims returned", name)
			}
		}
		if _, err = tk.Verify(tok); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"kid", "body", "signature"} {
		if !segments[name] {
			t.Fatalf("no %s segment with spare bits was exercised", name)
		}
	}
}

// TestRetiredKeyCapsExp (D-1259 item 1): a retired key verifies only tokens whose exp is at most
// retired_at + MaxRetiredTokenLife + TokenSlack, so retiring a key ends what it can vouch for.
func TestRetiredKeyCapsExp(t *testing.T) {
	r := newRig(t)
	first := newTokens(t, r)
	short, err := first.Mint(claims(0), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	long, err := first.Mint(claims(0), 100*24*time.Hour) // exp far beyond any job
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.Verify(long); err != nil {
		t.Fatalf("an active key must verify its token: %v", err)
	}
	r.exec(`UPDATE pki_token_key SET retired_at=?`, r.clk.now().UnixNano())
	retired := newTokens(t, r)
	if _, err = retired.Verify(short); err != nil {
		t.Fatalf("a short token of a retired key: %v", err)
	}
	_, err = retired.Verify(long)
	wantReason(t, err, ReasonTokenExpired)
	// the cap is exact: retired_at + life + slack passes, one second more does not
	edge := first.ca.now().Add(MaxRetiredTokenLife + TokenSlack).Unix()
	r.exec(`UPDATE pki_token_key SET retired_at=?`, r.clk.now().UnixNano())
	forge := func(exp int64) string {
		head := b64.EncodeToString([]byte(first.active)) + "." + func() string {
			body, _ := json.Marshal(claims(exp))
			return b64.EncodeToString(body)
		}()
		return head + "." + b64.EncodeToString(ed25519.Sign(first.priv, []byte(head)))
	}
	if _, err = retired.Verify(forge(edge)); err != nil {
		t.Fatalf("exp at the cap: %v", err)
	}
	_, err = retired.Verify(forge(edge + 1))
	wantReason(t, err, ReasonTokenExpired)
}

// TestTokensBootstrapRace (D-1259 item 2): processes that start together on an empty store end with one
// token key, and every one of them signs with it.
func TestTokensBootstrapRace(t *testing.T) {
	r := newRig(t)
	const n = 16
	cas := make([]*CA, n)
	for i := range cas {
		ca, err := r.reload()
		if err != nil {
			t.Fatal(err)
		}
		cas[i] = ca
	}
	start := make(chan struct{})
	active := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range cas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tk, err := cas[i].Tokens(context.Background())
			if err == nil {
				active[i] = tk.active
			}
			errs[i] = err
		}()
	}
	close(start)
	wg.Wait()
	for i := range cas {
		if errs[i] != nil {
			t.Fatalf("Tokens %d: %v", i, errs[i])
		}
		if active[i] != active[0] || active[i] == "" {
			t.Fatalf("Tokens %d signs with %q, Tokens 0 with %q", i, active[i], active[0])
		}
	}
	if k := r.count(`SELECT count(*) FROM pki_token_key`); k != 1 {
		t.Fatalf("%d token keys after a concurrent bootstrap", k)
	}
}
