package hasuracheck_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Access classes of the access review of 2026-11-13.
const (
	staff    = "staff"    // no entry for an end-user role
	personal = "personal" // end users reach only their own rows
	shared   = "shared"   // every signed-in user of the account may read
)

// tablePolicy is the decision for one table.
type tablePolicy struct {
	class string
	owner string
	// ops are the operations an end user keeps on a personal table.
	ops []string
}

// policy holds the access review of 2026-11-13 for this plugin.
// np_progress_webhook_events carries no end-user entry today and the review
// gives it none; it is listed so that every shipped table has a decision.
var policy = map[string]tablePolicy{
	"np_progress_favorites":      {personal, "user_id", []string{"select", "insert", "update", "delete"}},
	"np_progress_history":        {personal, "user_id", []string{"select"}},
	"np_progress_positions":      {personal, "user_id", []string{"select", "insert", "update", "delete"}},
	"np_progress_watchlists":     {personal, "user_id", []string{"select", "insert", "update", "delete"}},
	"np_progress_webhook_events": {staff, "", nil},
}

var endUserRoles = map[string]bool{"user": true, "nself_user": true, "anonymous": true, "public": true}

// entry is one permission entry of a table.
type entry struct {
	op    string // select, insert, update, delete
	role  string
	lines []string
}

// table is one table block of a Hasura file.
type table struct {
	file    string
	name    string
	entries []entry
}

func indent(line string) int { return len(line) - len(strings.TrimLeft(line, " ")) }

// loadTables reads every Hasura file of the plugin and splits it by indentation.
func loadTables(t *testing.T) []table {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "hasura", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no Hasura files found: %v", err)
	}
	var tables []table
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var cur *table
		var curEntry *entry
		op := ""
		flush := func() {
			if cur != nil && curEntry != nil {
				cur.entries = append(cur.entries, *curEntry)
			}
			curEntry = nil
		}
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			switch ind := indent(line); {
			case ind == 0 && strings.HasPrefix(trimmed, "- table:"):
				flush()
				if cur != nil {
					tables = append(tables, *cur)
				}
				cur = &table{file: f}
				op = ""
			case cur == nil:
			case ind == 4 && cur.name == "" && strings.HasPrefix(trimmed, "name:"):
				cur.name = strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
			case ind == 2:
				flush()
				op = ""
				if strings.HasSuffix(trimmed, "_permissions:") {
					op = strings.TrimSuffix(trimmed, "_permissions:")
				}
			case ind == 4 && op != "" && strings.HasPrefix(trimmed, "- role:"):
				flush()
				role := strings.TrimSpace(strings.TrimPrefix(trimmed, "- role:"))
				curEntry = &entry{op: op, role: role}
			case curEntry != nil:
				curEntry.lines = append(curEntry.lines, line)
			}
		}
		flush()
		if cur != nil {
			tables = append(tables, *cur)
		}
	}
	return tables
}

// block returns the trimmed lines under key (filter, check, set, columns).
func (e entry) block(key string) []string {
	var out []string
	in, base := false, 0
	for _, line := range e.lines {
		trimmed := strings.TrimSpace(line)
		if in {
			if indent(line) <= base {
				break
			}
			out = append(out, trimmed)
			continue
		}
		if trimmed == key+":" || strings.HasPrefix(trimmed, key+": ") {
			in, base = true, indent(line)
			if rest := strings.TrimSpace(strings.TrimPrefix(trimmed, key+":")); rest != "" {
				out = append(out, rest)
			}
		}
	}
	return out
}

// hasEq reports whether cond holds "- col:" followed by "_eq: value".
func hasEq(cond []string, col, value string) bool {
	for i := 0; i+1 < len(cond); i++ {
		if cond[i] == "- "+col+":" && cond[i+1] == "_eq: "+value {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func checkPersonal(t *testing.T, tb table, p tablePolicy, e entry) {
	t.Helper()
	where := tb.name + " " + e.role + ":" + e.op
	if !contains(p.ops, e.op) {
		t.Errorf("%s: personal table allows end users only %v", where, p.ops)
		return
	}
	cols := e.block("columns")
	for _, c := range cols {
		if c == `"*"` || c == "*" {
			t.Errorf("%s: columns must be an explicit list, not a star", where)
		}
		if (e.op == "insert" || e.op == "update") &&
			(c == "- "+p.owner || c == "- source_account_id") {
			t.Errorf("%s: columns must not list %s", where, strings.TrimPrefix(c, "- "))
		}
	}
	key := "filter"
	if e.op == "insert" {
		key = "check"
	}
	cond := e.block(key)
	if len(cond) == 0 || cond[0] != "_and:" ||
		!hasEq(cond, "source_account_id", "X-Hasura-Source-Account-Id") ||
		!hasEq(cond, p.owner, "X-Hasura-User-Id") {
		t.Errorf("%s: %s must be an _and of the account condition and %s _eq X-Hasura-User-Id", where, key, p.owner)
	}
	if e.op == "insert" {
		set := e.block("set")
		if !contains(set, p.owner+": X-Hasura-User-Id") ||
			!contains(set, "source_account_id: X-Hasura-Source-Account-Id") {
			t.Errorf("%s: set must fill %s and source_account_id from the session", where, p.owner)
		}
	}
}

// TestEndUserAccessMatchesPolicy keeps the end-user entries in line with the review.
func TestEndUserAccessMatchesPolicy(t *testing.T) {
	for _, tb := range loadTables(t) {
		p, ok := policy[tb.name]
		if !ok {
			continue // reported by TestEveryShippedTableIsInPolicy
		}
		for _, e := range tb.entries {
			if !endUserRoles[e.role] {
				continue
			}
			switch p.class {
			case staff:
				t.Errorf("%s: staff-only table has end-user entry %s:%s", tb.name, e.role, e.op)
			case shared:
				if e.op != "select" {
					t.Errorf("%s: shared table has end-user entry %s:%s", tb.name, e.role, e.op)
				}
			case personal:
				checkPersonal(t, tb, p, e)
			default:
				t.Errorf("%s: unknown class %q", tb.name, p.class)
			}
		}
	}
}

// TestEveryShippedTableIsInPolicy makes a new table ship only with a decision.
func TestEveryShippedTableIsInPolicy(t *testing.T) {
	tables := loadTables(t)
	if len(tables) == 0 {
		t.Fatal("no tables found in the Hasura files")
	}
	for _, tb := range tables {
		if tb.name == "" {
			t.Errorf("%s: table block without a name", tb.file)
			continue
		}
		if _, ok := policy[tb.name]; !ok {
			t.Errorf("%s: table %s is not in the access policy", tb.file, tb.name)
		}
	}
}
