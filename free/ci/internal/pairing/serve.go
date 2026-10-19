package pairing

// Purpose: the two ways the enrollment listener runs: the foreground pairing window of `nodes pair`
//          (RunWindow) and the burst listener inside `ci serve` (ServeWhileOffers).
// Inputs:  the listen address and the public URL (its host becomes the SANs of the server leaf).
// Outputs: RunWindow returns when its offer is redeemed, withdrawn, locked or expired; ServeWhileOffers
//          returns when ctx ends.
// Constraints: the listener binds only while an offer can still be redeemed (RunWindow: its own offer;
//          ServeWhileOffers: at least one live offer bound to an intent) and stops accepting within one
//          poll or wake (<= 250 ms; the contract is 1 s); a request already in flight finishes; the
//          server leaf is issued per bind and its key never touches disk.

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func (s *Service) listen(addr, publicURL string, h enroll.Redeemer) (*enroll.Server, error) {
	dns, ips, err := pki.SANsFromURL(publicURL)
	if err != nil {
		return nil, err
	}
	leaf, err := s.cfg.CA.IssueServer(s.cfg.CoordinatorID, dns, ips, 0)
	if err != nil {
		return nil, err
	}
	return enroll.Listen(addr, s.cfg.CA.EnrollTLSConfig(leaf), enroll.Handler(h, enroll.HandlerOptions{Now: s.cfg.CA.Now}))
}

// intentRedeemer adapts Service to the burst listener (intent-bound codes only).
type intentRedeemer struct{ *Service }

func (r intentRedeemer) Redeem(ctx context.Context, q enroll.Request, src string) (enroll.Response, error) {
	return r.RedeemIntent(ctx, q, src)
}

func (s *Service) liveIntents(ctx context.Context) int {
	offers, err := s.cfg.Store.PKIOffersLive(ctx, true)
	if err != nil {
		return 0
	}
	return len(offers)
}

// ServeWhileOffers runs inside `ci serve`: it binds addr only while at least one live offer with an
// Intent exists, and closes it after the last such offer is redeemed, expired or withdrawn.
func (s *Service) ServeWhileOffers(ctx context.Context, addr, publicURL string) error {
	var srv *enroll.Server
	defer func() {
		if srv != nil {
			srv.Close()
		}
	}()
	tick := time.NewTicker(s.cfg.Poll)
	defer tick.Stop()
	for {
		switch live := s.liveIntents(ctx); {
		case live > 0 && srv == nil:
			var err error
			if srv, err = s.listen(addr, publicURL, intentRedeemer{s}); err != nil {
				return err
			}
		case live == 0 && srv != nil:
			srv.Close()
			srv = nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-s.wake:
		case <-tick.C:
		}
	}
}

// Window is a bound foreground pairing window.
type Window struct {
	Offer OfferResult
	URL   string
	Addr  net.Addr
	srv   *enroll.Server
}

// OpenWindow makes an offer and binds the enrollment listener for it (`nodes pair`). The caller prints
// the URL, code and root fingerprint, then calls Wait.
func (s *Service) OpenWindow(ctx context.Context, addr, publicURL string, o OfferOptions) (*Window, error) {
	srv, err := s.listen(addr, publicURL, s)
	if err != nil {
		return nil, err
	}
	offer, err := s.Offer(ctx, o)
	if err != nil {
		srv.Close()
		return nil, err
	}
	return &Window{Offer: offer, URL: "https://" + net.JoinHostPort(hostOf(publicURL), portOf(srv.Addr())) + enroll.Route, Addr: srv.Addr(), srv: srv}, nil
}

// Wait blocks until the offer can no longer be redeemed or ctx ends, then closes the listener. It
// reports whether the code was redeemed.
func (w *Window) Wait(ctx context.Context, s *Service) (redeemed bool, err error) {
	defer w.srv.Close()
	tick := time.NewTicker(s.cfg.Poll)
	defer tick.Stop()
	for {
		live, lerr := s.cfg.Store.PKIOffersLive(ctx, false)
		if lerr != nil {
			return false, lerr
		}
		if !hasOffer(live, w.Offer.ID) {
			return s.redeemedOffer(ctx, w.Offer.ID), nil
		}
		select {
		case <-ctx.Done():
			return false, nil
		case <-s.wake:
		case <-tick.C:
		}
	}
}

func hostOf(publicURL string) string {
	u, err := url.Parse(publicURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func portOf(a net.Addr) string {
	if t, ok := a.(*net.TCPAddr); ok {
		return strconv.Itoa(t.Port)
	}
	return ""
}

func hasOffer(offers []store.PKIOffer, id string) bool {
	for _, o := range offers {
		if o.ID == id {
			return true
		}
	}
	return false
}

func (s *Service) redeemedOffer(_ context.Context, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.redeemed[id]
}
