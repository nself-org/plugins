package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRegularRefusesHostileFiles(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.yaml")
	if err := os.WriteFile(big, make([]byte, MaxFileBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	ok := filepath.Join(dir, "ok.yaml")
	if err := os.WriteFile(ok, []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(ok, link); err != nil {
		t.Fatal(err)
	}
	zero := filepath.Join(dir, "zero.yaml")
	if err := os.Symlink("/dev/zero", zero); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{big: "larger than", zero: "not a regular file", filepath.Join(dir, "dir"): ""} {
		_, err := readRegular(name)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if b, err := readRegular(link); err != nil || string(b) != "version: 1\n" {
		t.Errorf("symlink to a regular file must read: %v", err)
	}
	if _, err := Load(zero); err == nil || err.(*Error).Code != CodeInvalid {
		t.Errorf("Load of /dev/zero: %v", err)
	}
}
