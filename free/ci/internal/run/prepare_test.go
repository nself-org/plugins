package run

import (
	"context"
	"testing"

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
