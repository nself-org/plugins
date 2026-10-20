package update

// Purpose: the small pure parts: version order, the protocol ceiling, the atomic swap and the version line.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
)

func TestCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.4.0", "1.5.0", -1}, {"1.5.0", "1.5.0", 0}, {"2.0.0", "1.9.9", 1}, {"1.10.0", "1.9.0", 1},
		{"1.5.0-rc.1", "1.5.0", -1}, {"1.5.0", "1.5.0-rc.1", 1}, {"1.5.0-rc.2", "1.5.0-rc.10", -1},
		{"1.5.0-alpha", "1.5.0-1", 1}, {"1.5.0-rc", "1.5.0-rc.1", -1}, {"1.5.0-rc.1", "1.5.0-rc.1", 0},
	} {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%s,%s)=%d want %d", c.a, c.b, got, c.want)
		}
	}
	if DirectionOf("1.5.0", "1.4.0") != Downgrade || DirectionOf("1.4.0", "1.5.0") != Upgrade || DirectionOf("1.5.0", "1.5.0") != Same {
		t.Fatal("direction")
	}
}

func TestCeilingIsTheLowestCoordinatorMax(t *testing.T) {
	recs := []session.CoordinatorRecord{{ProtocolMax: 3}, {ProtocolMax: 2}, {ProtocolMax: 5}}
	if Ceiling(recs) != 2 || Ceiling(nil) != 0 {
		t.Fatalf("ceiling %d", Ceiling(recs))
	}
	if err := CheckCeiling(nil, manifest("9.0.0", 9, 9)); err != nil {
		t.Fatalf("no record means no ceiling: %v", err)
	}
}

func TestSwapKeepsPreviousAndRestores(t *testing.T) {
	dir := t.TempDir()
	bin, staged := filepath.Join(dir, "agent"), filepath.Join(dir, "staged")
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	read := func(p string) string { b, _ := os.ReadFile(p); return string(b) }
	write(staged, "new")
	if err := Swap(bin, staged); err != nil || read(bin) != "new" { // first install: nothing to keep
		t.Fatalf("first swap: %v %q", err, read(bin))
	}
	if _, err := os.Stat(bin + PrevSuffix); err == nil {
		t.Fatal("a first install invented a .prev")
	}
	write(bin+NewSuffix, "stale leftover")
	write(staged, "newer")
	if err := Swap(bin, staged); err != nil || read(bin) != "newer" || read(bin+PrevSuffix) != "new" {
		t.Fatalf("second swap: %v bin=%q prev=%q", err, read(bin), read(bin+PrevSuffix))
	}
	if _, err := os.Stat(bin + NewSuffix); err == nil {
		t.Fatal(".new left behind")
	}
	if err := Restore(bin); err != nil || read(bin) != "new" {
		t.Fatalf("restore: %v %q", err, read(bin))
	}
	if err := Restore(bin); err == nil {
		t.Fatal("a second restore with no .prev must fail")
	}
	if st, _ := os.Stat(bin); st.Mode().Perm()&0o100 == 0 {
		t.Fatalf("restored binary is not executable: %v", st.Mode())
	}
}

func TestSwapIsOneRenameSoAReaderNeverSeesAMix(t *testing.T) {
	dir := t.TempDir()
	bin, staged := filepath.Join(dir, "agent"), filepath.Join(dir, "staged")
	_ = os.WriteFile(bin, []byte("old-old-old"), 0o755)
	_ = os.WriteFile(staged, []byte("new-new-new"), 0o755)
	f, _ := os.Open(bin) // a running agent holds the old inode open
	defer func() { _ = f.Close() }()
	if err := Swap(bin, staged); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 11)
	if n, _ := f.Read(b); string(b[:n]) != "old-old-old" {
		t.Fatalf("the running image changed under the process: %q", b[:n])
	}
}

func TestParseVersionLine(t *testing.T) {
	for in, want := range map[string]string{"nself-ci-agent 1.5.0 linux/amd64\n": "1.5.0", "noise\nnself-ci-agent 1.5.0-rc.1 darwin/arm64\n": "1.5.0-rc.1"} {
		if got, ok := ParseVersionLine(in); !ok || got != want {
			t.Errorf("%q -> %q %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "nself-ci-agent\n", "other 1.5.0\n", "nself-ci-agent v1.5.0\n"} {
		if _, ok := ParseVersionLine(in); ok {
			t.Errorf("%q parsed", in)
		}
	}
}

func TestSwapKeepsTheModeOfTheFileItReplacesAndDropsAStaleMarker(t *testing.T) {
	dir := t.TempDir()
	bin, staged := filepath.Join(dir, "agent"), filepath.Join(dir, "staged")
	_ = os.WriteFile(bin, []byte("old"), 0o755)
	_ = os.WriteFile(staged, []byte("new"), 0o700)
	_ = os.WriteFile(bin+PrevVersionSuffix, []byte("0.9.0\n"), 0o600)
	if err := Swap(bin, staged); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(bin); st.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v: job users can no longer run the agent", st.Mode().Perm())
	}
	if _, err := os.Stat(bin + PrevVersionSuffix); err == nil {
		t.Fatal("a marker from an earlier swap survived")
	}
	fresh := filepath.Join(dir, "fresh")
	if err := Swap(fresh, staged); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(fresh); st.Mode().Perm() != 0o755 {
		t.Fatalf("first install mode %v", st.Mode().Perm())
	}
}
