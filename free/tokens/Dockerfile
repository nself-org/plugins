# Build context: plugins/ root (docker build -f free/tokens/Dockerfile .)
# nself build / nself plugin install invoke this via docker-compose.plugin.yml's
# `build.context: ${NSELF_PLUGIN_DIR}/../..` + `build.dockerfile: free/tokens/Dockerfile`
# so the ../../sdk replace directive in go.mod resolves inside the image too.
FROM golang:1.26-alpine AS builder
WORKDIR /build
# Copy the SDK first for layer caching (go.mod's replace target).
COPY sdk/ sdk/
# Copy plugin source.
COPY free/tokens/ free/tokens/
WORKDIR /build/free/tokens
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /tokens ./cmd/main.go

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /tokens /usr/local/bin/tokens

EXPOSE 3107

CMD ["tokens"]
