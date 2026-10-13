# Build context: this plugin's own directory (${NSELF_PLUGIN_DIR}/link-preview),
# matching the ai/claw/mux/voice plugin pattern — nself installs a plugin flat
# with no sibling sdk/ directory. Run: docker build -f Dockerfile . (from within this dir)
#
# go.mod's replace directives point inside this directory (third_party/sdk,
# third_party/cli-sdk-go) and vendor/ (committed, `go mod vendor`) holds every
# module, so the build below runs in vendor mode (-mod=vendor) and never
# touches the network.
# Re-vendor after any go.mod/go.sum change: `go mod vendor` from this dir.
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o /plugin ./cmd/

FROM alpine:3.19
# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /plugin /plugin
EXPOSE 3718
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://localhost:3718/health || exit 1
CMD ["/plugin"]
