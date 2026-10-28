package publish

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadOperatorKeysFailsClosed(t *testing.T) {
	for _, kind := range []string{"non-regular", "oversized", "unreadable", "unstatable"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "keys")
			switch kind {
			case "non-regular":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				if err := os.WriteFile(path, make([]byte, maxOperatorKeys+1), 0600); err != nil {
					t.Fatal(err)
				}
			case "unreadable":
				if os.Geteuid() == 0 {
					t.Skip("root can read mode-000 regular files")
				}
				if err := os.WriteFile(path, []byte("trust line"), 0000); err != nil {
					t.Fatal(err)
				}
			case "unstatable":
				// ELOOP is deterministic even when tests run as root.
				if err := os.Symlink(path, path); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := readOperatorKeys(path); err == nil {
				t.Fatal("invalid trust file accepted")
			}
		})
	}
}
