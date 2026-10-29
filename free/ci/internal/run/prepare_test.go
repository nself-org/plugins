package run

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type imageFunc func(context.Context, AttemptInfo) (string, error)

func (f imageFunc) Image(c context.Context, a AttemptInfo) (string, error) { return f(c, a) }

func TestContainerUsesOperatorImage(t *testing.T) {
	for _, img := range []string{"untrusted:latest", "example/job@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		t.Run(img, func(t *testing.T) {
			calls := 0
			rs := &runState{c: &Coordinator{Opts: Options{Container: true}}, hooks: Hooks{Image: imageFunc(func(context.Context, AttemptInfo) (string, error) {
				calls++
				return "operator@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", nil
			})}}
			p, err := rs.prepare(context.Background(), &AttemptInfo{Job: model.Job{Container: &model.ContainerConfig{Image: img}}})
			if err != nil || calls != 1 || p.job.Container.Image == img {
				t.Fatalf("image %v calls %v error %v", p.job.Container, calls, err)
			}
		})
	}
}

// TestCacheKeyUsesOperatorImageDigest asserts that cache restoration and
// execution share the same operator image digest, even when the job's
// configured image differs. Without early resolution in prepare.go the cache
// hook would key on the job image and fragment caches across toolchains.
func TestCacheKeyUsesOperatorImageDigest(t *testing.T) {
	const opDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	opImage := "operator@" + opDigest
	jobImage := "job/toolchain@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	saved := Registered
	t.Cleanup(func() { Registered = saved })
	RegisterImage(imageFunc(func(context.Context, AttemptInfo) (string, error) {
		t.Fatal("cache must use the resolved execution image")
		return "", nil
	}))
	a := AttemptInfo{Container: true, Job: model.Job{Container: &model.ContainerConfig{Image: jobImage}}}
	cache := &digestCache{}
	calls := 0
	rs := &runState{c: &Coordinator{Opts: Options{Container: true}}, hooks: Hooks{Cache: cache, Image: imageFunc(func(context.Context, AttemptInfo) (string, error) {
		calls++
		return opImage, nil
	})}}
	p, err := rs.prepare(context.Background(), &a)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if p.job.Container.Image != opImage || cache.digest != opDigest || calls != 1 {
		t.Fatalf("execution image %q cache digest %q image calls %d", p.job.Container.Image, cache.digest, calls)
	}
}

type digestCache struct{ digest string }

func (c *digestCache) Before(ctx context.Context, a AttemptInfo) (CacheSetup, error) {
	c.digest = (&cacheHook{}).imageDigest(ctx, a)
	return CacheSetup{}, nil
}
func (*digestCache) After(context.Context, AttemptInfo, Outcome) error { return nil }

func TestCacheDigestUsesJobImageWithoutContainerOverride(t *testing.T) {
	saved := Registered
	t.Cleanup(func() { Registered = saved })
	RegisterImage(imageFunc(func(context.Context, AttemptInfo) (string, error) {
		t.Fatal("ordinary jobs must not resolve the operator image")
		return "", nil
	}))
	for _, image := range []string{"", "job@sha256:dddd"} {
		a := AttemptInfo{}
		want := ""
		if image != "" {
			a.Job.Container = &model.ContainerConfig{Image: image}
			want = "sha256:dddd"
		}
		if got := (&cacheHook{}).imageDigest(context.Background(), a); got != want {
			t.Fatalf("image %q digest %q want %q", image, got, want)
		}
	}
}

func TestPrepareImageFailurePrecedesCache(t *testing.T) {
	for _, panicHook := range []bool{false, true} {
		cache := &fakeCache{}
		rs := &runState{c: &Coordinator{Opts: Options{Container: true}}, hooks: Hooks{Cache: cache, Image: imageFunc(func(context.Context, AttemptInfo) (string, error) {
			if panicHook {
				panic("image unavailable")
			}
			return "", errors.New("image unavailable")
		})}}
		p, err := rs.prepare(context.Background(), &AttemptInfo{})
		if err == nil || p.cacheStarted {
			t.Fatalf("panic %v error %v cache started %v", panicHook, err, p.cacheStarted)
		}
	}
}
