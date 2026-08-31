package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nself-org/nself-web3/internal"
)

// newRouter builds the chi router with all middleware and routes wired up.
// Extracted from main() so it can be exercised via httptest without binding
// a real port or connecting to a database (h.pool may be a mock for
// route-shape tests; individual handler behavior is covered in the internal
// package). Mirrors the pattern established in paid/moderation.
func newRouter(h *internal.Handlers) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", h.Health)
	r.Get("/ready", h.Ready)

	r.Route("/api/v1/wallets", func(r chi.Router) {
		r.Get("/", h.ListWallets)
		r.Post("/", h.CreateWallet)
		r.Get("/{id}", h.GetWallet)
		r.Put("/{id}", h.UpdateWallet)
		r.Delete("/{id}", h.DeleteWallet)
	})

	r.Route("/api/v1/collections", func(r chi.Router) {
		r.Get("/", h.ListCollections)
		r.Post("/", h.CreateCollection)
		r.Get("/{id}", h.GetCollection)
		r.Put("/{id}", h.UpdateCollection)
		r.Delete("/{id}", h.DeleteCollection)
	})

	r.Route("/api/v1/nfts", func(r chi.Router) {
		r.Get("/", h.ListNFTs)
		r.Post("/", h.CreateNFT)
		r.Get("/{id}", h.GetNFT)
		r.Put("/{id}", h.UpdateNFT)
		r.Delete("/{id}", h.DeleteNFT)
	})

	r.Route("/api/v1/tokens", func(r chi.Router) {
		r.Get("/", h.ListTokens)
		r.Post("/", h.CreateToken)
		r.Get("/{id}", h.GetToken)
		r.Put("/{id}", h.UpdateToken)
		r.Delete("/{id}", h.DeleteToken)
	})

	r.Route("/api/v1/token-balances", func(r chi.Router) {
		r.Get("/", h.ListTokenBalances)
		r.Post("/", h.UpsertTokenBalance)
		r.Delete("/{id}", h.DeleteTokenBalance)
	})

	r.Route("/api/v1/token-gates", func(r chi.Router) {
		r.Get("/", h.ListTokenGates)
		r.Post("/", h.CreateTokenGate)
		r.Get("/{id}", h.GetTokenGate)
		r.Put("/{id}", h.UpdateTokenGate)
		r.Delete("/{id}", h.DeleteTokenGate)
	})

	r.Route("/api/v1/gate-checks", func(r chi.Router) {
		r.Get("/", h.ListGateChecks)
		r.Post("/", h.CreateGateCheck)
		r.Get("/{id}", h.GetGateCheck)
	})

	r.Route("/api/v1/daos", func(r chi.Router) {
		r.Get("/", h.ListDAOs)
		r.Post("/", h.CreateDAO)
		r.Get("/{id}", h.GetDAO)
		r.Put("/{id}", h.UpdateDAO)
		r.Delete("/{id}", h.DeleteDAO)
	})

	r.Route("/api/v1/proposals", func(r chi.Router) {
		r.Get("/", h.ListProposals)
		r.Post("/", h.CreateProposal)
		r.Get("/{id}", h.GetProposal)
		r.Put("/{id}", h.UpdateProposal)
		r.Delete("/{id}", h.DeleteProposal)
	})

	r.Route("/api/v1/votes", func(r chi.Router) {
		r.Get("/", h.ListVotes)
		r.Post("/", h.CreateVote)
		r.Get("/{id}", h.GetVote)
	})

	r.Route("/api/v1/transactions", func(r chi.Router) {
		r.Get("/", h.ListTransactions)
		r.Post("/", h.CreateTransaction)
		r.Get("/{id}", h.GetTransaction)
	})

	r.Route("/api/v1/events", func(r chi.Router) {
		r.Get("/", h.ListEvents)
		r.Post("/", h.CreateEvent)
		r.Get("/{id}", h.GetEvent)
	})

	r.Get("/api/v1/gate-stats", h.GetGateStats)
	r.Post("/api/v1/gate-invalidate", h.InvalidateGate)

	r.Post("/api/v1/rpc/{chainId}", h.RPCBridge)

	return r
}

func main() {
	cfg := internal.LoadConfig()

	ctx := context.Background()
	pool, err := internal.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := internal.InitSchema(ctx, pool); err != nil {
		log.Fatalf("schema init: %v", err)
	}

	h := internal.NewHandlers(pool)
	r := newRouter(h)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("web3 plugin listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
}
