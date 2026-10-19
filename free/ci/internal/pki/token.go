package pki

// Purpose: job tokens (Epic D5): Ed25519 signed claims minted by the coordinator for one lease.
// Inputs:  claims {jti, job_id, attempt_id, revision, runner_id, epoch, exp}; the job timeout.
// Outputs: a three-part token kid.claims.signature (base64url, no padding); verified claims.
// Constraints: exp is capped at job timeout + 5 min and must be in the future; the signing seed is
//          sealed under the KEK and never stored raw; Verify refuses a malformed token, an unknown key
//          id, a bad signature and an expired exp (E660 reasons token_*; P7-NODE-08 maps them to E661
//          and owns replay binding by jti); a retired key verifies exp up to retired_at + 24 h + 5 min only;
//          a token never enters a job environment.

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

// MaxRetiredTokenLife bounds how long after retirement a retired key still vouches for a token: a retired
// key verifies exp up to retired_at + MaxRetiredTokenLife + TokenSlack, so a token minted with a very long
// exp does not outlive the key's retirement. No job runs longer than this.
const MaxRetiredTokenLife = 24 * time.Hour

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
	// retiredAt is the retirement time in unix seconds of every retired key (absent for an active key).
	retiredAt map[string]int64
}

var b64 = base64.RawURLEncoding.Strict() // strict: one string per token, no non-canonical tail bits

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
		// first creation is atomic: of several processes starting on an empty store only one key lands
		if err = c.st.PKITokenKeyBootstrap(ctx, kid, base64.StdEncoding.EncodeToString(pub), sealed); err != nil {
			return nil, err
		}
		if keys, err = c.st.PKITokenKeys(ctx); err != nil {
			return nil, err
		}
	}
	t := &Tokens{ca: c, pubs: map[string]ed25519.PublicKey{}, retiredAt: map[string]int64{}}
	for _, k := range keys {
		raw, derr := base64.StdEncoding.DecodeString(k.Public)
		if derr != nil || len(raw) != ed25519.PublicKeySize {
			return nil, fail(ReasonTokenMalformed, "token key %s is damaged", k.KID)
		}
		t.pubs[k.KID] = raw
		if k.Retired {
			t.retiredAt[k.KID] = k.RetiredAt / int64(time.Second)
		}
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
	if t.priv == nil {
		return "", fail(ReasonTokenUnknownKey, "no active token key (every key is retired)")
	}
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
	if at, retired := t.retiredAt[string(kid)]; retired && cl.Exp > at+int64((MaxRetiredTokenLife+TokenSlack).Seconds()) {
		return TokenClaims{}, fail(ReasonTokenExpired, "token outlives its retired key")
	}
	return cl, nil
}
