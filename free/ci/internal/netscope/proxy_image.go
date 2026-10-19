package netscope

// Purpose: the minimal image the proxy sidecar runs in.
// Inputs:  none. Outputs: ProxyImage, pinned by the multi-arch index digest.
// Constraints: the digest equals the images.d/ci-netproxy.json entry (a test asserts it) so the daily image
// probe (ADR 0030) covers what the planner starts; the image holds no shell and no package manager, the
// static nself-ci binary is bind-mounted into it read-only at start.

// ProxyImage is gcr.io/distroless/static, the nonroot tag's index digest (linux/amd64 and linux/arm64
// present), resolved 2026-10-19.
const ProxyImage = "gcr.io/distroless/static@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3"
