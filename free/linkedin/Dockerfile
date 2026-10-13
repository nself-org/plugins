# nself-linkedin (Go) — multi-stage build
# Build context: this plugin's own directory (docker build -f Dockerfile .); modules come from vendor/, no network needed.

# Stage 1: builder
FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
COPY vendor/ vendor/

COPY cmd/ cmd/
COPY internal/ internal/

RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /nself-linkedin ./cmd/

# Stage 2: runtime
FROM alpine:3.19

# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN addgroup -S nself && \
    adduser -S -G nself nself

COPY --from=builder /nself-linkedin /usr/local/bin/nself-linkedin

USER nself

EXPOSE 3722

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:3722/health || exit 1

CMD ["/usr/local/bin/nself-linkedin"]
