// Package schema maps Postgres column types to warehouse target types.
package schema

import (
	"fmt"
	"strings"
)

// Column describes a single column in a Postgres table.
type Column struct {
	Name     string
	PGType   string // e.g. "int4", "text", "timestamptz", "uuid", "jsonb"
	Nullable bool
}

// TableSchema holds the column list for one np_* table.
type TableSchema struct {
	TableName string
	Columns   []Column
}

// ClickHouseType returns the ClickHouse column type for the given Postgres type.
func ClickHouseType(col Column) string {
	base := clickhouseBase(col.PGType)
	if col.Nullable {
		return fmt.Sprintf("Nullable(%s)", base)
	}
	return base
}

func clickhouseBase(pgType string) string {
	switch strings.ToLower(pgType) {
	case "bool", "boolean":
		return "Bool"
	case "int2", "smallint":
		return "Int16"
	case "int4", "integer", "int":
		return "Int32"
	case "int8", "bigint":
		return "Int64"
	case "float4", "real":
		return "Float32"
	case "float8", "double precision", "numeric", "decimal":
		return "Float64"
	case "text", "varchar", "char", "bpchar", "name", "citext":
		return "String"
	case "uuid":
		return "UUID"
	case "date":
		return "Date"
	case "timestamp", "timestamp without time zone":
		return "DateTime"
	case "timestamptz", "timestamp with time zone":
		return "DateTime64(3, 'UTC')"
	case "jsonb", "json":
		return "String" // Store as JSON string; parse in ClickHouse if needed
	case "bytea":
		return "String"
	default:
		return "String" // safe fallback
	}
}

// BigQueryType returns the BigQuery field type for the given Postgres type.
func BigQueryType(col Column) string {
	switch strings.ToLower(col.PGType) {
	case "bool", "boolean":
		return "BOOL"
	case "int2", "smallint", "int4", "integer", "int", "int8", "bigint":
		return "INT64"
	case "float4", "real", "float8", "double precision":
		return "FLOAT64"
	case "numeric", "decimal":
		return "NUMERIC"
	case "text", "varchar", "char", "bpchar", "name", "citext", "bytea":
		return "STRING"
	case "uuid":
		return "STRING"
	case "date":
		return "DATE"
	case "timestamp", "timestamp without time zone":
		return "DATETIME"
	case "timestamptz", "timestamp with time zone":
		return "TIMESTAMP"
	case "jsonb", "json":
		return "JSON"
	default:
		return "STRING"
	}
}

// SnowflakeType returns the Snowflake column type for the given Postgres type.
func SnowflakeType(col Column) string {
	switch strings.ToLower(col.PGType) {
	case "bool", "boolean":
		return "BOOLEAN"
	case "int2", "smallint":
		return "SMALLINT"
	case "int4", "integer", "int":
		return "INTEGER"
	case "int8", "bigint":
		return "BIGINT"
	case "float4", "real":
		return "FLOAT"
	case "float8", "double precision":
		return "DOUBLE"
	case "numeric", "decimal":
		return "NUMBER(38,9)"
	case "text", "varchar", "char", "bpchar", "name", "citext", "bytea":
		return "TEXT"
	case "uuid":
		return "VARCHAR(36)"
	case "date":
		return "DATE"
	case "timestamp", "timestamp without time zone":
		return "TIMESTAMP_NTZ"
	case "timestamptz", "timestamp with time zone":
		return "TIMESTAMP_TZ"
	case "jsonb", "json":
		return "VARIANT"
	default:
		return "TEXT"
	}
}
