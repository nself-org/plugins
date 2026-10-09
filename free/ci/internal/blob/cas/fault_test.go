//go:build fault

package cas

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFaultBeforeRename(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("fault")
	s.beforeRename = func() error { return errors.New("injected crash") }
	if _, _, e := s.Put(context.Background(), r, strings.NewReader("new")); e == nil {
		t.Fatal("fault missed")
	}
	if _, e := s.Stat(r, digestOf([]byte("new"))); !errors.Is(e, os.ErrNotExist) {
		t.Fatal(e)
	}
	tmp := filepath.Join(s.blobs(), "tmp", "orphan")
	if e := os.WriteFile(tmp, []byte("orphan"), 0600); e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	os.Chtimes(tmp, now.Add(-25*time.Hour), now.Add(-25*time.Hour))
	s.beforeRename = nil
	if e := s.Sweep(context.Background(), "cache", func(Realm, string) bool { return false }, now); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(tmp); !errors.Is(e, os.ErrNotExist) {
		t.Fatal(e)
	}
}
