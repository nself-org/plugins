// Package gvisor is the sandboxed-container isolation profile: the hardened container profile
// run under gVisor's runsc runtime (ADR 0025 level `sandboxed-container`, Epic D9).
package gvisor

// Purpose: the image the real-start probe runs.
// Inputs:  none. Outputs: ProbeImage, pinned by the multi-arch index digest.
// Constraints: never a tag (ADR 0030); the same digest is the images.d/ci-gvisor-probe.json
// fragment, so the daily image probe covers it. Re-pin with
// `crane digest docker.io/library/busybox:<tag>`, then regenerate images.json.

// ProbeImage is busybox 1.37.0 (index digest, linux/amd64 and linux/arm64 present), resolved 2026-10-19.
const ProbeImage = "docker.io/library/busybox@sha256:bdf57e528e45e4433820e045b29b4597825a1c9e38353532d90a01445013f82e"
