package agent

// Purpose: the gate's cost saving (D-1376): a token's signature is verified once per process and a lease is read
//          on state-changing frames, not on every log line.
// Inputs:  the token text and the call; the Gate's clock and caches.
// Outputs: verified claims, and whether a bulk frame may trust the last lease read.
// Constraints: claims are keyed by a digest, never the token text, and trusted only until their Exp; a lease read
//          that found the attempt over, or a spent token, drops the trust so the next frame reads the lease again;
//          every cache is bounded by maxKnown.

import (
	"crypto/sha256"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

type leaseUse struct{ jti, lease string }

// rereads says whether a frame type always reads the lease: the state-changing ones do, the bulk ones trust the last read.
func rereads(typ string) bool { return typ == "heartbeat" || typ == "result" }

// verify is Tokens.Verify with a cache: a token is checked once, then trusted until its expiry.
func (g *Gate) verify(token string) (pki.TokenClaims, error) {
	key := sha256.Sum256([]byte(token))
	g.mu.Lock()
	cl, ok := g.claims[key]
	g.mu.Unlock()
	if ok && cl.Exp > g.now().Unix() {
		return cl, nil
	}
	cl, err := g.Tokens.Verify(token)
	g.mu.Lock()
	defer g.mu.Unlock()
	if err != nil {
		delete(g.claims, key)
		return cl, err
	}
	if g.claims == nil || len(g.claims) >= maxKnown {
		g.claims = map[[sha256.Size]byte]pki.TokenClaims{}
	}
	g.claims[key] = cl
	return cl, nil
}

func (g *Gate) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

func (g *Gate) isFresh(jti, lease string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.fresh[leaseUse{jti, lease}]
}

func (g *Gate) markFresh(jti, lease string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.spent[jti] { // spent while this call was checking: do not let log frames skip the lease for it
		return
	}
	if g.fresh == nil || len(g.fresh) >= maxKnown {
		g.fresh = map[leaseUse]bool{}
	}
	g.fresh[leaseUse{jti, lease}] = true
}

func (g *Gate) forgetLease(jti, lease string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.fresh, leaseUse{jti, lease})
}
