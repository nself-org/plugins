# Build context: this plugin's own directory (${NSELF_PLUGIN_DIR}/tmdb), matching
# the ai/claw/mux/voice plugin pattern — nself installs a plugin flat with no
# sibling shared-utils/ directory.
#
# go.mod's `replace github.com/nself-org/plugins/free/shared-utils => ../shared-utils`
# cannot resolve once the build context is this plugin's own directory (the old
# `go mod download` step here always failed once that replace target left the
# build context). That's fine: vendor/ (committed, `go mod vendor`) already
# contains github.com/nself-org/plugins/free/shared-utils, so the build below
# runs in vendor mode (-mod=vendor) and never touches the network or the
# replace path. Re-vendor after any go.mod/go.sum change: `go mod vendor` from
# this dir.
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -ldflags="-w -s" -o /plugin ./cmd/

FROM alpine:3.19
RUN addgroup -S nself && adduser -S nself -G nself
COPY --from=builder /plugin /plugin
USER nself
EXPOSE 3122
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:3122/health || exit 1
ENTRYPOINT ["/plugin"]
