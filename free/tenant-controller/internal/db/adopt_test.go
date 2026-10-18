package db

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/migrate"
	"github.com/nself-org/nself-tenant-controller/migrations"
)

// A pre-v2 install applied only 001, so adopting its legacy ledger must record 001 alone;
// recording 002/003 without running them would skip the metering tables.
func TestLegacyAdoptionFSHoldsOnly001(t *testing.T) {
	var names []string
	ents, err := fs.ReadDir(legacyAdoptionFS(), ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		names = append(names, e.Name())
	}
	if len(names) != 2 || names[0] != "001_controller_schema.down.sql" && names[0] != "001_controller_schema.sql" {
		t.Fatalf("want only the 001 files, got %v", names)
	}
	for _, n := range names {
		if !strings.HasPrefix(n, "001_") {
			t.Fatalf("unexpected %s", n)
		}
	}
	if _, err := fs.ReadFile(legacyAdoptionFS(), "001_controller_schema.sql"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFile(migrations.FS, "002_cloud_metering.sql"); err != nil {
		t.Fatal("002 must still exist in the real FS")
	}
}

func TestLegacyLedgerHintNamesTheFlag(t *testing.T) {
	err := legacyLedgerError(migrate.ErrLegacyLedger)
	if !errors.Is(err, migrate.ErrLegacyLedger) || !strings.Contains(err.Error(), "NSELF_CONTROLLER_ADOPT_LEGACY") {
		t.Fatalf("hint missing: %v", err)
	}
}
