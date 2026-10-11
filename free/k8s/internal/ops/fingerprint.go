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

// sessionPrelude pins everything that changes how a row prints as text.
const sessionPrelude = "SET TimeZone = 'UTC';\nSET DateStyle = 'ISO, MDY';\nSET IntervalStyle = 'postgres';\nSET extra_float_digits = 3;\nSET bytea_output = 'hex';\n"

// fingerprintSQL generates one query per table and runs it with \gexec. The
// whole-row alias has a name no column plausibly has (a column called like the
// alias would win over the whole-row reference).
const fingerprintSQL = `SELECT format('SELECT json_build_object(''schema'', %L::text, ''table'', %L::text, ''rows'', count(*), ''sum'', coalesce(sum((''x'' || substr(md5(nself_fp_row::text), 1, 16))::bit(64)::bigint), 0)::text)::text FROM %I.%I AS nself_fp_row', n.nspname, c.relname, n.nspname, c.relname)
FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND n.nspname NOT LIKE 'pg\_toast%' AND n.nspname NOT LIKE 'pg\_temp\_%'
ORDER BY n.nspname, c.relname
\gexec
`

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
