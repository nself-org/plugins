package pki

// Purpose: the live revocation check (review F4): an in-memory view of the deny list that follows the
//          store within one refresh interval, per-request checks, and cancellation of the live
//          requests of a certificate the moment it is denied.
// Inputs:  the store; Run(ctx, every) drives refresh; Middleware wraps an http.Handler.
// Outputs: Check (direct store read, used on every handshake), Denied (memory, used per request),
//          Middleware (RequestGuard).
// Constraints: RequestGuard fails closed (503) when the deny list was never read or the last successful
//          refresh is older than StaleAfter; a handshake never trusts the cache (Check reads the store); a request does, so the
//          refresh interval bounds the lag (callers use <= 250 ms against the 1 s promise); a watcher
//          is registered BEFORE the deny check, so a revoke between check and registration still
//          cancels the request.

import (
	"context"
	"crypto/x509"
	"net/http"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Guard follows the deny list.
type Guard struct {
	st       *store.Store
	mu       sync.Mutex
	denied   map[string]bool
	after    int64
	okAt     time.Time
	now      func() time.Time // injected by tests; time.Now otherwise
	watchers map[string]map[int]context.CancelFunc
	next     int
}

// NewGuard returns a guard; call Refresh or Run before serving.
func NewGuard(st *store.Store) *Guard {
	return &Guard{st: st, now: time.Now, denied: map[string]bool{}, watchers: map[string]map[int]context.CancelFunc{}}
}

// Refresh reads deny-list rows added since the last call and cancels the live requests of those serials.
func (g *Guard) Refresh(ctx context.Context) error {
	g.mu.Lock()
	after := g.after
	g.mu.Unlock()
	rows, err := g.st.PKIDenied(ctx, after)
	if err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.okAt = g.now()
	for _, r := range rows {
		g.denied[r.Serial] = true
		g.after = max(g.after, r.RowID)
		for _, cancel := range g.watchers[r.Serial] {
			cancel()
		}
	}
	return nil
}

// StaleAfter is how long the in-memory deny list may go without a successful refresh before
// RequestGuard refuses requests (fail closed: a store that cannot be read must not look like "nothing denied").
const StaleAfter = 10 * time.Second

func (g *Guard) fresh() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return !g.okAt.IsZero() && g.now().Sub(g.okAt) <= StaleAfter
}

// Run refreshes every interval until ctx ends.
func (g *Guard) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		_ = g.Refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Denied reports the in-memory verdict for a serial.
func (g *Guard) Denied(serial string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.denied[serial]
}

// Check is the handshake test: the deny list straight from the store and, for a node, its state.
func (g *Guard) Check(ctx context.Context, leaf *x509.Certificate) error {
	id, kind := IdentityOf(leaf)
	if denied, err := g.st.PKIIsDenied(ctx, serialHex(leaf)); err != nil || denied {
		return fail(ReasonRevoked, "certificate of %s is revoked", id)
	}
	if kind != KindNode {
		return nil
	}
	rec, err := g.st.NodeGet(ctx, id)
	if err != nil {
		return fail(ReasonUnknownNode, "node %s is not registered", id)
	}
	if rec.State == "revoked" {
		return fail(ReasonRevoked, "node %s is revoked", id)
	}
	if rec.State == store.NodeStateNeedsReconfirm { // restored onto an empty data dir: refused until `nodes enable` (D21, F15)
		return fail(ReasonRevoked, "node %s needs reconfirmation (nodes enable)", id)
	}
	return nil
}

func (g *Guard) watch(serial string, cancel context.CancelFunc) (stop func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.watchers[serial] == nil {
		g.watchers[serial] = map[int]context.CancelFunc{}
	}
	g.next++
	n := g.next
	g.watchers[serial][n] = cancel
	return func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		delete(g.watchers[serial], n)
		if len(g.watchers[serial]) == 0 {
			delete(g.watchers, serial)
		}
	}
}

// Middleware is RequestGuard: every request on a connection is checked against the in-memory deny
// list and wanted kind; the request context is cancelled when its certificate is denied meanwhile.
func (g *Guard) Middleware(kind Kind) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !g.fresh() {
				http.Error(w, "E660: revocation list unavailable", http.StatusServiceUnavailable)
				return
			}
			if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
				http.Error(w, "E660: no client certificate", http.StatusUnauthorized)
				return
			}
			leaf := r.TLS.PeerCertificates[0]
			if _, k := IdentityOf(leaf); k != kind {
				http.Error(w, "E660: wrong certificate kind", http.StatusForbidden)
				return
			}
			if now := g.now(); now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
				// TLS 1.3 never re-handshakes: a connection opened before NotAfter must stop at it.
				http.Error(w, "E660: certificate outside its validity period", http.StatusForbidden)
				return
			}
			ctx, cancel := context.WithCancel(r.Context())
			defer cancel()
			expiry := time.AfterFunc(leaf.NotAfter.Sub(g.now()), cancel) // a request in flight ends with its certificate
			defer expiry.Stop()
			defer g.watch(serialHex(leaf), cancel)()
			if g.Denied(serialHex(leaf)) {
				http.Error(w, "E660: certificate revoked", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
