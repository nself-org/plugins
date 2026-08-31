package main

import (
	"os"
	"strconv"
	"strings"
)

type config struct {
	DatabaseURL     string
	BrokerType      string   // kafka | redpanda | rabbitmq
	BrokerURLs      []string // comma-separated CDC_BROKER_URLS
	TopicPrefix     string
	Tables          []string // empty = all np_* tables
	Mode            string   // embedded | debezium-connect
	DebeziumURL     string
	SlotName        string
	PublicationName string
	BatchSize       int
	FlushMS         int
	Port            string
}

func loadConfig() config {
	c := config{
		DatabaseURL:     env("DATABASE_URL", ""),
		BrokerType:      env("CDC_BROKER", ""),
		TopicPrefix:     env("CDC_TOPIC_PREFIX", "nself"),
		Mode:            env("CDC_MODE", "embedded"),
		DebeziumURL:     env("CDC_DEBEZIUM_URL", ""),
		SlotName:        env("CDC_SLOT_NAME", "nself_cdc"),
		PublicationName: env("CDC_PUBLICATION_NAME", "nself_pub"),
		Port:            env("CDC_PORT", "8209"),
		BatchSize:       envInt("CDC_BATCH_SIZE", 100),
		FlushMS:         envInt("CDC_FLUSH_MS", 50),
	}

	if raw := env("CDC_BROKER_URLS", ""); raw != "" {
		for _, u := range strings.Split(raw, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				c.BrokerURLs = append(c.BrokerURLs, u)
			}
		}
	}

	if raw := env("CDC_TABLES", ""); raw != "" {
		for _, t := range strings.Split(raw, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				c.Tables = append(c.Tables, t)
			}
		}
	}

	return c
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
