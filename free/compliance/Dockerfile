FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o compliance ./cmd/main.go

# ---

FROM alpine:3.19

# No apk at build time (the tarball build runs with the network off): CA bundle and zoneinfo come from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /usr/local/share/zoneinfo.zip
ENV ZONEINFO=/usr/local/share/zoneinfo.zip

WORKDIR /app

COPY --from=builder /app/compliance .

EXPOSE 3211

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:${COMPLIANCE_PLUGIN_PORT:-3211}/health || exit 1

ENTRYPOINT ["./compliance"]
