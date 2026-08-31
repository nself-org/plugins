package schema_test

import (
	"testing"

	"github.com/nself-org/nself-warehouse/schema"
)

func TestClickHouseType(t *testing.T) {
	cases := []struct {
		pgType   string
		nullable bool
		want     string
	}{
		{"int4", false, "Int32"},
		{"int8", false, "Int64"},
		{"text", false, "String"},
		{"uuid", false, "UUID"},
		{"timestamptz", false, "DateTime64(3, 'UTC')"},
		{"bool", false, "Bool"},
		{"jsonb", false, "String"},
		{"float8", false, "Float64"},
		{"int4", true, "Nullable(Int32)"},
		{"text", true, "Nullable(String)"},
		{"unknown_custom_type", false, "String"}, // safe fallback
	}

	for _, tc := range cases {
		col := schema.Column{Name: "col", PGType: tc.pgType, Nullable: tc.nullable}
		got := schema.ClickHouseType(col)
		if got != tc.want {
			t.Errorf("ClickHouseType(%q, nullable=%v) = %q; want %q",
				tc.pgType, tc.nullable, got, tc.want)
		}
	}
}

func TestBigQueryType(t *testing.T) {
	cases := []struct {
		pgType string
		want   string
	}{
		{"int4", "INT64"},
		{"bigint", "INT64"},
		{"text", "STRING"},
		{"uuid", "STRING"},
		{"timestamptz", "TIMESTAMP"},
		{"bool", "BOOL"},
		{"jsonb", "JSON"},
		{"numeric", "NUMERIC"},
	}

	for _, tc := range cases {
		col := schema.Column{Name: "col", PGType: tc.pgType}
		got := schema.BigQueryType(col)
		if got != tc.want {
			t.Errorf("BigQueryType(%q) = %q; want %q", tc.pgType, got, tc.want)
		}
	}
}

func TestSnowflakeType(t *testing.T) {
	cases := []struct {
		pgType string
		want   string
	}{
		{"int4", "INTEGER"},
		{"bigint", "BIGINT"},
		{"text", "TEXT"},
		{"uuid", "VARCHAR(36)"},
		{"timestamptz", "TIMESTAMP_TZ"},
		{"bool", "BOOLEAN"},
		{"jsonb", "VARIANT"},
		{"numeric", "NUMBER(38,9)"},
	}

	for _, tc := range cases {
		col := schema.Column{Name: "col", PGType: tc.pgType}
		got := schema.SnowflakeType(col)
		if got != tc.want {
			t.Errorf("SnowflakeType(%q) = %q; want %q", tc.pgType, got, tc.want)
		}
	}
}
