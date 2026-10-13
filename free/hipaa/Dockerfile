# Build context: this plugin's own install directory (docker build -f Dockerfile .). The Go module root is the
# plugin root (go.mod next to this file) so migrations/ can be go:embed-ed; the entry point is ./go. The module
# is fully vendored (vendor/, third_party/cli-sdk-go), so the build never touches the network.
FROM golang:1.26-alpine AS builder
ENV GOTOOLCHAIN=local
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o /plugin ./go

FROM alpine:3.19
# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /plugin /plugin
# The manifest port; the cli passes HIPAA_PLUGIN_PORT itself, this is the standalone default.
ENV HIPAA_PLUGIN_PORT=3214
EXPOSE 3214
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:${HIPAA_PLUGIN_PORT}/health || exit 1
CMD ["/plugin"]
