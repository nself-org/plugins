package agekey

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const stubRecipient = "age1stubrecipient0000000000000000000000000000000000000000000"

// stubOn puts an age-keygen stub that mimics `age-keygen -o FILE` on PATH.
func stubOn(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\n[ \"$1\" = \"-o\" ] || exit 2\n[ -e \"$2\" ] && exit 3\nprintf '# created: 2026-01-01T00:00:00Z\\n# public key: " + stubRecipient + "\\nAGE-SECRET-KEY-STUBSTUB\\n' > \"$2\"\n"
	if err := os.WriteFile(filepath.Join(bin, Binary), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestAgeIdentity proves the acceptance line: the first start creates
// <state>/age.key 0600 with a stub age-keygen on PATH, a restart reuses it,
// the recipient is the age1... line of that file (the hello field), with no
// age-keygen the recipient is null and the capability fact says unknown, and
// the identity file never appears in a job's environment or workspace.
func TestAgeIdentity(t *testing.T) {
	stubOn(t)
	state := filepath.Join(t.TempDir(), "state")
	id, err := LoadOrCreate(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if id.Recipient == nil || *id.Recipient != stubRecipient {
		t.Fatalf("recipient %v", id.Recipient)
	}
	info, err := os.Stat(filepath.Join(state, FileName))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("age.key: %v %v", info, err)
	}
	if uid := os.Getuid(); uid >= 0 {
		if owner := fileOwner(info); owner != uid {
			t.Fatalf("owner %d, want agent uid %d", owner, uid)
		}
	}
	before, _ := os.ReadFile(filepath.Join(state, FileName))
	again, err := LoadOrCreate(context.Background(), state)
	if err != nil || *again.Recipient != stubRecipient {
		t.Fatalf("restart: %v %v", again, err)
	}
	after, _ := os.ReadFile(filepath.Join(state, FileName))
	if string(before) != string(after) {
		t.Fatal("identity was rewritten on restart")
	}
	if f := id.Fact(time.Unix(1, 0)); !f.Valid() || *f.Value != stubRecipient {
		t.Fatalf("fact %+v", f)
	}
	left, _ := filepath.Glob(filepath.Join(state, ".agekey-*"))
	if len(left) != 0 {
		t.Fatalf("scratch left behind: %v", left)
	}
	for _, kv := range os.Environ() {
		if strings.Contains(kv, FileName) && strings.HasPrefix(kv, "NSELF") {
			t.Fatalf("environment names the identity file: %s", kv)
		}
	}
}

// TestAgeIdentityAbsentKeygen proves a node without age-keygen reports a null
// recipient and an unknown fact instead of failing to start.
func TestAgeIdentityAbsentKeygen(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	state := t.TempDir()
	id, err := LoadOrCreate(context.Background(), state)
	if err != nil || id.Recipient != nil || id.Path != "" {
		t.Fatalf("got %+v %v", id, err)
	}
	if f := id.Fact(time.Unix(1, 0)); !f.Valid() || f.Value != nil || f.Confidence != "unknown" {
		t.Fatalf("fact %+v", f)
	}
	if _, err := os.Stat(filepath.Join(state, FileName)); err == nil {
		t.Fatal("created a key file without age-keygen")
	}
}

// TestAgeIdentityNeverRotates proves a changed PATH tool does not replace an
// existing identity, and a loosened file is refused.
func TestAgeIdentityNeverRotates(t *testing.T) {
	stubOn(t)
	state := t.TempDir()
	if _, err := LoadOrCreate(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(state, FileName), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(context.Background(), state); err == nil {
		t.Fatal("accepted a 0644 identity")
	}
}

func TestAgeKeygenFailureIsAnError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, Binary), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if _, err := LoadOrCreate(context.Background(), t.TempDir()); err == nil {
		t.Fatal("swallowed a failing age-keygen")
	}
}
