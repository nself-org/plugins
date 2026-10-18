# Stage 1: Build the Go service binary.
FROM golang:1.26-alpine AS builder

WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /functions-v8 ./cmd

# Stage 2: Runtime image with Deno bundled.
# denoland/deno provides the deno binary at /usr/local/bin/deno.
FROM denoland/deno:2.3.3

# Copy the compiled Go binary.
COPY --from=builder /functions-v8 /usr/local/bin/functions-v8

# Deno needs a home dir for caching imports.
ENV DENO_DIR=/var/cache/deno
RUN mkdir -p /var/cache/deno && chmod 755 /var/cache/deno

# Pre-warm the Deno module cache for the std library used by functions.
# This avoids network calls on first invocation in air-gapped or slow environments.
RUN deno cache https://deno.land/std@0.208.0/http/server.ts || true

ENV DENO_BINARY_PATH=/usr/local/bin/deno
ENV FUNCTIONS_EDGE_PORT=3088
ENV FUNCTIONS_EDGE_POOL_SIZE=5
ENV FUNCTIONS_EDGE_MAX_DURATION_MS=30000
ENV FUNCTIONS_EDGE_MAX_MEMORY_MB=128
ENV FUNCTIONS_EDGE_LOG_RETENTION_DAYS=7

EXPOSE 3088

# The deno image ships no wget or curl: probe /health with the bundled deno binary.
HEALTHCHECK --interval=15s --timeout=5s --start-period=5s --retries=3 \
  CMD ["deno", "eval", "--allow-net=127.0.0.1:3088", "const r = await fetch('http://127.0.0.1:3088/health'); Deno.exit(r.ok ? 0 : 1)"]

ENTRYPOINT ["/usr/local/bin/functions-v8"]
