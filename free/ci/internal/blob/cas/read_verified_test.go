package cas

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
)

func putBlob(t *testing.T, s *Store, r Realm, b []byte) string {
	t.Helper()
	d, _, err := s.Put(context.Background(), r, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestReadVerifiedServesTheBlobInOneRead(t *testing.T) {
	s, r := testStore(t), CacheRealm("ns")
	want := bytes.Repeat([]byte("0123456789"), 5000)
	d := putBlob(t, s, r, want)
	var got []byte
	err := s.ReadVerified(r, d, func(in io.Reader) error {
		var e error
		got, e = io.ReadAll(in)
		return e
	})
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("read: %v (%d bytes)", err, len(got))
	}
	// a consumer that stops early (a tar reader stops before the padding) still gets the whole blob judged
	if err := s.ReadVerified(r, d, func(in io.Reader) error { _, e := io.ReadFull(in, make([]byte, 10)); return e }); err != nil {
		t.Fatalf("early stop: %v", err)
	}
}

func TestReadVerifiedNamesACorruptBlobAndQuarantinesIt(t *testing.T) {
	s, r := testStore(t), CacheRealm("ns")
	d := putBlob(t, s, r, bytes.Repeat([]byte("a"), 4096))
	p, _ := s.blobPath(r, d)
	if err := os.WriteFile(p, bytes.Repeat([]byte("b"), 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	// the consumer fails on the garbage first: the blob is still named corrupt
	consumerErr := errors.New("unreadable archive")
	err := s.ReadVerified(r, d, func(in io.Reader) error { _, _ = io.ReadFull(in, make([]byte, 8)); return consumerErr })
	if !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("err %v, want a digest mismatch", err)
	}
	if _, err := s.Stat(r, d); err == nil {
		t.Fatal("the corrupt blob was left in place")
	}
}

func TestReadVerifiedPassesTheConsumersOwnError(t *testing.T) {
	s, r := testStore(t), CacheRealm("ns")
	d := putBlob(t, s, r, []byte("fine"))
	stop := errors.New("refused by the sanitiser")
	if err := s.ReadVerified(r, d, func(io.Reader) error { return stop }); !errors.Is(err, stop) {
		t.Fatalf("err %v", err)
	}
	if _, err := s.Stat(r, d); err != nil {
		t.Fatalf("a sound blob was quarantined: %v", err)
	}
}
