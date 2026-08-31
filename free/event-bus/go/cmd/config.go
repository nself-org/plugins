package main

import (
	"os"
	"strconv"
	"strings"
)

const (
	defaultRetentionMS = 86_400_000 // 24 h
	defaultMaxBytes    = 1 << 30    // 1 GB
	defaultNATSPort    = 4222
	defaultStoreDir    = "/var/lib/nself-event-bus/jetstream"
)

type config struct {
	// Broker selection
	BrokerType string // nats | redpanda | kafka

	// NATS settings
	NATSEmbedded    bool
	NATSClientPort  int    // port for the embedded server
	NATSStoreDir    string // JetStream persistence dir
	NATSURL         string // used when NATSEmbedded=false

	// Redpanda / Kafka settings
	RedpandaBrokers []string
	KafkaBrokers    []string
	KafkaSASLUser   string
	KafkaSASLPass   string

	// Stream retention
	RetentionMS int64
	MaxBytes    int64

	// Service
	ListenAddr  string
	DatabaseURL string
}

func loadConfig() *config {
	c := &config{
		BrokerType:   envOr("EVENT_BUS", "nats"),
		NATSURL:      envOr("NATS_URL", ""),
		NATSClientPort: envIntOr("NATS_PORT", defaultNATSPort),
		NATSStoreDir:  envOr("NATS_STORE_DIR", defaultStoreDir),
		RetentionMS:  envInt64Or("EVENT_BUS_RETENTION_MS", defaultRetentionMS),
		MaxBytes:     envInt64Or("EVENT_BUS_MAX_BYTES", defaultMaxBytes),
		ListenAddr:   envOr("LISTEN_ADDR", ":8212"),
		DatabaseURL:  os.Getenv("DATABASE_URL"),
	}

	embedded := envOr("NATS_EMBEDDED", "true")
	c.NATSEmbedded = strings.ToLower(embedded) != "false"

	if brokers := envOr("REDPANDA_BROKERS", ""); brokers != "" {
		c.RedpandaBrokers = splitCSV(brokers)
	}
	if brokers := envOr("KAFKA_BROKERS", ""); brokers != "" {
		c.KafkaBrokers = splitCSV(brokers)
	}
	c.KafkaSASLUser = os.Getenv("KAFKA_SASL_USERNAME")
	c.KafkaSASLPass = os.Getenv("KAFKA_SASL_PASSWORD")

	return c
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOr(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envInt64Or(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
