package boot

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestBootSeedCoversManifest prevents the upgrade pass from falling back to
// DEFAULT VALUES, which cannot satisfy Stripe's required IDs and billing fields.
func TestBootSeedCoversManifest(t *testing.T) {
	for _, fixture := range []string{"seed.sql", "legacy.sql"} {
		t.Run(fixture, func(t *testing.T) { checkBootSeed(t, fixture) })
	}
}

func checkBootSeed(t *testing.T, fixture string) {
	t.Helper()
	manifest, err := os.ReadFile("../../plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var plugin struct{ Tables []string }
	if err := json.Unmarshal(manifest, &plugin); err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	inserts := regexp.MustCompile(`(?i)INSERT\s+INTO\s+(\w+)\s*\([^;]+?\)\s*VALUES\s*\([^;]+?\)\s*;`).FindAllStringSubmatch(string(seed), -1)
	counts := make(map[string]int)
	for _, insert := range inserts {
		counts[insert[1]]++
		if !strings.Contains(insert[0], "source_account_id") {
			t.Errorf("%s must seed an explicit source account", insert[1])
		}
	}
	if len(plugin.Tables) == 0 {
		t.Fatal("manifest has no tables")
	}
	for _, table := range plugin.Tables {
		if fixture == "legacy.sql" {
			table = strings.TrimPrefix(table, "np_")
		}
		if counts[table] != 1 {
			t.Errorf("%s: want one seed row, got %d", table, counts[table])
		}
		delete(counts, table)
	}
	for table := range counts {
		t.Errorf("seed inserts into undeclared table %s", table)
	}
}
