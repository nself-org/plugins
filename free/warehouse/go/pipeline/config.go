package pipeline

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for the warehouse plugin.
type Config struct {
	DatabaseURL string
	Driver      string // clickhouse | bigquery | snowflake
	Port        string

	// ClickHouse
	ClickhouseDSN string

	// BigQuery
	BQProject     string
	BQDataset     string
	BQCredentials string // raw JSON from WAREHOUSE_BQ_CREDENTIALS_JSON

	// Snowflake
	SnowflakeDSN string

	// Streaming behaviour
	BatchSize     int
	FlushInterval time.Duration

	// Optional table filter — nil means ALL np_* tables
	Tables []string
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() *Config {
	batchSize := 1000
	if v := os.Getenv("WAREHOUSE_BATCH_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			batchSize = n
		}
	}

	flushInterval := 30 * time.Second
	if v := os.Getenv("WAREHOUSE_FLUSH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			flushInterval = d
		}
	}

	var tables []string
	if v := os.Getenv("WAREHOUSE_TABLES"); v != "" {
		for _, t := range strings.Split(v, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tables = append(tables, t)
			}
		}
	}

	port := os.Getenv("WAREHOUSE_PORT")
	if port == "" {
		port = "8210"
	}

	return &Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		Driver:        strings.ToLower(strings.TrimSpace(os.Getenv("WAREHOUSE_DRIVER"))),
		Port:          port,
		ClickhouseDSN: os.Getenv("WAREHOUSE_CLICKHOUSE_DSN"),
		BQProject:     os.Getenv("WAREHOUSE_BQ_PROJECT"),
		BQDataset:     os.Getenv("WAREHOUSE_BQ_DATASET"),
		BQCredentials: os.Getenv("WAREHOUSE_BQ_CREDENTIALS_JSON"),
		SnowflakeDSN:  os.Getenv("WAREHOUSE_SNOWFLAKE_DSN"),
		BatchSize:     batchSize,
		FlushInterval: flushInterval,
		Tables:        tables,
	}
}
