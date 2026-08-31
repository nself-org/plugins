// Package snowflake implements the warehouse Driver for Snowflake.
// Uses COPY INTO via staged newline-delimited JSON — no row-by-row inserts.
package snowflake

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sf "github.com/snowflakedb/gosnowflake"

	// Blank import registers the "snowflake" sql driver via init().
	_ "github.com/snowflakedb/gosnowflake"
)

// Driver writes rows to Snowflake using PUT + COPY INTO staged JSON.
type Driver struct {
	db *sql.DB
}

// New opens a Snowflake connection using the provided DSN.
// DSN format: user:pass@account/database/schema?warehouse=WH
func New(ctx context.Context, dsn string) (*Driver, error) {
	if dsn == "" {
		return nil, fmt.Errorf("WAREHOUSE_SNOWFLAKE_DSN is required")
	}
	db, err := sql.Open("snowflake", dsn)
	if err != nil {
		return nil, fmt.Errorf("open snowflake: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping snowflake: %w", err)
	}
	return &Driver{db: db}, nil
}

// EnsureTable creates a Snowflake table if it does not already exist.
// Idempotent — CREATE TABLE IF NOT EXISTS.
func (d *Driver) EnsureTable(ctx context.Context, table string, columns map[string]string) error {
	cols := make([]string, 0, len(columns))
	for name, typ := range columns {
		cols = append(cols, fmt.Sprintf(`"%s" %s`, name, typ))
	}
	// #nosec G201 — table validated against np_* allowlist upstream
	ddl := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", table, strings.Join(cols, ", "))
	_, err := d.db.ExecContext(ctx, ddl)
	return err
}

// WriteBatch stages newline-delimited JSON in a transient internal stage and
// issues COPY INTO to load it. No row-by-row inserts.
func (d *Driver) WriteBatch(ctx context.Context, table string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}

	// Serialise rows as newline-delimited JSON.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, row := range rows {
		if err := enc.Encode(row); err != nil {
			return fmt.Errorf("encode row: %w", err)
		}
	}

	stageName := fmt.Sprintf("nself_warehouse_%s_%d", table, time.Now().UnixNano())
	fileName := stageName + ".ndjson"

	// Create a transient internal stage.
	if _, err := d.db.ExecContext(ctx,
		fmt.Sprintf("CREATE TEMPORARY STAGE %s FILE_FORMAT = (TYPE = JSON)", stageName),
	); err != nil {
		return fmt.Errorf("create stage: %w", err)
	}

	// PUT the JSON data into the stage.
	// sf.WithFileStream attaches the reader to the context (2-arg form).
	putCtx := sf.WithFileStream(ctx, strings.NewReader(buf.String()))
	putSQL := fmt.Sprintf("PUT file://%s @%s AUTO_COMPRESS=FALSE", fileName, stageName)
	if _, err := d.db.ExecContext(putCtx, putSQL); err != nil {
		return fmt.Errorf("put stage: %w", err)
	}

	// COPY INTO from stage.
	// #nosec G201
	copySQL := fmt.Sprintf(`
		COPY INTO %s
		FROM @%s
		FILE_FORMAT = (TYPE = JSON STRIP_OUTER_ARRAY = FALSE)
		MATCH_BY_COLUMN_NAME = CASE_INSENSITIVE
		PURGE = TRUE
	`, table, stageName)
	if _, err := d.db.ExecContext(ctx, copySQL); err != nil {
		return fmt.Errorf("copy into: %w", err)
	}
	return nil
}

// Close releases the Snowflake connection pool.
func (d *Driver) Close() error {
	return d.db.Close()
}
