# Build context: this plugin's own directory (${NSELF_PLUGIN_DIR}/backup), matching
# the ai/claw/mux/voice plugin pattern — nself installs a plugin flat with no
# sibling sdk/ directory. Run: docker build -f Dockerfile . (from within this dir)
#
# go.mod's `replace github.com/nself-org/plugin-sdk => ../../sdk` still points
# at the plugins/ repo-root layout and cannot resolve once the build context is
# this plugin's own directory. That's fine: vendor/ (committed, `go mod vendor`)
# already contains github.com/nself-org/plugin-sdk, so the build below runs in
# vendor mode (-mod=vendor) and never touches the network or the replace path.
# Re-vendor after any go.mod/go.sum change: `go mod vendor` from this dir.
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o /plugin ./cmd/

FROM alpine:3.19
RUN apk --no-cache add ca-certificates wget postgresql16-client
COPY --from=builder /plugin /plugin
EXPOSE 3050
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://localhost:3050/health || exit 1
CMD ["/plugin"]
