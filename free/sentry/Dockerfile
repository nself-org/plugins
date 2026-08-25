# CLI-proxy plugin: this builds the nself-sentry binary for distribution.
# There is no runtime container/service — nself installs the binary at
# ~/.nself/plugins/bin/nself-sentry and execs it directly; this Dockerfile
# exists only so CI can build+lint the plugin the same way service plugins
# are built, and to hand the resulting /plugin binary to the release pipeline.
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
# Two binaries: this plugin provides both `nself sentry` and
# `nself sentry-server`.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /nself-sentry ./cmd/sentry/ \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /nself-sentry-server ./cmd/sentry-server/

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /nself-sentry /nself-sentry
COPY --from=builder /nself-sentry-server /nself-sentry-server
ENTRYPOINT ["/nself-sentry"]
