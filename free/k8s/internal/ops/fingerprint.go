// Purpose: table fingerprints: the SQL that reads them, their parsing and their
// comparison.
//
// Inputs: the lines psql prints for the fingerprint query (one JSON object per
// table), two fingerprint lists to compare.
//
// Outputs: []Fingerprint ordered by schema and table; []Diff from Compare.
//
// Constraints: EPIC D8. One line per ordinary table (relkind r, partitions
// included) of every schema except pg_catalog, information_schema, pg_toast* and
// pg_temp_*: count(*) and the order-independent sum of the first 64 bits of
// md5(row text). The statements are generated inside SQL from pg_catalog with
// format(%L, %I) (Constitution 7.3a): no identifier is ever built in Go, and each
// result is a JSON object so an odd table name cannot break the parsing. The
// session settings that change the text of a row are pinned (sessionPrelude) on
// the backup and the restore side alike.
package ops

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// sessionPrelude opens every psql session of this package. The first three lines lift the
// timeouts a database or role may set (pg_dump and pg_restore reset them too): the holding
// session idles while pg_dump runs, and a fingerprint query on a large table can be slow.
// The rest pins everything that changes how a row prints as text.
const sessionPrelude = "SET statement_timeout = 0;\nSET lock_timeout = 0;\nSET idle_in_transaction_session_timeout = 0;\n" +
	"SET TimeZone = 'UTC';\nSET DateStyle = 'ISO, MDY';\nSET IntervalStyle = 'postgres';\nSET extra_float_digits = 3;\nSET bytea_output = 'hex';\n"

// The tables the fingerprint covers: ordinary tables (partitions included) outside
// pg_catalog, information_schema, pg_toast* and pg_temp_*. The query is split so the
// lock and fingerprint statements share one definition.
const (
	coveredFrom  = "\nFROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace"
	coveredWhere = "\nWHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog', 'information_schema')" +
		"\n  AND n.nspname NOT LIKE 'pg\\_toast%' AND n.nspname NOT LIKE 'pg\\_temp\\_%'"
	coveredOrder = "\nORDER BY n.nspname, c.relname\n"
)

// lockSQL takes ACCESS SHARE on every covered table, as pg_dump does, right after the
// snapshot is exported and before any fingerprint is read: a later TRUNCATE, rewrite or
// DDL waits for the holding session. (A TRUNCATE that commits before its table is locked
// is the same window pg_dump --snapshot has.)
const lockSQL = "SELECT format('LOCK TABLE %I.%I IN ACCESS SHARE MODE', n.nspname, c.relname)" +
	coveredFrom + coveredWhere + coveredOrder + "\\gexec\n"

// fingerprintSQL generates one query per table and runs it with \gexec. The whole-row alias
// is 'nself_fp_' + the table oid + '_' + the first number from 0 that no column of the
// table is named after, so no column can shadow the whole-row reference (a column wins over
// a same-named alias). A table has at most 1600 columns, so 1601 candidates always leave
// one free, and the alias stays under 30 bytes: postgres truncates an identifier at 63
// bytes, and a truncated alias could equal a 63-byte column name.
const fingerprintSQL = "SELECT format('SELECT json_build_object(''schema'', %1$L::text, ''table'', %2$L::text, ''rows'', count(*), " +
	"''sum'', coalesce(sum((''x'' || substr(md5(%3$I::text), 1, 16))::bit(64)::bigint), 0)::text)::text FROM %1$I.%2$I AS %3$I', " +
	"n.nspname, c.relname, x.alias)" + coveredFrom +
	"\nCROSS JOIN LATERAL (SELECT 'nself_fp_' || c.oid::text || '_' || k.n::text AS alias FROM generate_series(0, 1600) AS k(n)" +
	"\n  WHERE NOT EXISTS (SELECT 1 FROM pg_catalog.pg_attribute a WHERE a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped" +
	"\n  AND a.attname = 'nself_fp_' || c.oid::text || '_' || k.n::text) ORDER BY k.n LIMIT 1) x" +
	coveredWhere + coveredOrder + "\\gexec\n"

// Fingerprint is the content fingerprint of one table.
type Fingerprint struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Rows   int64  `json:"rows"`
	Sum    string `json:"sum"`
}

// parseFingerprints reads the fingerprint lines psql printed.
func parseFingerprints(lines []string) ([]Fingerprint, error) {
	out := make([]Fingerprint, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		var f Fingerprint
		if err := json.Unmarshal([]byte(l), &f); err != nil || f.Table == "" || f.Sum == "" {
			return nil, errFingerprint("a fingerprint line is not a table fingerprint")
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out, nil
}

func less(a, b Fingerprint) bool {
	if a.Schema != b.Schema {
		return a.Schema < b.Schema
	}
	return a.Table < b.Table
}

// Diff kinds.
const (
	DiffMissing = "missing" // in the backup, not in the database
	DiffExtra   = "extra"   // in the database, not in the backup
	DiffRows    = "rows"
	DiffSum     = "sum"
)

// maxDiffs bounds how many differences an error lists.
const maxDiffs = 20

// Diff is one difference between two fingerprint lists.
type Diff struct {
	Schema, Table, Kind string
	Want, Got           string
}

func (d Diff) String() string {
	name := d.Schema + "." + d.Table
	switch d.Kind {
	case DiffRows:
		return fmt.Sprintf("%s rows %s != %s", name, d.Want, d.Got)
	case DiffSum:
		return name + " content differs"
	}
	return name + " " + d.Kind
}

// Compare lists how got differs from want: tables missing from got, tables only
// in got, and tables whose row count or content sum differs. The order is
// deterministic (schema, table, kind).
func Compare(want, got []Fingerprint) []Diff {
	key := func(f Fingerprint) string { return f.Schema + "\x00" + f.Table }
	g := make(map[string]Fingerprint, len(got))
	for _, f := range got {
		g[key(f)] = f
	}
	var diffs []Diff
	seen := make(map[string]bool, len(want))
	for _, w := range want {
		seen[key(w)] = true
		f, ok := g[key(w)]
		switch {
		case !ok:
			diffs = append(diffs, Diff{Schema: w.Schema, Table: w.Table, Kind: DiffMissing})
		case f.Rows != w.Rows:
			diffs = append(diffs, Diff{Schema: w.Schema, Table: w.Table, Kind: DiffRows, Want: fmt.Sprint(w.Rows), Got: fmt.Sprint(f.Rows)})
		case f.Sum != w.Sum:
			diffs = append(diffs, Diff{Schema: w.Schema, Table: w.Table, Kind: DiffSum})
		}
	}
	for _, f := range got {
		if !seen[key(f)] {
			diffs = append(diffs, Diff{Schema: f.Schema, Table: f.Table, Kind: DiffExtra})
		}
	}
	sort.SliceStable(diffs, func(i, j int) bool {
		a, b := diffs[i], diffs[j]
		if a.Schema != b.Schema {
			return a.Schema < b.Schema
		}
		if a.Table != b.Table {
			return a.Table < b.Table
		}
		return a.Kind < b.Kind
	})
	return diffs
}
