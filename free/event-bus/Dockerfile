# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go/go.mod go/go.sum ./
RUN go mod download
COPY go/ .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o event-bus ./cmd/

# Runtime stage — minimal scratch image
FROM scratch
COPY --from=builder /build/event-bus /event-bus
EXPOSE 8212
ENTRYPOINT ["/event-bus"]
