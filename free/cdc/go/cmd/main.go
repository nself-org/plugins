// nself-cdc: Postgres WAL → Kafka/Redpanda/RabbitMQ change-data-capture plugin (CS_5).
package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nself-org/nself-cdc/api"
	"github.com/nself-org/nself-cdc/broker"
	"github.com/nself-org/nself-cdc/replication"
	"github.com/nself-org/nself-cdc/router"
)

func main() {
	cfg := loadConfig()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if cfg.BrokerType == "" {
		log.Fatal("CDC_BROKER is required (kafka|redpanda|rabbitmq)")
	}
	if len(cfg.BrokerURLs) == 0 {
		log.Fatal("CDC_BROKER_URLS is required")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// Run DB migrations (create slot, publication, np_cdc_events).
	if err := replication.Migrate(ctx, cfg.DatabaseURL); err != nil {
		log.Fatalf("cdc: migration: %v", err)
	}

	// Build topic router.
	tr := router.New(cfg.TopicPrefix)

	// Build broker adapter.
	brk, err := broker.New(cfg.BrokerType, cfg.BrokerURLs)
	if err != nil {
		log.Fatalf("cdc: broker init: %v", err)
	}

	// Build replication engine (embedded WAL reader or Debezium Connect proxy).
	eng, err := replication.NewEngine(ctx, replication.EngineConfig{
		DatabaseURL:     cfg.DatabaseURL,
		SlotName:        cfg.SlotName,
		PublicationName: cfg.PublicationName,
		Tables:          cfg.Tables,
		BatchSize:       cfg.BatchSize,
		FlushMS:         cfg.FlushMS,
		Mode:            cfg.Mode,
		DebeziumURL:     cfg.DebeziumURL,
	}, tr, brk)
	if err != nil {
		log.Fatalf("cdc: engine init: %v", err)
	}

	go func() {
		if err := eng.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("cdc engine error", "err", err)
		}
	}()

	// HTTP control plane.
	mux := api.NewMux(eng, brk)
	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		slog.Info("cdc listening", "addr", addr, "broker", cfg.BrokerType, "mode", cfg.Mode)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("cdc: http: %v", err)
		}
	}()

	<-ctx.Done()
	slog.Info("cdc shutting down")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
	eng.Close()
	brk.Close()
}
