//go:build !windows

package hygiene

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnedFilesReportsAnIncompleteScan(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < maxScanned+10; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%05d", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	found, complete := ownedFiles([]string{dir}, os.Geteuid(), "")
	if complete {
		t.Fatal("a scan that hit its entry limit must say so")
	}
	if len(found) == 0 || len(found) > maxScanned {
		t.Fatalf("found %d", len(found))
	}
	small := t.TempDir()
	if err := os.WriteFile(filepath.Join(small, "mine"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if found, complete := ownedFiles([]string{small}, os.Geteuid(), ""); !complete || len(found) != 1 {
		t.Fatalf("%v %v", found, complete)
	}
}
