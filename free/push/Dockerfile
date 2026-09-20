# nself-push plugin — multi-stage Go build
# Build context: this plugin's own directory (${NSELF_PLUGIN_DIR}/push), matching
# the ai/claw/mux/voice plugin pattern — nself installs a plugin flat with no
# sibling sdk/ directory. Run: docker build -f Dockerfile . (from within this dir)
#
# KNOWN ISSUE (not fixed by this change): go.mod's
#   replace github.com/nself-org/plugin-sdk => ../../sdk
# assumes a plugins/ repo-root build context and will not resolve once the
# context is this plugin's own directory — `go mod download` will fail until
# the sdk source is vendored into this plugin's package at release-tarball
# time or plugin-sdk is published as its own versioned module. Not addressed
# here; flagged for follow-up.

# Stage 1: builder
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Copy plugin source (context root == this plugin's own directory).
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /nself-push ./cmd/main.go

# Stage 2: minimal runtime
FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget

COPY --from=builder /nself-push /usr/local/bin/nself-push

ENV PORT=3053
EXPOSE 3053

HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://localhost:3053/health || exit 1

CMD ["/usr/local/bin/nself-push"]
