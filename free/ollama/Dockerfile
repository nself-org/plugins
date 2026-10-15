# nself ollama plugin — multi-stage Docker build
# Stage 1: Build the Go lifecycle-hook binary
# Stage 2: Minimal runtime image

# ----- builder -----
FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod ./

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o /ollama-plugin ./cmd

# ----- runtime -----
FROM alpine:3.19

# No apk at build time (the tarball build runs with the network off): the CA bundle and zoneinfo come from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /usr/local/share/zoneinfo.zip
ENV ZONEINFO=/usr/local/share/zoneinfo.zip
# docker-cli comes from the official docker image (no apk at build time).
COPY --from=docker:27-cli /usr/local/bin/docker /usr/local/bin/docker

COPY --from=builder /ollama-plugin /usr/local/bin/ollama-plugin

# Default action is "start" — override with PLUGIN_ACTION=install for first-run
ENV PLUGIN_ACTION=start
ENV OLLAMA_MODEL=gemma-3-4b
ENV OLLAMA_CONTAINER=nself-ollama
ENV OLLAMA_HOST=http://localhost:11434

ENTRYPOINT ["/usr/local/bin/ollama-plugin"]
