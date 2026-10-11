FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod ./

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o /plugin-storage ./cmd

# ----

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata wget

WORKDIR /app

COPY --from=builder /plugin-storage .

EXPOSE 9007

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:${STORAGE_PLUGIN_PORT:-9007}/health || exit 1

ENTRYPOINT ["/app/plugin-storage"]
