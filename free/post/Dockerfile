# nself-post (Go) — multi-stage build
#
# Build context: this plugin's own directory (${NSELF_PLUGIN_DIR}/post), matching
# the ai/claw/mux/voice plugin pattern — nself installs a plugin flat with no
# sibling shared-utils/ directory.
#
# go.mod's `replace github.com/nself-org/plugins/free/shared-utils => ../shared-utils`
# cannot resolve once the build context is this plugin's own directory (the old
# module download step here always failed once that replace target left the
# build context). That's fine: vendor/ (committed, `go mod vendor`) already
# contains github.com/nself-org/plugins/free/shared-utils, so the build below
# runs in vendor mode (-mod=vendor) and never touches the network or the
# replace path. Re-vendor after any go.mod/go.sum change: `go mod vendor` from
# this dir.

# Stage 1: builder
FROM golang:1.26-alpine AS builder

WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /nself-post ./cmd/

# Stage 2: runtime
FROM alpine:3.19

# No apk at build time (the tarball build runs with the network off): the CA bundle and zoneinfo come from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /usr/local/share/zoneinfo.zip
ENV ZONEINFO=/usr/local/share/zoneinfo.zip

RUN addgroup -S nself && \
    adduser -S -G nself nself

COPY --from=builder /nself-post /usr/local/bin/nself-post

USER nself

EXPOSE 3129

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:${PORT:-3129}/health || exit 1

CMD ["/usr/local/bin/nself-post"]
