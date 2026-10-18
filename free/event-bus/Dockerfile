# Build stage: modules come from the committed vendor/ tree, so the build needs no network.
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o event-bus ./cmd/

# Runtime stage: alpine, so the compose healthcheck can probe /health with busybox wget (scratch has no shell or wget)
FROM alpine:3.19
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /build/event-bus /event-bus
EXPOSE 8212
ENTRYPOINT ["/event-bus"]
