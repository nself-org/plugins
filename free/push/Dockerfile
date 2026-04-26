# nself-push plugin — multi-stage Go build
# Build context: plugins/ root (docker build -f free/push/Dockerfile .)

# Stage 1: builder
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Copy SDK first for layer caching.
COPY sdk/ sdk/

# Copy plugin source.
COPY free/push/ free/push/

WORKDIR /build/free/push
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /nself-push ./cmd/main.go

# Stage 2: minimal runtime
FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget

COPY --from=builder /nself-push /usr/local/bin/nself-push

ENV PORT=3053
EXPOSE 3053

HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://localhost:3053/health || exit 1

CMD ["/usr/local/bin/nself-push"]
