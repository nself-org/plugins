package hasuracheck_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type tablePolicy struct {
	name, class, owner string
}

// The access review of 2026-11-13 covers only the four end-user roles.
var accessPolicy = []tablePolicy{
	{"np_encrypted_values", "staff", ""},
	{"np_encryption_key_events", "staff", ""},
	{"np_kms_configs", "staff", ""},
}

type permission struct {
	operation, role string
	lines           []string
}

type table struct {
	name, file  string
	permissions []permission
}

// TestEndUserAccessMatchesPolicy enforces the reviewed end-user access classes.
func TestEndUserAccessMatchesPolicy(t *testing.T) {
	policies := policyByName()
	seen := make(map[string]bool)
	for _, table := range shippedTables(t) {
		policy, ok := policies[table.name]
		if !ok {
			t.Errorf("%s: table %s has no access policy", table.file, table.name)
			continue
		}
		seen[table.name] = true
		for _, entry := range table.permissions {
			if !isEndUser(entry.role) {
				continue
			}
			switch policy.class {
			case "staff":
				t.Errorf("%s: staff-only table %s permits %s %s", table.file, table.name, entry.role, entry.operation)
			case "personal":
				checkPersonal(t, table.name, policy.owner, entry)
			case "shared":
				if entry.operation != "select" {
					t.Errorf("%s: shared table permits %s %s", table.name, entry.role, entry.operation)
				}
			default:
				t.Errorf("%s: unknown policy class %q", table.name, policy.class)
			}
		}
	}
	for name := range policies {
		if !seen[name] {
			t.Errorf("policy table %s was not found in metadata", name)
		}
	}
}

// TestEveryShippedTableIsInPolicy requires an access decision for each table.
func TestEveryShippedTableIsInPolicy(t *testing.T) {
	policies := policyByName()
	for _, table := range shippedTables(t) {
		if _, ok := policies[table.name]; !ok {
			t.Errorf("%s: shipped table %s is missing from the access policy", table.file, table.name)
		}
	}
}

func policyByName() map[string]tablePolicy {
	policies := make(map[string]tablePolicy)
	for _, policy := range accessPolicy {
		policies[policy.name] = policy
	}
	return policies
}

func isEndUser(role string) bool {
	switch role {
	case "user", "nself_user", "anonymous", "public":
		return true
	}
	return false
}

func indent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

func scalar(value string) string {
	return strings.Trim(strings.TrimSpace(strings.SplitN(value, " #", 2)[0]), "\"'")
}

// field returns a scalar and its indented children, excluding siblings.
func field(lines []string, key string) []string {
	for i, line := range lines {
		text := strings.TrimSpace(line)
		text = strings.TrimPrefix(text, "- ")
		if !strings.HasPrefix(text, key+":") {
			continue
		}
		end := i + 1
		for end < len(lines) {
			next := strings.TrimSpace(lines[end])
			if next != "" && !strings.HasPrefix(next, "#") && indent(lines[end]) <= indent(line) {
				break
			}
			end++
		}
		return append([]string{strings.TrimSpace(strings.TrimPrefix(text, key+":"))}, lines[i+1:end]...)
	}
	return nil
}

func shippedTables(t *testing.T) []table {
	t.Helper()
	var tables []table
	root := filepath.Join("..", "hasura", "metadata", "databases")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || (filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if text := strings.TrimSpace(line); text != "table:" && text != "- table:" {
				continue
			}
			end := i + 1
			for end < len(lines) && strings.TrimSpace(lines[end]) != "- table:" && strings.TrimSpace(lines[end]) != "table:" {
				end++
			}
			block := lines[i:end]
			name := field(field(block, "table"), "name")
			if len(name) == 0 || scalar(name[0]) == "" {
				t.Fatalf("%s: table block has no name", path)
			}
			tables = append(tables, table{scalar(name[0]), path, permissions(block)})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) == 0 {
		t.Fatal("no Hasura tables found")
	}
	return tables
}

func permissions(lines []string) []permission {
	var entries []permission
	for _, operation := range []string{"select", "insert", "update", "delete"} {
		block := field(lines, operation+"_permissions")
		for i := 0; i < len(block); i++ {
			text := strings.TrimSpace(block[i])
			if !strings.HasPrefix(text, "- role:") {
				continue
			}
			end := i + 1
			for end < len(block) {
				next := strings.TrimSpace(block[end])
				if next != "" && !strings.HasPrefix(next, "#") && indent(block[end]) <= indent(block[i]) {
					break
				}
				end++
			}
			entries = append(entries, permission{operation, scalar(strings.TrimPrefix(text, "- role:")), block[i:end]})
			i = end - 1
		}
	}
	return entries
}

func checkPersonal(t *testing.T, tableName, owner string, entry permission) {
	t.Helper()
	condition := "filter"
	if entry.operation == "insert" {
		condition = "check"
	}
	block := field(entry.lines, condition)
	and := field(block, "_and")
	eq := field(field(and, owner), "_eq")
	if owner == "" || len(eq) == 0 || scalar(eq[0]) != "X-Hasura-User-Id" {
		t.Errorf("%s: %s %s must compare owner %s with X-Hasura-User-Id in %s _and", tableName, entry.role, entry.operation, owner, condition)
	}
	if entry.operation == "insert" {
		set := field(entry.lines, "set")
		for column, session := range map[string]string{owner: "X-Hasura-User-Id", "source_account_id": "X-Hasura-Source-Account-Id"} {
			value := field(set, column)
			if len(value) == 0 || scalar(value[0]) != session {
				t.Errorf("%s: insert must set %s to %s", tableName, column, session)
			}
		}
	}
	if entry.operation == "insert" || entry.operation == "update" {
		for _, line := range field(entry.lines, "columns") {
			column := scalar(strings.TrimPrefix(strings.TrimSpace(line), "- "))
			if column == "*" || column == owner || column == "source_account_id" {
				t.Errorf("%s: %s columns must exclude owner and source_account_id and use an explicit list", tableName, entry.operation)
			}
		}
	}
}
