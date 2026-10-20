package run

// Resolve the pinned container digest used in cache identity.

import (
	"context"
	"strings"
)

// imageDigest is the pinned digest of the job's container image, "" if unknown.
func (h *cacheHook) imageDigest(ctx context.Context, a AttemptInfo) string {
	img := ""
	if a.Job.Container != nil {
		img = a.Job.Container.Image
	}
	if img == "" && Registered.Image != nil {
		img, _ = Registered.Image.Image(ctx, a)
	}
	if _, d, ok := strings.Cut(img, "@"); ok && strings.HasPrefix(d, "sha256:") {
		return d
	}
	return ""
}
