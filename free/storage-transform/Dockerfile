# storage-transform — multi-stage build.
# Stage 1: build the Go binary with CGO enabled for govips (libvips C binding).
FROM golang:1.25-bookworm AS builder

# Install libvips + build dependencies.
RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config \
    libvips-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go/go.mod go/go.sum ./
RUN go mod download

COPY go/ ./
RUN CGO_ENABLED=1 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o storage-transform \
    ./cmd/main.go

# Stage 2: minimal runtime image.
FROM debian:bookworm-slim

# Install libvips runtime (same version as build stage).
RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips42 \
    ca-certificates \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

# Create cache directory with appropriate permissions.
RUN mkdir -p /var/cache/nself/images && chmod 755 /var/cache/nself/images

WORKDIR /app

COPY --from=builder /app/storage-transform .

EXPOSE 3085

# Run as non-root.
RUN useradd -r -u 1001 -g root nself
RUN chown -R nself:root /var/cache/nself
USER nself

ENV STORAGE_TRANSFORM_PORT=3085 \
    STORAGE_TRANSFORM_DISK_CACHE_DIR=/var/cache/nself/images

ENTRYPOINT ["./storage-transform"]
