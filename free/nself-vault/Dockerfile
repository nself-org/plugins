# Build context: this plugin's own directory (docker build .). Every Go dependency is vendored (vendor/, including
# the migrate package under third_party/cli-sdk-go), so the build needs no network and never reaches outside its context.
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o /plugin ./cmd/nself-vault

FROM alpine:3.19
# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /plugin /plugin
USER nobody
# The bind host is 0.0.0.0 inside the container so the published port is reachable.
ENV VAULT_BIND=0.0.0.0 VAULT_PORT=3823
EXPOSE 3823
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:3823/health || exit 1
CMD ["/plugin"]
