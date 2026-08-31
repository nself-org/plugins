// Package server provides shared HTTP server lifecycle helpers for nSelf paid
// plugins. It eliminates the repeated shutdown boilerplate that was previously
// duplicated across every plugin main.go.
//
// Purpose: Centralise graceful HTTP server shutdown so each plugin only calls
//
//	GracefulShutdown(srv, timeout) — no per-plugin signal wiring needed.
//
// Inputs:  *http.Server to shut down, timeout for the shutdown context.
// Outputs: none — logs errors via log/slog; os.Exit(1) on fatal startup error.
// Constraints: Listens for SIGINT/SIGTERM via signal.NotifyContext. Must be
//
//	called from the main goroutine after srv.ListenAndServe is started in
//	a background goroutine.
//
// SPORT: F08-SERVICE-INVENTORY — shared lifecycle helper, P2-E6-W3-S02-T03.
//
// MIT licensed. Free counterpart of plugins-pro/paid/shared/go/server.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

// GracefulShutdown blocks until SIGINT or SIGTERM is received, then initiates
// a graceful HTTP server shutdown with the given timeout. If the server fails
// to shut down within the timeout, the error is logged but execution continues
// to allow deferred cleanup to run.
//
// Typical usage in a plugin main():
//
//	srv := &http.Server{Addr: "127.0.0.1:" + port, Handler: r}
//	go func() {
//	    if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
//	        slog.Error("server error", "err", err)
//	        os.Exit(1)
//	    }
//	}()
//	server.GracefulShutdown(srv, 30*time.Second)
func GracefulShutdown(srv *http.Server, timeout time.Duration) {
	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-sigCtx.Done()
	slog.Info("shutting down server", "addr", srv.Addr)

	shutCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		slog.Error("shutdown error", "err", err)
	}
}
