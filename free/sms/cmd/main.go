// Package main is the plugin-sms entry point.
//
// Purpose: Start the SMS HTTP service backed by Twilio.
// Port: 9009 (default, overridable via SMS_PLUGIN_PORT).
// License: Pro (requires_license=true).
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/plugin-sms/internal"
)

func main() {
	cfg, err := internal.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "plugin-sms: config error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DBUrl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "plugin-sms: db connect error: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	srv := internal.NewServer(pool, cfg)
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	srv.Routes(r)

	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: r,
	}

	go func() {
		fmt.Printf("plugin-sms listening on :%d\n", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "plugin-sms: server error: %v\n", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutCtx)
}
