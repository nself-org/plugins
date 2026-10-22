package coordinator

import (
	"context"
	"testing"
	"time"
)

type maintenanceClock struct {
	*fakeClock
	cancel context.CancelFunc
}

func (c maintenanceClock) After(d time.Duration) <-chan time.Time {
	c.cancel()
	return c.fakeClock.After(d)
}

func TestMaintenancePrunesProviderMarks(t *testing.T) {
	r := newRig(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.c.cfg.Clock = maintenanceClock{r.clk, cancel}
	cases := []struct {
		key  string
		age  time.Duration
		keep bool
	}{
		{"term|old", 31 * 24 * time.Hour, false}, {"run|old", 8 * 24 * time.Hour, false}, {"demand|new", time.Hour, true},
	}
	for _, tc := range cases {
		if err := r.st.ProviderMarkPut(ctx, "github-reconcile", tc.key, r.clk.Now().Add(-tc.age)); err != nil {
			t.Fatal(err)
		}
	}
	r.c.maintain(ctx)
	for _, tc := range cases {
		_, ok, err := r.st.ProviderMarkGet(context.Background(), "github-reconcile", tc.key)
		if err != nil || ok != tc.keep {
			t.Fatalf("key=%s kept=%v want=%v err=%v", tc.key, ok, tc.keep, err)
		}
	}
}
