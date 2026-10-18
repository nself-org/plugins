package run

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type mountCache struct {
	fakeCache
	mounts []Mount
}

func (c *mountCache) Before(context.Context, AttemptInfo) (CacheSetup, error) {
	return CacheSetup{Mounts: c.mounts}, nil
}

// A CacheHook mount reaches the executor: a read-only mount on a process job
// is refused there (E611) instead of being passed on as a writable path.
func TestCacheMountsReachTheExecutor(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Hooks.Cache = &mountCache{mounts: []Mount{{Host: r.Home, Target: "/cache", ReadOnly: true}}}
	res := r.run(t)
	if o := outcome(res, "a"); o.State != StateFailed || res.Exit == 0 {
		t.Fatalf("a: %+v exit %d", o, res.Exit)
	}
}
