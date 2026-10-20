package store_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/cache/remote"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestCacheLocalityMigrate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	now := time.Unix(1234, 5678)
	for reopen := 0; reopen < 2; reopen++ {
		s, err := store.Open(path, store.Options{Clock: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		if reopen == 0 {
			for _, realm := range []string{"realm-a", "realm-b"} {
				if err := s.CacheLocalityPut(ctx, store.CacheLocalityRow{Node: "node-a", Realm: realm, Key: "same-key", Bytes: 5}); err != nil {
					t.Fatal(err)
				}
			}
		}
		rows, err := s.CacheLocality(ctx, "", "", "")
		if err != nil || len(rows) != 2 {
			t.Fatalf("reopen %d: rows=%+v err=%v", reopen, rows, err)
		}
		for _, row := range rows {
			if !row.LastSeen.Equal(now) {
				t.Fatalf("last seen=%v want %v", row.LastSeen, now)
			}
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCacheLocalityMetadata(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1234, 5678)
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for _, row := range []store.CacheLocalityRow{
		{Node: "node-a", Realm: "realm-a", Key: "key", Bytes: 1},
		{Node: "node-a", Realm: "realm-b", Key: "key", Bytes: 2},
		{Node: "node-b", Realm: "realm-a", Key: "key", Bytes: 3},
	} {
		if err := s.CacheLocalityPut(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	seen := now.Add(time.Hour)
	if err := s.CacheLocalityPut(ctx, store.CacheLocalityRow{Node: "node-a", Realm: "realm-a", Key: "key", Bytes: 4, LastSeen: seen}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.CacheLocality(ctx, "node-a", "realm-a", "key")
	if err != nil || len(rows) != 1 || rows[0].Bytes != 4 || !rows[0].LastSeen.Equal(seen) {
		t.Fatalf("refresh must replace just the matching copy: %+v %v", rows, err)
	}
	for _, row := range []store.CacheLocalityRow{
		{Realm: "realm-a", Key: "key"},
		{Node: "node-a", Key: "key"},
		{Node: "node-a", Realm: "realm-a"},
		{Node: "node-a", Realm: "realm-a", Key: "key", Bytes: -1},
	} {
		if err := s.CacheLocalityPut(ctx, row); err == nil {
			t.Fatalf("invalid metadata accepted: %+v", row)
		}
	}
	if err := s.CacheLocalityDrop(ctx, "node-a", "realm-a", "key"); err != nil {
		t.Fatal(err)
	}
	rows, err = s.CacheLocality(ctx, "", "", "")
	if err != nil || len(rows) != 2 || rows[0].Realm != "realm-b" || rows[0].Node != "node-a" || rows[1].Node != "node-b" || rows[1].Bytes != 3 {
		t.Fatalf("delete crossed node or realm boundaries: %+v %v", rows, err)
	}
}

func TestCacheLocalityReadThrough(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	cache, err := remote.NewReadThrough(t.TempDir(), 12)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	fetches := map[string]int{}
	open := func(realm, body string) string {
		t.Helper()
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
		f, err := cache.Open(ctx, realm, digest, int64(len(body)), func(_ context.Context, w io.Writer) error {
			fetches[realm+body]++
			_, err := io.Copy(w, bytes.NewBufferString(body))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(f)
		path := f.Name()
		_ = f.Close()
		if err != nil || string(got) != body {
			t.Fatalf("read-through bytes=%q err=%v", got, err)
		}
		if err := s.CacheLocalityPut(ctx, store.CacheLocalityRow{Node: "node-b", Realm: realm, Key: digest, Bytes: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		paths[realm+body] = path
		return path
	}
	path := open("realm-a", "aaaa")
	open("realm-a", "aaaa")
	if fetches["realm-a"+"aaaa"] != 1 {
		t.Fatal("verified copy fetched again")
	}
	if err := os.WriteFile(path, []byte("evil"), 0600); err != nil {
		t.Fatal(err)
	}
	open("realm-a", "aaaa")
	if fetches["realm-a"+"aaaa"] != 2 {
		t.Fatal("corrupt copy was served without a re-hash and replacement")
	}
	if other := open("realm-b", "aaaa"); other == path || fetches["realm-b"+"aaaa"] != 1 {
		t.Fatal("realm copies shared a path or bypassed the realm fetch")
	}
	for _, realm := range []string{"realm-a", "realm-b"} {
		rows, err := s.CacheLocality(ctx, "node-b", realm, "")
		if err != nil || len(rows) != 1 || rows[0].Bytes != 4 {
			t.Fatalf("per-realm locality %+v err=%v", rows, err)
		}
	}
	// A three-entry cap retains the most recently read copies at the next lease start.
	open("realm-a", "bbbb")
	old := time.Unix(10, 0)
	if err := os.Chtimes(paths["realm-a"+"aaaa"], old, old); err != nil {
		t.Fatal(err)
	}
	open("realm-a", "cccc")
	if err := cache.Prune(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths["realm-a"+"aaaa"]); !os.IsNotExist(err) {
		t.Fatalf("least-recently-used copy not removed: %v", err)
	}
	var total int64
	for key, path := range paths {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
		if key != "realm-a"+"aaaa" && info.Size() != 4 {
			t.Fatalf("copy %q has size %d", key, info.Size())
		}
	}
	if total != 12 {
		t.Fatalf("three-entry cap holds %d bytes, want 12", total)
	}
	if err := s.CacheLocalityDrop(ctx, "node-b", "realm-a", fmt.Sprintf("%x", sha256.Sum256([]byte("aaaa")))); err != nil {
		t.Fatal(err)
	}
	rows, err := s.CacheLocality(ctx, "node-b", "realm-a", "")
	if err != nil || len(rows) != 2 {
		t.Fatalf("remaining locality %+v err=%v", rows, err)
	}
}
