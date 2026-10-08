package architecture

import "testing"

func TestPreexistingGlobalImportBoundaries(t *testing.T) {
	m := readLayers(t)
	for _, imp := range []string{"os/exec", "database/sql", "modernc.org/sqlite", module + "internal/legacy", module + "cmd"} {
		if err := checkImport(m, "internal/clui", "review_probe.go", imp); err == nil {
			t.Errorf("clui imports %s", imp)
		}
	}
	if err := checkImport(m, "internal/serve", "review_probe.go", "os/exec"); err == nil {
		t.Error("new serve import bypassed boundary")
	}
}
