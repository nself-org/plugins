package hasuracheck_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type tablePolicy struct {
	name, class, owner string
}

// The access review of 2026-11-13 is the complete end-user policy.
var policy = []tablePolicy{
	{"np_contentacquisition_acquisition_history", "staff", ""},
	{"np_contentacquisition_acquisition_queue", "staff", ""},
	{"np_contentacquisition_acquisition_rules", "staff", ""},
	{"np_contentacquisition_acquisition_subscriptions", "staff", ""},
	{"np_contentacquisition_download_queue", "staff", ""},
	{"np_contentacquisition_download_rules", "staff", ""},
	{"np_contentacquisition_download_state_history", "staff", ""},
	{"np_contentacquisition_downloads", "staff", ""},
	{"np_contentacquisition_movie_monitoring", "staff", ""},
	{"np_contentacquisition_pipeline_runs", "staff", ""},
	{"np_contentacquisition_quality_profiles", "staff", ""},
	{"np_contentacquisition_release_calendar", "shared", ""},
	{"np_contentacquisition_rss_feed_items", "staff", ""},
	{"np_contentacquisition_rss_feeds", "staff", ""},
}

type permissionEntry struct {
	role, operation, text string
}

type tableBlock struct {
	name    string
	entries []permissionEntry
}

func scalar(s string) string {
	return strings.Trim(strings.TrimSpace(strings.SplitN(s, " #", 2)[0]), "\"'")
}

// readTables splits the shipped YAML by its table, section and role indentation.
// It uses no YAML dependency and rejects empty or unnamed table input.
func readTables(t *testing.T) []tableBlock {
	t.Helper()
	files, err := os.ReadDir("../hasura")
	if err != nil {
		t.Fatal(err)
	}
	var tables []tableBlock
	for _, file := range files {
		if file.IsDir() || (filepath.Ext(file.Name()) != ".yaml" && filepath.Ext(file.Name()) != ".yml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join("../hasura", file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var table *tableBlock
		var entry *permissionEntry
		var operation string
		for _, line := range strings.Split(string(data), "\n") {
			if line == "- table:" {
				tables = append(tables, tableBlock{})
				table = &tables[len(tables)-1]
				entry, operation = nil, ""
				continue
			}
			if table == nil {
				continue
			}
			if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
				operation = strings.TrimSuffix(strings.TrimSpace(line), "_permissions:")
				if !strings.HasSuffix(line, "_permissions:") {
					operation = ""
				}
				entry = nil
			}
			if strings.HasPrefix(line, "    name:") && operation == "" {
				table.name = scalar(strings.TrimPrefix(line, "    name:"))
			}
			if operation != "" && strings.HasPrefix(line, "    - role:") {
				table.entries = append(table.entries, permissionEntry{
					role: scalar(strings.TrimPrefix(line, "    - role:")), operation: operation,
				})
				entry = &table.entries[len(table.entries)-1]
			}
			if entry != nil {
				entry.text += line + "\n"
			}
		}
	}
	if len(tables) == 0 {
		t.Fatal("no Hasura tables found")
	}
	for _, table := range tables {
		if table.name == "" {
			t.Fatal("unnamed Hasura table")
		}
	}
	return tables
}

func endUser(role string) bool {
	switch role {
	case "user", "nself_user", "anonymous", "public":
		return true
	}
	return false
}

func field(text, name string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "        "+name+":") {
			j := i + 1
			for j < len(lines) && (strings.TrimSpace(lines[j]) == "" || strings.HasPrefix(lines[j], "          ")) {
				j++
			}
			return strings.Join(lines[i:j], "\n")
		}
	}
	return ""
}

func compares(text, column, session string) bool {
	pattern := `(?m)^([ ]+)(?:- )?` + regexp.QuoteMeta(column) + `:\s*\n[ ]+_eq: ["']?` + regexp.QuoteMeta(session) + `["']?[ \t]*$`
	return regexp.MustCompile(pattern).MatchString(text)
}

func checkPersonal(t *testing.T, p tablePolicy, entry permissionEntry) {
	t.Helper()
	condition := field(entry.text, "filter")
	if entry.operation == "insert" {
		condition = field(entry.text, "check")
		set := field(entry.text, "set")
		for column, session := range map[string]string{p.owner: "X-Hasura-User-Id", "source_account_id": "X-Hasura-Source-Account-Id"} {
			if !strings.Contains(set, "          "+column+": "+session) {
				t.Errorf("%s: insert must preset %s", p.name, column)
			}
		}
	}
	if p.owner == "" || !strings.Contains(condition, "          _and:") ||
		!compares(condition, p.owner, "X-Hasura-User-Id") ||
		!compares(condition, "source_account_id", "X-Hasura-Source-Account-Id") {
		t.Errorf("%s: %s must require account AND owner", p.name, entry.operation)
	}
	if entry.operation == "insert" || entry.operation == "update" {
		columns := field(entry.text, "columns")
		if columns == "" || strings.Contains(columns, "*") || strings.Contains(columns, p.owner) || strings.Contains(columns, "source_account_id") {
			t.Errorf("%s: %s columns must exclude owner and account", p.name, entry.operation)
		}
	}
}

// TestEndUserAccessMatchesPolicy enforces the reviewed access for all four end-user roles.
func TestEndUserAccessMatchesPolicy(t *testing.T) {
	for _, table := range readTables(t) {
		for _, p := range policy {
			if table.name != p.name {
				continue
			}
			for _, entry := range table.entries {
				if !endUser(entry.role) {
					continue
				}
				switch p.class {
				case "staff":
					t.Errorf("%s: staff-only table grants %s %s", p.name, entry.role, entry.operation)
				case "shared":
					if entry.operation != "select" {
						t.Errorf("%s: shared table grants %s %s", p.name, entry.role, entry.operation)
					}
					if !compares(field(entry.text, "filter"), "source_account_id", "X-Hasura-Source-Account-Id") {
						t.Errorf("%s: shared read must require the session account", p.name)
					}
				case "personal":
					checkPersonal(t, p, entry)
				default:
					t.Errorf("%s: unknown policy class %q", p.name, p.class)
				}
			}
		}
	}
}

// TestEveryShippedTableIsInPolicy requires a review decision for every shipped table.
func TestEveryShippedTableIsInPolicy(t *testing.T) {
	known := make(map[string]bool)
	for _, p := range policy {
		if known[p.name] {
			t.Errorf("duplicate policy: %s", p.name)
		}
		known[p.name] = true
	}
	for _, table := range readTables(t) {
		if !known[table.name] {
			t.Errorf("%s: shipped table has no access review decision", table.name)
		}
	}
}
