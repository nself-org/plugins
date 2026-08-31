// Package bigquery implements the warehouse Driver for Google BigQuery.
// Rows are batched and flushed via the streaming insertAll API.
package bigquery

import (
	"context"
	"fmt"

	bq "cloud.google.com/go/bigquery"
	"google.golang.org/api/option"
)

// Config holds BigQuery connection parameters.
type Config struct {
	Project     string
	Dataset     string
	Credentials string // raw JSON service-account credentials
}

// Driver writes rows to BigQuery using the storage write API (batch path).
type Driver struct {
	client  *bq.Client
	dataset *bq.Dataset
	cfg     Config
}

// New creates a BigQuery client.
// Credentials may be a raw JSON string (WAREHOUSE_BQ_CREDENTIALS_JSON).
func New(ctx context.Context, cfg Config) (*Driver, error) {
	if cfg.Project == "" {
		return nil, fmt.Errorf("WAREHOUSE_BQ_PROJECT is required")
	}
	if cfg.Dataset == "" {
		return nil, fmt.Errorf("WAREHOUSE_BQ_DATASET is required")
	}

	var opts []option.ClientOption
	if cfg.Credentials != "" {
		opts = append(opts, option.WithCredentialsJSON([]byte(cfg.Credentials)))
	}

	client, err := bq.NewClient(ctx, cfg.Project, opts...)
	if err != nil {
		return nil, fmt.Errorf("bigquery client: %w", err)
	}

	return &Driver{
		client:  client,
		dataset: client.Dataset(cfg.Dataset),
		cfg:     cfg,
	}, nil
}

// EnsureTable creates a BigQuery table if it does not already exist.
// Schema is derived from the supplied columns map (col→BQ type string).
// Re-running on an existing table is a no-op (idempotent).
func (d *Driver) EnsureTable(ctx context.Context, table string, columns map[string]string) error {
	schema := make(bq.Schema, 0, len(columns))
	for name, typ := range columns {
		ft := bq.StringFieldType
		switch typ {
		case "INT64":
			ft = bq.IntegerFieldType
		case "FLOAT64":
			ft = bq.FloatFieldType
		case "BOOL":
			ft = bq.BooleanFieldType
		case "TIMESTAMP":
			ft = bq.TimestampFieldType
		case "DATE":
			ft = bq.DateFieldType
		case "DATETIME":
			ft = bq.DateTimeFieldType
		case "NUMERIC":
			ft = bq.NumericFieldType
		case "JSON":
			ft = bq.JSONFieldType
		}
		schema = append(schema, &bq.FieldSchema{
			Name: name,
			Type: ft,
		})
	}

	tbl := d.dataset.Table(table)
	meta := &bq.TableMetadata{Schema: schema}
	if err := tbl.Create(ctx, meta); err != nil {
		// Already-exists is not an error — idempotent
		if isAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("create table %s: %w", table, err)
	}
	return nil
}

// WriteBatch flushes a batch of rows to a BigQuery table via the
// legacy streaming insertAll API (simple, reliable, <30s latency).
func (d *Driver) WriteBatch(ctx context.Context, table string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}

	tbl := d.dataset.Table(table)
	ins := tbl.Inserter()

	items := make([]*bq.ValuesSaver, 0, len(rows))
	for _, row := range rows {
		// Convert to BigQuery ValuesSaver
		schema := make(bq.Schema, 0, len(row))
		vals := make([]bq.Value, 0, len(row))
		for k, v := range row {
			schema = append(schema, &bq.FieldSchema{Name: k, Type: bq.StringFieldType})
			vals = append(vals, v)
		}
		items = append(items, &bq.ValuesSaver{Schema: schema, Row: vals})
	}

	if err := ins.Put(ctx, items); err != nil {
		return fmt.Errorf("bigquery insert: %w", err)
	}
	return nil
}

// Close releases BigQuery client resources.
func (d *Driver) Close() error {
	return d.client.Close()
}

func isAlreadyExists(err error) bool {
	return err != nil && containsStr(err.Error(), "Already Exists")
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
