package run

import (
	"context"
	"sync"
	"sync/atomic"
)

// rec records hook calls from concurrent jobs.
type rec struct {
	mu sync.Mutex
	s  []string
}

func (r *rec) add(v string) { r.mu.Lock(); r.s = append(r.s, v); r.mu.Unlock() }
func (r *rec) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.s...)
}

type startFn func(AttemptInfo) error

func (f startFn) AttemptStart(_ context.Context, a AttemptInfo) error { return f(a) }

type afterFn func(RunInfo) error

func (f afterFn) AfterRun(_ context.Context, r RunInfo) error { return f(r) }

type fin struct {
	name  string
	order *rec
}

func (f fin) Name() string { return f.name }
func (f fin) Finalize(context.Context, RunInfo) error {
	f.order.add("fin:" + f.name)
	return nil
}

type fakeCache struct{ after atomic.Int32 }

func (c *fakeCache) Before(_ context.Context, a AttemptInfo) (CacheSetup, error) {
	return CacheSetup{Env: map[string]string{"FROM_CACHE": "yes"}, Mounts: []Mount{{Host: a.Home, Target: "/t"}}}, nil
}

func (c *fakeCache) After(context.Context, AttemptInfo, Outcome) error { c.after.Add(1); return nil }
