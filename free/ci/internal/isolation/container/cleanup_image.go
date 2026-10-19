package container

// Purpose: the image the post-job cleanup helper runs (D-0963).
// Inputs:  none. Outputs: CleanupImage, pinned by the multi-arch index digest.
// Constraints: never a tag (ADR 0030) and never the job's image: a distroless image has no sh, and a hostile one
// can make its sh do nothing, which leaves job-UID files in the workspace. The same digest is the
// images.d/ci-cleanup.json fragment, so the daily image probe covers it. Re-pin with
// `crane digest docker.io/library/busybox:<tag>`, then regenerate images.json.

// CleanupImage is busybox 1.37.0 (index digest, linux/amd64 and linux/arm64 present), resolved 2026-10-19.
const CleanupImage = "docker.io/library/busybox@sha256:bdf57e528e45e4433820e045b29b4597825a1c9e38353532d90a01445013f82e"
