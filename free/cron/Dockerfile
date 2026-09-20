# nself-cron — multi-stage Go build
# Build context: this plugin's own directory (${NSELF_PLUGIN_DIR}/cron), matching
# the ai/claw/mux/voice plugin pattern — nself installs a plugin flat with no
# sibling sdk/ directory. Run: docker build -f Dockerfile . (from within this dir)
#
# go.mod's `replace github.com/nself-org/plugin-sdk => ../../sdk` still points
# at the plugins/ repo-root layout and cannot resolve once the build context is
# this plugin's own directory. That's fine: vendor/ (committed, `go mod vendor`)
# already contains github.com/nself-org/plugin-sdk, so the build below runs in
# vendor mode (-mod=vendor) and never touches the network or the replace path.
# Re-vendor after any go.mod/go.sum change: `go mod vendor` from this dir.

# Stage 1: builder
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

# Copy plugin source, including the committed vendor/ dir (context root ==
# this plugin's own directory).
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o /nself-cron ./cmd/main.go

# Stage 2: runtime
FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget

COPY --from=builder /nself-cron /usr/local/bin/nself-cron

ENV PORT=3051
EXPOSE 3051

CMD ["/usr/local/bin/nself-cron"]
