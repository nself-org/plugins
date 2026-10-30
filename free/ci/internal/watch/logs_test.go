package watch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
)

func TestRotatingLog(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs", "watch")
	l := &RotatingLog{Dir: dir, MaxBytes: 200, Keep: 2}
	for i := 0; i < 20; i++ {
		if err := l.Write("pass", map[string]any{"n": i, "pad": strings.Repeat("x", 40)}); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"watch.log", "watch.log.1", "watch.log.2"} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil || fi.Size() > 200 || fi.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", name, fi, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "watch.log.3")); !os.IsNotExist(err) {
		t.Fatal("only Keep rotated files are kept")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "watch.log"))
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil || rec["event"] != "pass" || rec["ts"] == "" {
			t.Fatalf("line %q", line)
		}
	}
	if err := Heartbeat(dir)(t0); err != nil {
		t.Fatal(err)
	}
	if b, _ = os.ReadFile(filepath.Join(dir, "heartbeat")); strings.TrimSpace(string(b)) != "2026-10-28T12:00:00Z" {
		t.Fatalf("heartbeat %q", b)
	}
}

func TestKeysDecode(t *testing.T) {
	specs := map[string]policy.KeySpec{}
	for _, k := range Keys() {
		specs[k.Key] = k
	}
	for key, raw := range map[string]string{KeyBranches: `["release/*","dev"]`, KeyMaxBranches: `20`, KeyRetryGap: `"10m"`, KeyStatusContext: `"nself-ci/watch"`, KeyDrainSec: `60`} {
		if _, err := specs[key].Decode([]byte(raw)); err != nil {
			t.Errorf("%s %s: %v", key, raw, err)
		}
	}
	for key, raw := range map[string]string{KeyBranches: `"main"`, KeyMaxBranches: `-1`, KeyRetryGap: `"5s"`, KeyStatusContext: `"other"`, KeyRetryMax: `1.5`} {
		if _, err := specs[key].Decode([]byte(raw)); err == nil {
			t.Errorf("%s %s accepted", key, raw)
		}
	}
	if errs := RegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
}
