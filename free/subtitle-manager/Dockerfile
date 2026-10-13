# Build context: this plugin's own directory (${NSELF_PLUGIN_DIR}/subtitle-manager),
# matching the ai/claw/mux/voice plugin pattern — nself installs a plugin flat
# with no sibling sdk/ directory. Run: docker build -f Dockerfile . (from within this dir)
#
# go.mod's `replace github.com/nself-org/plugin-sdk => ../../sdk` still points
# at the plugins/ repo-root layout and cannot resolve once the build context is
# this plugin's own directory. That's fine: vendor/ (committed, `go mod vendor`)
# already contains github.com/nself-org/plugin-sdk, so the build below runs in
# vendor mode (-mod=vendor) and never touches the network or the replace path.
# Re-vendor after any go.mod/go.sum change: `go mod vendor` from this dir.
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o /subtitle-manager ./cmd/main.go

FROM alpine:3.20
# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /zoneinfo.zip
ENV ZONEINFO=/zoneinfo.zip
# python3/ffsubsync (optional sync fallback) are not baked in: pip needs the network, and the image never installed ffsubsync.
COPY --from=builder /subtitle-manager /usr/local/bin/subtitle-manager
EXPOSE 3204
CMD ["subtitle-manager"]
