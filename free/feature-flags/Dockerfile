# Build context: plugins/ root (docker build -f free/feature-flags/Dockerfile .)
# nself build / nself plugin install invoke this via docker-compose.plugin.yml's
# `build.context: ${NSELF_PLUGIN_DIR}/../..` + `build.dockerfile: free/feature-flags/Dockerfile`
# so the ../../sdk replace directive in go.mod resolves inside the image too.
FROM golang:1.26-alpine AS builder
WORKDIR /build
# Copy the SDK first for layer caching (go.mod's replace target).
COPY sdk/ sdk/
# Copy plugin source.
COPY free/feature-flags/ free/feature-flags/
WORKDIR /build/free/feature-flags
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /feature-flags ./cmd/main.go

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /feature-flags /usr/local/bin/feature-flags

EXPOSE 3305

CMD ["feature-flags"]
