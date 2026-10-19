package store

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

// Every table that references a pipeline, job, attempt or lease by a foreign
// key must be emptied by pruneSteps, or prune would roll back on the first
// pipeline that has such a row. A new ticket that adds such a table fails here
// and must add its delete step (before the table it references).
func TestPruneStepsCoverEveryTableThatReferencesAPipelineTree(t *testing.T) {
	s := testStore(t)
	rows, err := s.readers.QueryContext(context.Background(),
		`SELECT m.name, f."table" FROM sqlite_master m, pragma_foreign_key_list(m.name) f WHERE m.type='table'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	deleted := map[string]bool{}
	re := regexp.MustCompile(`^DELETE FROM (\w+)`)
	for _, q := range pruneSteps {
		deleted[re.FindStringSubmatch(q)[1]] = true
	}
	tree := map[string]bool{"pipeline": true, "job": true, "attempt": true, "lease": true}
	for rows.Next() {
		var table, ref string
		if err = rows.Scan(&table, &ref); err != nil {
			t.Fatal(err)
		}
		if tree[strings.ToLower(ref)] && !deleted[table] {
			t.Errorf("table %s references %s but no prune step deletes from it (internal/store/prune.go pruneSteps)", table, ref)
		}
	}
}
