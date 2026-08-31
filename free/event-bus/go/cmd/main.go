// nself-event-bus: NATS JetStream / Redpanda / Kafka event bus plugin (CS_9).
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nself-org/nself-event-bus/api"
	"github.com/nself-org/nself-event-bus/broker"
	"github.com/nself-org/nself-event-bus/embedded"
	"github.com/nself-org/nself-event-bus/stats"
)

func main() {
	cfg := loadConfig()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// Start broker.
	brk, embeddedSrv, err := buildBroker(cfg)
	if err != nil {
		log.Fatalf("event-bus: broker init: %v", err)
	}

	store := stats.New()
	mux := api.NewMux(brk, store)

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("event-bus starting",
		"broker", brk.Type(),
		"addr", cfg.ListenAddr,
		"retention_ms", cfg.RetentionMS,
	)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("event-bus: http server: %v", err)
		}
	}()

	<-ctx.Done()
	slog.Info("event-bus shutting down")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()

	_ = srv.Shutdown(shutCtx)
	_ = brk.Drain()

	if embeddedSrv != nil {
		embeddedSrv.Shutdown()
	}

	slog.Info("event-bus stopped")
}

// buildBroker constructs the correct Broker from config and optionally starts
// the embedded NATS server.
func buildBroker(cfg *config) (broker.Broker, *embedded.Server, error) {
	switch strings.ToLower(cfg.BrokerType) {
	case "nats", "":
		var natsURL string
		var embSrv *embedded.Server

		if cfg.NATSEmbedded {
			srv, err := embedded.Start(cfg.NATSClientPort, cfg.NATSStoreDir)
			if err != nil {
				return nil, nil, fmt.Errorf("embedded nats: %w", err)
			}
			natsURL = srv.ClientURL()
			embSrv = srv
		} else {
			if cfg.NATSURL == "" {
				return nil, nil, fmt.Errorf("NATS_URL must be set when NATS_EMBEDDED=false")
			}
			natsURL = cfg.NATSURL
		}

		brk, err := broker.NewNATSBroker(natsURL, cfg.RetentionMS, cfg.MaxBytes)
		if err != nil {
			if embSrv != nil {
				embSrv.Shutdown()
			}
			return nil, nil, fmt.Errorf("nats broker: %w", err)
		}
		return brk, embSrv, nil

	case "redpanda":
		brk, err := broker.NewRedpandaBroker(cfg.RedpandaBrokers)
		if err != nil {
			return nil, nil, fmt.Errorf("redpanda broker: %w", err)
		}
		return brk, nil, nil

	case "kafka":
		brk, err := broker.NewKafkaBroker(cfg.KafkaBrokers, cfg.KafkaSASLUser, cfg.KafkaSASLPass)
		if err != nil {
			return nil, nil, fmt.Errorf("kafka broker: %w", err)
		}
		return brk, nil, nil

	default:
		return nil, nil, fmt.Errorf("unknown EVENT_BUS value %q (must be nats|redpanda|kafka)", cfg.BrokerType)
	}
}
