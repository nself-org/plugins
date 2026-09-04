# Build context: this plugin's own install directory
# (docker build -f Dockerfile .), i.e. ${NSELF_PLUGIN_DIR}/tokens
# after `nself plugin install tokens` — matches the flat layout
# internal/plugin/flatten_extracted.go produces and
# internal/build/plugins.go:canonicalDockerfile expects. sdk/ is
# vendored into this directory (see go.mod's replace directive) so
# the build never reaches outside its own context (P6-E3-W2-S1-T5
# 2026-09-03: the prior ${NSELF_PLUGIN_DIR}/../.. context assumed a
# per-plugin NSELF_PLUGIN_DIR the CLI never sets — it's always the
# flat global plugin root).
FROM golang:1.26-alpine AS builder
WORKDIR /build
# Copy the vendored SDK first for layer caching (go.mod's replace target).
COPY sdk/ sdk/
COPY go.mod go.sum ./
RUN go mod download
# Copy the rest of the plugin source.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /tokens ./cmd/main.go

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /tokens /usr/local/bin/tokens

EXPOSE 3107

CMD ["tokens"]
