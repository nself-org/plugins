# Build context: this plugin's own directory (${NSELF_PLUGIN_DIR}/donorbox), matching
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
RUN CGO_ENABLED=0 GOOS=linux go build -tags timetzdata -mod=vendor -ldflags="-s -w" -o /donorbox ./cmd/

FROM alpine:3.20
# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage and time zone data is compiled in (-tags timetzdata).
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /donorbox /usr/local/bin/donorbox
EXPOSE 3074
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://localhost:3074/health || exit 1
CMD ["donorbox"]
