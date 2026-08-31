# nself-linkedin (Go) — multi-stage build
# Build context: paid/linkedin/ directory

# Stage 1: builder
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /nself-linkedin ./cmd/

# Stage 2: runtime
FROM alpine:3.19

RUN apk add --no-cache ca-certificates wget && \
    addgroup -S nself && \
    adduser -S -G nself nself

COPY --from=builder /nself-linkedin /usr/local/bin/nself-linkedin

USER nself

EXPOSE 3722

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:3722/health || exit 1

CMD ["/usr/local/bin/nself-linkedin"]
