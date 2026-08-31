# plugin-home — Home automation bridge (port 3127)
# Build context: plugins-pro/paid/ root (depends on ../shared via replace).
# Multi-stage Go build; runs as non-root; HEALTHCHECK hits /health.

FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY home/go.mod home/go.sum ./home/
COPY shared/ ./shared/
WORKDIR /app/home
RUN go mod download
COPY home/ .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /plugin ./cmd/

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata wget && \
    addgroup -S nself && adduser -S nself -G nself
COPY --from=builder /plugin /plugin
USER nself
ENV PORT=3127
EXPOSE 3127

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:${PORT:-3127}/health || exit 1

ENTRYPOINT ["/plugin"]
