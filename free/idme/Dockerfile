FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o idme-plugin ./cmd/main.go

FROM alpine:3.19

# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /zoneinfo.zip
ENV ZONEINFO=/zoneinfo.zip

WORKDIR /app
COPY --from=builder /app/idme-plugin .

EXPOSE 3010

HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:${PORT:-3010}/health || exit 1

ENTRYPOINT ["/app/idme-plugin"]
