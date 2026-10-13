FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.* ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o /plugin ./cmd/

FROM alpine:3.19
# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /plugin /plugin
EXPOSE 3002
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://localhost:3002/health || exit 1
CMD ["/plugin"]
