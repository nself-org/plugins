package pki

// Purpose: job tokens (Epic D5): Ed25519 signed claims minted by the coordinator for one lease.
// Inputs:  claims {jti, job_id, attempt_id, revision, runner_id, epoch, exp}; the job timeout.
// Outputs: a three-part token kid.claims.signature (base64url, no padding); verified claims.
// Constraints: exp is capped at job timeout + 5 min and must be in the future; the signing seed is
//          sealed under the KEK and never stored raw; Verify refuses a malformed token, an unknown key
//          id, a bad signature and an expired exp (E660 reasons token_*; P7-NODE-08 maps them to E661
//          and owns replay binding by jti); a token never enters a job environment.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

// TokenSlack is added to the job timeout for the cap on exp.
const TokenSlack = 5 * time.Minute

// TokenClaims are the signed claims.
type TokenClaims struct {
	JTI       string `json:"jti"`
	JobID     string `json:"job_id"`
	AttemptID string `json:"attempt_id"`
	Revision  string `json:"revision"`
	RunnerID  string `json:"runner_id"`
	Epoch     int64  `json:"epoch"`
	Exp       int64  `json:"exp"`
}

// Tokens signs and verifies with the coordinator's token keys.
type Tokens struct {
	ca     *CA
	active string
	priv   ed25519.PrivateKey
	pubs   map[string]ed25519.PublicKey
}

var b64 = base64.RawURLEncoding

// Tokens loads the token keys, creating the first one when none exists.
func (c *CA) Tokens(ctx context.Context) (*Tokens, error) {
	keys, err := c.st.PKITokenKeys(ctx)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		pub, seed, gerr := ed25519.GenerateKey(rand.Reader)
		if gerr != nil {
			return nil, gerr
		}
		sealed, serr := Seal(c.kek, seed.Seed(), "token-key")
		if serr != nil {
			return nil, serr
		}
		kid := NodeKeyID(pub)
		if err = c.st.PKITokenKeyPut(ctx, kid, base64.StdEncoding.EncodeToString(pub), sealed); err != nil {
			return nil, err
		}
		if keys, err = c.st.PKITokenKeys(ctx); err != nil {
			return nil, err
		}
	}
	t := &Tokens{ca: c, pubs: map[string]ed25519.PublicKey{}}
	for _, k := range keys {
		raw, derr := base64.StdEncoding.DecodeString(k.Public)
		if derr != nil || len(raw) != ed25519.PublicKeySize {
			return nil, fail(ReasonTokenMalformed, "token key %s is damaged", k.KID)
		}
		t.pubs[k.KID] = raw
		if t.active == "" && !k.Retired {
			seed, uerr := Unseal(c.kek, k.Sealed, "token-key")
			if uerr != nil || len(seed) != ed25519.SeedSize {
				return nil, fail(ReasonTokenMalformed, "token key %s cannot be unsealed", k.KID)
			}
			t.active, t.priv = k.KID, ed25519.NewKeyFromSeed(seed)
		}
	}
	return t, nil
}

// Mint signs claims. Exp is set to the cap when zero and refused when beyond it or already past.
func (t *Tokens) Mint(cl TokenClaims, jobTimeout time.Duration) (string, error) {
	now := t.ca.now()
	limit := now.Add(jobTimeout + TokenSlack).Unix()
	if cl.Exp == 0 {
		cl.Exp = limit
	}
	if cl.Exp > limit || cl.Exp <= now.Unix() {
		return "", fail(ReasonTokenExpired, "token exp must be in the future and at most job timeout + 5 minutes")
	}
	body, err := json.Marshal(cl)
	if err != nil {
		return "", err
	}
	head := b64.EncodeToString([]byte(t.active)) + "." + b64.EncodeToString(body)
	return head + "." + b64.EncodeToString(ed25519.Sign(t.priv, []byte(head))), nil
}

// Verify checks a token and returns its claims.
func (t *Tokens) Verify(token string) (TokenClaims, error) {
	var cl TokenClaims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return cl, fail(ReasonTokenMalformed, "token is malformed")
	}
	kid, e1 := b64.DecodeString(parts[0])
	body, e2 := b64.DecodeString(parts[1])
	sig, e3 := b64.DecodeString(parts[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return cl, fail(ReasonTokenMalformed, "token is malformed")
	}
	pub, ok := t.pubs[string(kid)]
	if !ok {
		return cl, fail(ReasonTokenUnknownKey, "token key id is unknown")
	}
	if !ed25519.Verify(pub, []byte(parts[0]+"."+parts[1]), sig) {
		return cl, fail(ReasonTokenSignature, "token signature is invalid")
	}
	if err := json.Unmarshal(body, &cl); err != nil {
		return TokenClaims{}, fail(ReasonTokenMalformed, "token claims are malformed")
	}
	if cl.Exp <= t.ca.now().Unix() {
		return TokenClaims{}, fail(ReasonTokenExpired, "token expired")
	}
	return cl, nil
}
