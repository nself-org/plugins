//go:build linux || darwin || freebsd

package records

import (
	"syscall"
	"testing"
)

func TestVolumeSpaceMatchesStatfs(t *testing.T) {
	dir := t.TempDir()
	free, total, err := volumeSpace(dir)
	if err != nil {
		t.Fatal(err)
	}
	var st syscall.Statfs_t
	if err = syscall.Statfs(dir, &st); err != nil {
		t.Fatal(err)
	}
	wantTotal := uint64(st.Blocks) * uint64(st.Bsize)
	wantFree := uint64(st.Bavail) * uint64(st.Bsize)
	if total != wantTotal {
		t.Fatalf("total %d, want %d", total, wantTotal)
	}
	diff := free - wantFree
	if wantFree > free {
		diff = wantFree - free
	}
	if free > total || diff > 1<<30 {
		t.Fatalf("free %d vs %d (total %d)", free, wantFree, total)
	}
	if _, _, err = volumeSpace(dir + "/missing"); err == nil {
		t.Fatal("a missing path must error")
	}
}
