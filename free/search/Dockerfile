# Build context: plugins/ root (docker build -f free/search/Dockerfile .)
# nself build / nself plugin install invoke this via docker-compose.plugin.yml's
# `build.context: ${NSELF_PLUGIN_DIR}/../..` + `build.dockerfile: free/search/Dockerfile`
# so the ../../sdk replace directive in go.mod resolves inside the image too.
FROM golang:1.26-alpine AS builder
WORKDIR /build
# Copy the SDK first for layer caching (go.mod's replace target).
COPY sdk/ sdk/
# Copy plugin source.
COPY free/search/ free/search/
WORKDIR /build/free/search
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /plugin ./cmd/

FROM alpine:3.19
RUN apk --no-cache add ca-certificates wget
COPY --from=builder /plugin /plugin
EXPOSE 3302
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://localhost:3302/health || exit 1
CMD ["/plugin"]
