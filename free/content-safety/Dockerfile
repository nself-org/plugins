# nself content-safety — multi-stage Go build
# Build context: this plugin dir (docker build -t nself-org/plugin-content-safety:latest .)
# The module is fully vendored (vendor/), so the build never touches the network.

# Stage 1: builder
FROM golang:1.26-alpine AS builder
ENV GOTOOLCHAIN=local
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -ldflags="-w -s" -o /plugin ./cmd/

# Stage 2: runtime
FROM alpine:3.19
# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN addgroup -S nself && adduser -S nself -G nself
COPY --from=builder /plugin /plugin
USER nself

ENV CONTENT_SAFETY_PLUGIN_PORT=3213
EXPOSE 3213

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:${CONTENT_SAFETY_PLUGIN_PORT:-3213}/health || exit 1

ENTRYPOINT ["/plugin"]
