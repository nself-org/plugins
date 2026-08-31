FROM golang:1.22-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o plugin-pty ./cmd/plugin-pty/main.go

FROM alpine:3.19
RUN apk add --no-cache wget ca-certificates bash
WORKDIR /app
COPY --from=builder /app/plugin-pty .
COPY migrations/ ./migrations/
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:9100/health || exit 1
EXPOSE 9100
CMD ["./plugin-pty"]
