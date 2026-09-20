# Build context: this plugin's own directory (${NSELF_PLUGIN_DIR}/paypal), matching
# the ai/claw/mux/voice plugin pattern — nself installs a plugin flat with no
# sibling sdk/ directory. The docker-compose.plugin.yml here already used
# `context: .` (the plugin's own dir), but the old Dockerfile still tried to
# `COPY sdk/ /sdk/` from a `sdk/` subdirectory that was never part of this repo
# and sed-patch go.mod's replace target at build time — both steps always failed.
#
# go.mod's `replace github.com/nself-org/plugin-sdk => ../../sdk` (also fixed in
# this change from a stale `../../../plugins/sdk` that resolved outside the repo)
# cannot resolve once the build context is this plugin's own directory. That's
# fine: vendor/ (committed, `go mod vendor`) already contains
# github.com/nself-org/plugin-sdk, so the build below runs in vendor mode
# (-mod=vendor) and never touches the network or the replace path.
# Re-vendor after any go.mod/go.sum change: `go mod vendor` from this dir.
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o /nself-paypal ./cmd/

FROM alpine:3.19
RUN apk --no-cache add ca-certificates wget
COPY --from=builder /nself-paypal /usr/local/bin/nself-paypal
EXPOSE 3071
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://localhost:3071/health || exit 1
CMD ["nself-paypal"]
