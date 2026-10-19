package run

// Purpose: D-0998 regression. Under --container a job's own container.image
// must not bypass the digest-pinned toolchain image.

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

const (
	pinnedToolchain = "ghcr.io/nself-org/ci-toolchain@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pinnedJobImage  = "registry.example/job:1@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type fixedImage string

func (f fixedImage) Image(context.Context, AttemptInfo) (string, error) { return string(f), nil }

func prepareImage(t *testing.T, hook ContainerImageHook, container bool, jobImage string) (string, error) {
	t.Helper()
	rs := &runState{c: &Coordinator{Opts: Options{Container: container}}, hooks: Hooks{Image: hook}}
	info := &AttemptInfo{Job: model.Job{Run: []string{"true"}}}
	if jobImage != "" {
		info.Job.Container = &model.ContainerConfig{Image: jobImage}
	}
	p, err := rs.prepare(context.Background(), info)
	if err != nil || p.job.Container == nil {
		return "", err
	}
	return p.job.Container.Image, nil
}

// TestContainerIgnoresUnpinnedJobImage: a job naming a tag-only image runs in
// the pinned toolchain image instead.
func TestContainerIgnoresUnpinnedJobImage(t *testing.T) {
	for _, img := range []string{"evil:latest", "alpine:3.19", "x@sha256:abc", ""} {
		got, err := prepareImage(t, fixedImage(pinnedToolchain), true, img)
		if err != nil || got != pinnedToolchain {
			t.Fatalf("job image %q: got %q err %v, want the toolchain image", img, got, err)
		}
	}
}

// TestContainerKeepsDigestPinnedJobImage: a job image pinned by digest is honoured.
func TestContainerKeepsDigestPinnedJobImage(t *testing.T) {
	got, err := prepareImage(t, fixedImage(pinnedToolchain), true, pinnedJobImage)
	if err != nil || got != pinnedJobImage {
		t.Fatalf("got %q err %v, want %q", got, err, pinnedJobImage)
	}
}

// TestContainerUnpinnedJobImageNeedsRegisteredToolchain: with no toolchain
// image registered an unpinned job image is refused, never run.
func TestContainerUnpinnedJobImageNeedsRegisteredToolchain(t *testing.T) {
	_, err := prepareImage(t, nil, true, "evil:latest")
	if err == nil || !strings.Contains(err.Error(), "pinned toolchain image") {
		t.Fatalf("err %v, want the pinned-toolchain error", err)
	}
}

// TestNoContainerLeavesJobImageAlone: without --container nothing changes.
func TestNoContainerLeavesJobImageAlone(t *testing.T) {
	got, err := prepareImage(t, fixedImage(pinnedToolchain), false, "alpine:3.19")
	if err != nil || got != "alpine:3.19" {
		t.Fatalf("got %q err %v", got, err)
	}
}
