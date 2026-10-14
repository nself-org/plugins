FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o /app/plugin-clawde ./cmd/main/main.go

FROM alpine:3.19
# No apk at build time (the tarball build runs with the network off): the CA bundle and zoneinfo come from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /usr/local/share/zoneinfo.zip
ENV ZONEINFO=/usr/local/share/zoneinfo.zip
WORKDIR /app
COPY --from=builder /app/plugin-clawde .
COPY migrations/ ./migrations/
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:${NSELF_CLAWDE_PORT:-3847}/health || exit 1
EXPOSE 3847
CMD ["./plugin-clawde"]
