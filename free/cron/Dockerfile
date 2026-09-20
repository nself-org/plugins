# nself-cron — multi-stage Go build
# Build context: this plugin's own directory (${NSELF_PLUGIN_DIR}/cron), matching
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

RUN apk add --no-cache git

WORKDIR /build

# Copy plugin source (context root == this plugin's own directory).
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /nself-cron ./cmd/main.go

# Stage 2: runtime
FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget

COPY --from=builder /nself-cron /usr/local/bin/nself-cron

ENV PORT=3051
EXPOSE 3051

CMD ["/usr/local/bin/nself-cron"]
