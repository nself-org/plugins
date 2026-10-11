package internal

import (
	"context"
	"errors"
	"testing"
)

// delStore records Delete calls; every other ObjectStore method panics if reached.
type delStore struct {
	ObjectStore
	deleted []string
}

func (d *delStore) Delete(_ context.Context, bucket, key string) error {
	d.deleted = append(d.deleted, bucket+"/"+key)
	return nil
}

// The pool is concrete (*pgxpool.Pool), so the re-check is tested through cleanOrphan, which holds
// the decision removeOrphan makes.
func TestCleanOrphan_recheckDecidesTheDelete(t *testing.T) {
	cases := []struct {
		name       string
		exists     bool
		err        error
		wantDelete bool
	}{
		{"concurrent PUT committed the row", true, nil, false},
		{"no row", false, nil, true},
		{"re-check failed", false, errors.New("db down"), false},
	}
	for _, c := range cases {
		st := &delStore{}
		cleanOrphan(st, "bkt", "acct/k", func(ctx context.Context) (bool, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Errorf("%s: re-check context has no deadline", c.name)
			}
			return c.exists, c.err
		})
		if got := len(st.deleted) == 1; got != c.wantDelete {
			t.Errorf("%s: deleted=%v, want delete=%v", c.name, st.deleted, c.wantDelete)
		}
		if c.wantDelete && st.deleted[0] != "bkt/acct/k" {
			t.Errorf("%s: deleted %v", c.name, st.deleted)
		}
	}
}
