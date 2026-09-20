# plugin-home — Home automation bridge (port 3127)
#
# Build context: this plugin's own directory (${NSELF_PLUGIN_DIR}/home), matching
# the ai/claw/mux/voice plugin pattern — nself installs a plugin flat with no
# sibling shared-utils/ directory. The previous version of this file assumed a
# `plugins-pro/paid/` root context with a `home/` subdir and a `shared/` sibling
# copy — neither exists when the build context is this plugin's own directory.
#
# go.mod's `replace github.com/nself-org/plugins/free/shared-utils => ../shared-utils`
# cannot resolve once the build context is this plugin's own directory. That's
# fine: vendor/ (committed, `go mod vendor`) already contains
# github.com/nself-org/plugins/free/shared-utils, so the build below runs in
# vendor mode (-mod=vendor) and never touches the network or the replace path.
# Re-vendor after any go.mod/go.sum change: `go mod vendor` from this dir.
#
# Multi-stage Go build; runs as non-root; HEALTHCHECK hits /health.

FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-w -s" -o /plugin ./cmd/

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata wget && \
    addgroup -S nself && adduser -S nself -G nself
COPY --from=builder /plugin /plugin
USER nself
ENV PORT=3127
EXPOSE 3127

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:${PORT:-3127}/health || exit 1

ENTRYPOINT ["/plugin"]
