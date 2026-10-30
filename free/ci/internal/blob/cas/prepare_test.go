package cas

// Purpose: Prepare writes and hashes a blob without making it visible; Publish
// makes it so, Discard leaves nothing (D-1515).

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestPrepareIsInvisibleUntilPublished(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("namespace")
	data := bytes.Repeat([]byte("prepared"), 5000)
	p, err := s.Prepare(context.Background(), r, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stat(r, digestOf(data)); err == nil {
		t.Fatal("a prepared blob is visible before Publish")
	}
	d, n, err := p.Publish()
	if err != nil || d != digestOf(data) || n != int64(len(data)) || p.Size() != n {
		t.Fatalf("publish %q %d %v", d, n, err)
	}
	if _, err := s.Stat(r, d); err != nil {
		t.Fatalf("published blob missing: %v", err)
	}
}

func TestPrepareDiscardLeavesNothing(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("namespace")
	data := []byte("discard me")
	p, err := s.Prepare(context.Background(), r, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	p.Discard()
	if _, err := s.Stat(r, digestOf(data)); err == nil {
		t.Fatal("a discarded blob is in the store")
	}
}

func TestPrepareReadErrorLeavesNothing(t *testing.T) {
	s := testStore(t)
	boom := errors.New("boom")
	if _, err := s.Prepare(context.Background(), CacheRealm("namespace"), errReader{boom}); !errors.Is(err, boom) {
		t.Fatalf("err %v", err)
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }
