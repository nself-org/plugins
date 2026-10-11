# CLI-proxy plugin: this builds the nself-k8s binary for distribution.
# There is no runtime container/service: nself installs the binary at
# ~/.nself/plugins/bin/nself-k8s and execs it directly; this Dockerfile
# exists only so CI can build+lint the plugin the same way service plugins
# are built, and to hand the resulting /plugin binary to the release pipeline.
# Modules are vendored (go mod vendor), so the build needs no network
# (conformance rule `build` runs it with --network none).
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o /plugin ./cmd/

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /plugin /plugin
ENTRYPOINT ["/plugin"]
