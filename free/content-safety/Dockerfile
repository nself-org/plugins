# nself content-safety — multi-stage Go build
# Build context: this plugin dir (docker build -t nself-org/plugin-content-safety:latest .)

# Stage 1: builder
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o /plugin ./cmd/

# Stage 2: runtime
FROM alpine:3.19
RUN apk add --no-cache ca-certificates wget && \
    addgroup -S nself && adduser -S nself -G nself
COPY --from=builder /plugin /plugin
USER nself

ENV CONTENT_SAFETY_PLUGIN_PORT=3213
EXPOSE 3213

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:${CONTENT_SAFETY_PLUGIN_PORT:-3213}/health || exit 1

ENTRYPOINT ["/plugin"]
