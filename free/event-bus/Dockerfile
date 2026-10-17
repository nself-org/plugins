# Build stage
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY go/go.mod go/go.sum ./
RUN go mod download
COPY go/ .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o event-bus ./cmd/

# Runtime stage: alpine with wget, so the compose healthcheck can probe /health (scratch has no shell or wget)
FROM alpine:3.19
RUN apk add --no-cache wget ca-certificates
COPY --from=builder /build/event-bus /event-bus
EXPOSE 8212
ENTRYPOINT ["/event-bus"]
