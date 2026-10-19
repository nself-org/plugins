package cas

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func putAged(t *testing.T, s *Store, r Realm, data []byte, age time.Duration) (string, string) {
	t.Helper()
	d, _, err := s.Put(context.Background(), r, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.blobPath(r, d)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-age)
	if err = os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	return d, p
}

// TestRefreshRestartsABlobsAge: a blob older than the sweep grace is swept; after
// Refresh it is young again, so Sweep keeps it without a second upload (D-1011).
func TestRefreshRestartsABlobsAge(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("refresh")
	d, p := putAged(t, s, r, []byte("early blob"), 3*time.Hour)
	if err := s.Refresh(r, d); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if age := time.Since(info.ModTime()); age > time.Minute {
		t.Fatalf("blob still %v old after Refresh", age)
	}
	none := func(Realm, string) bool { return false }
	if err = s.Sweep(context.Background(), "cache", none, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(p); err != nil {
		t.Fatalf("a refreshed blob was swept: %v", err)
	}
}

func TestRefreshRejectsMissingInvalidAndCorruptBlobs(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("refresh-bad")
	if err := s.Refresh(r, digestOf([]byte("absent"))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing blob: %v", err)
	}
	if err := s.Refresh(r, "nope"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad digest: %v", err)
	}
	if err := s.Refresh(Realm("bad"), digestOf(nil)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad realm: %v", err)
	}
	d, p := putAged(t, s, r, []byte("will rot"), 3*time.Hour)
	if err := os.WriteFile(p, []byte("rotten!!"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if err := s.Refresh(r, d); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("corrupt blob: %v", err)
	}
	if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("corrupt blob must be quarantined, stat: %v", err)
	}
}
