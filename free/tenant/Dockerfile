# CLI-proxy plugin: this builds the nself-tenant binary for distribution.
# There is no runtime container/service — nself installs the binary at
# ~/.nself/plugins/bin/nself-tenant and execs it directly; this Dockerfile
# exists only so CI can build+lint the plugin the same way service plugins
# are built, and to hand the resulting /plugin binary to the release pipeline.
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
# Two binaries: this plugin provides both `nself tenant` and
# `nself billing`.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /nself-tenant ./cmd/tenant/ \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /nself-billing ./cmd/billing/

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /nself-tenant /nself-tenant
COPY --from=builder /nself-billing /nself-billing
ENTRYPOINT ["/nself-tenant"]
