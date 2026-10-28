package run

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type imageFunc func(context.Context, AttemptInfo) (string, error)

func (f imageFunc) Image(c context.Context, a AttemptInfo) (string, error) { return f(c, a) }

func TestContainerUsesOperatorImage(t *testing.T) {
	for _, img := range []string{"untrusted:latest", "example/job@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		t.Run(img, func(t *testing.T) {
			called := false
			rs := &runState{c: &Coordinator{Opts: Options{Container: true}}, hooks: Hooks{Image: imageFunc(func(context.Context, AttemptInfo) (string, error) {
				called = true
				return "operator@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", nil
			})}}
			p, err := rs.prepare(context.Background(), &AttemptInfo{Job: model.Job{Container: &model.ContainerConfig{Image: img}}})
			if err != nil || !called || p.job.Container.Image == img {
				t.Fatalf("image %v called %v error %v", p.job.Container, called, err)
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
	var cacheDigest string
	saved := Registered
	t.Cleanup(func() { Registered = saved })
	RegisterImage(imageFunc(func(context.Context, AttemptInfo) (string, error) { return opImage, nil }))
	h := &cacheHook{
		scanner: func() authz.ValueScanner { return Redactions },
		environ: func() []string { return nil },
		home:    func() string { return t.TempDir() },
		probe:   func(context.Context, model.Job, string, []string) (string, error) { return "", nil },
		warn:    func(string) {},
	}
	a := AttemptInfo{Job: model.Job{Container: &model.ContainerConfig{Image: jobImage}}}
	cacheDigest = h.imageDigest(context.Background(), a)
	if cacheDigest != opDigest {
		t.Fatalf("cache digest %q want %q (job image must not leak into the key)", cacheDigest, opDigest)
	}
	rs := &runState{c: &Coordinator{Opts: Options{Container: true}}, hooks: Hooks{Image: imageFunc(func(context.Context, AttemptInfo) (string, error) { return opImage, nil })}}
	p, err := rs.prepare(context.Background(), &a)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	execDigest := ""
	if p.job.Container != nil {
		if _, d, ok := strings.Cut(p.job.Container.Image, "@"); ok {
			execDigest = d
		}
	}
	if execDigest != cacheDigest {
		t.Fatalf("exec digest %q != cache digest %q", execDigest, cacheDigest)
	}
}
