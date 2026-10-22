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

// The descriptor ReadVerified reads is the one it checked: a blob cut short or
// grown after the open is refused by size, and one swapped by rename after the
// open cannot change what the consumer read.
func TestReadVerifiedRefusesATruncatedOrGrownBlob(t *testing.T) {
	for name, mutate := range map[string]func(p string){
		"truncated": func(p string) { _ = os.Truncate(p, 100) },
		"grown": func(p string) {
			f, _ := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
			_, _ = f.Write([]byte("tail"))
			_ = f.Close()
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, r := testStore(t), CacheRealm("ns")
			d := putBlob(t, s, r, bytes.Repeat([]byte("z"), 4096))
			p, _ := s.blobPath(r, d)
			s.beforeHash = func() { mutate(p) }
			err := s.ReadVerified(r, d, func(in io.Reader) error { _, e := io.Copy(io.Discard, in); return e })
			if !errors.Is(err, ErrSizeMismatch) && !errors.Is(err, ErrDigestMismatch) {
				t.Fatalf("err %v, want a size or digest mismatch", err)
			}
			if _, err := s.Stat(r, d); err == nil {
				t.Fatal("the damaged blob was left in place")
			}
		})
	}
}

func TestReadVerifiedIgnoresASwapByRenameAfterTheOpen(t *testing.T) {
	s, r := testStore(t), CacheRealm("ns")
	want := bytes.Repeat([]byte("good"), 1024)
	d := putBlob(t, s, r, want)
	p, _ := s.blobPath(r, d)
	s.beforeHash = func() {
		evil := p + ".swap"
		_ = os.WriteFile(evil, bytes.Repeat([]byte("evil"), 1024), 0o600)
		_ = os.Rename(evil, p)
	}
	var got []byte
	err := s.ReadVerified(r, d, func(in io.Reader) error {
		var e error
		got, e = io.ReadAll(in)
		return e
	})
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("a swapped file changed what was read: %v", err)
	}
}

func TestReadVerifiedContextStopsTheDrain(t *testing.T) {
	for _, consume := range []bool{false, true} {
		t.Run(map[bool]string{false: "drain", true: "consumer"}[consume], func(t *testing.T) {
			s, realm := testStore(t), CacheRealm("ns")
			digest := putBlob(t, s, realm, bytes.Repeat([]byte("data"), 4096))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := s.ReadVerifiedContext(ctx, realm, digest, func(r io.Reader) error {
				cancel()
				if consume {
					_, err := io.Copy(io.Discard, r)
					return err
				}
				return nil
			})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want cancellation", err)
			}
			if _, err := s.Stat(realm, digest); err != nil {
				t.Fatalf("cancelled verification quarantined valid blob: %v", err)
			}
		})
	}
}
