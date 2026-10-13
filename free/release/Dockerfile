# CLI-proxy plugin: this builds the nself-release binary for distribution.
# There is no runtime container/service — nself installs the binary at
# ~/.nself/plugins/bin/nself-release and execs it directly; this Dockerfile
# exists only so CI can build+lint the plugin the same way service plugins
# are built, and to hand the resulting /plugin binary to the release pipeline.
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.* ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o /plugin ./cmd/

FROM alpine:3.19
# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /plugin /plugin
ENTRYPOINT ["/plugin"]
