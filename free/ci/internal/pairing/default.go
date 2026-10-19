package pairing

// Purpose: the package-level entry points other tickets call (P7-SCHED-10: Offer, ServeWhileOffers,
//          Withdraw) against the service the composition root registered with Use.
// Inputs:  Use(service) once at start-up (cmd/nodes_pair.go for the foreground flow, `ci serve` for burst).
// Outputs: the same results as the Service methods; ErrNoService before Use.
// Constraints: one default service per process; Use replaces it (tests do).

import (
	"context"
	"errors"
	"sync/atomic"
)

// ErrNoService means no service was registered with Use.
var ErrNoService = errors.New("pairing: no service registered (call pairing.Use)")

var def atomic.Pointer[Service]

// Use registers the default service.
func Use(s *Service) { def.Store(s) }

// Offer creates an offer on the default service.
func Offer(ctx context.Context, o OfferOptions) (OfferResult, error) {
	s := def.Load()
	if s == nil {
		return OfferResult{}, ErrNoService
	}
	return s.Offer(ctx, o)
}

// Withdraw ends an offer on the default service and revokes its certificate.
func Withdraw(ctx context.Context, code string) error {
	s := def.Load()
	if s == nil {
		return ErrNoService
	}
	return s.Withdraw(ctx, code)
}

// ServeWhileOffers runs the burst listener of the default service.
func ServeWhileOffers(ctx context.Context, addr, publicURL string) error {
	s := def.Load()
	if s == nil {
		return ErrNoService
	}
	return s.ServeWhileOffers(ctx, addr, publicURL)
}
