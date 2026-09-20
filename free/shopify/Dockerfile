# Build context: this plugin's own directory (${NSELF_PLUGIN_DIR}/shopify), matching
# the ai/claw/mux/voice plugin pattern — nself installs a plugin flat with no
# sibling sdk/ directory. The old `COPY plugins/sdk /plugins/sdk` line assumed a
# `plugins/sdk` subdirectory inside this plugin's own directory, which never
# existed here — that COPY always failed.
#
# go.mod's `replace github.com/nself-org/plugin-sdk => ../../sdk` (also fixed in
# this change from a stale `../../../plugins/sdk` that resolved outside the repo)
# cannot resolve once the build context is this plugin's own directory. That's
# fine: vendor/ (committed, `go mod vendor`) already contains
# github.com/nself-org/plugin-sdk, so the build below runs in vendor mode
# (-mod=vendor) and never touches the network or the replace path.
# Re-vendor after any go.mod/go.sum change: `go mod vendor` from this dir.
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -ldflags="-w -s" -o /plugin ./cmd/

FROM alpine:3.19
RUN addgroup -S nself && adduser -S nself -G nself
COPY --from=builder /plugin /plugin
USER nself
ENTRYPOINT ["/plugin"]
