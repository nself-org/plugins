//go:build unix

package entries

import (
	"syscall"
	"testing"
)

func TestStatfsMatchesTheKernel(t *testing.T) {
	dir := t.TempDir()
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		t.Fatal(err)
	}
	free, total, err := statfs(dir)
	if err != nil {
		t.Fatal(err)
	}
	wantTotal := int64(st.Blocks) * int64(st.Bsize)
	wantFree := int64(st.Bavail) * int64(st.Bsize)
	if total != wantTotal {
		t.Fatalf("total %d want %d", total, wantTotal)
	}
	if d := free - wantFree; d > 256<<20 || d < -256<<20 {
		t.Fatalf("free %d want about %d", free, wantFree)
	}
	if _, _, err = statfs(dir + "/missing"); err == nil {
		t.Fatal("a missing path must fail")
	}
}
