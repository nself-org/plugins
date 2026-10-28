package netscope

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStagePrivateProxyBinary(t *testing.T) {
	source := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(source, []byte("fixture executable"), 0700); err != nil {
		t.Fatal(err)
	}
	staged, remove, err := stageProxyBinary(source)
	if err != nil {
		t.Fatal(err)
	}
	defer remove()
	st, err := os.Stat(staged)
	if err != nil || st.Mode().Perm() != 0755 {
		t.Fatalf("stage: %v %v", st, err)
	}
	original, err := os.Stat(source)
	if err != nil || original.Mode().Perm() != 0700 {
		t.Fatalf("source: %v %v", original, err)
	}
	raw, err := os.ReadFile(staged)
	if err != nil || string(raw) != "fixture executable" {
		t.Fatalf("copy: %q %v", raw, err)
	}
	remove()
	remove()
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("stage survived cleanup: %v", err)
	}
}
