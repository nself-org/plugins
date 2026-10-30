package report

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestResultContextPinnedOrUnreadable(t *testing.T) {
	if ResultContextFor(false, true, "") != Context || ResultContextFor(true, true, "") != WatchContext ||
		ResultContextFor(false, false, "") != WatchContext || ResultContextFor(true, true, Context) != Context {
		t.Fatal("rule")
	}
	now := time.Unix(1000, 0)
	calls := 0
	var pinned bool
	var rerr error
	rc := &ResultContexts{Now: func() time.Time { return now }, Read: func(context.Context, string, string) (bool, error) {
		calls++
		return pinned, rerr
	}}
	ctx := context.Background()
	if got := rc.ResolveResultContext(ctx, "o", "r", ""); got != Context {
		t.Fatal(got)
	}
	pinned = true
	if got := rc.ResolveResultContext(ctx, "o", "r", ""); got != Context || calls != 1 {
		t.Fatalf("cached read expected: %s %d", got, calls)
	}
	now = now.Add(ProtectionTTL)
	if got := rc.ResolveResultContext(ctx, "o", "r", ""); got != WatchContext || calls != 2 {
		t.Fatalf("pinned after ttl: %s %d", got, calls)
	}
	now = now.Add(ProtectionTTL)
	rerr = errors.New("403")
	if got := rc.ResolveResultContext(ctx, "o", "r", ""); got != WatchContext {
		t.Fatal(got)
	}
	if got := rc.ResolveResultContext(ctx, "o", "r", "nself-ci"); got != Context {
		t.Fatal("override wins")
	}
}
