# Build context: this plugin's own directory (${NSELF_PLUGIN_DIR}/backup), matching
# the ai/claw/mux/voice plugin pattern: nself installs a plugin flat with no
# sibling sdk/ directory. Run: docker build -f Dockerfile . (from within this dir)
#
# plugin-sdk and the cli migrate package live in third_party/ (go.mod replaces) and vendor/ (committed,
# `go mod vendor`), so the build runs in vendor mode (-mod=vendor) and never touches the network.
# Re-vendor after any go.mod/go.sum change: `go mod vendor` from this dir.
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o /plugin ./cmd/

# pg_dump and pg_restore (client 16, as before with postgresql16-client) come from the postgres image: apk needs the
# network and the tarball build runs with it off. The image's entrypoint script is cleared; the plugin is the only process.
FROM postgres:16-alpine
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /plugin /plugin
EXPOSE 3050
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://localhost:3050/health || exit 1
ENTRYPOINT []
CMD ["/plugin"]
