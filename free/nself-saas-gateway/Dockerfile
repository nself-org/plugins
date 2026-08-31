# nself-saas-gateway — multi-stage build
# Build context: paid/ (parent of this plugin's directory)
# Run: docker build -f nself-saas-gateway/Dockerfile -t nself-saas-gateway:latest .
# (from the paid/ directory, not from within nself-saas-gateway/)
#
# Why parent context: go.mod has a replace directive for the shared module
# at ../shared. Docker requires the sibling to be in the build context.
#
# Port: 3848 (F10-PORT-REGISTRY: nself-sentry-api)

# Stage 1: builder
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /build

# Copy the sibling module required by the replace directive.
COPY shared/ ./shared/

# Copy this plugin's module.
COPY nself-saas-gateway/ ./nself-saas-gateway/

WORKDIR /build/nself-saas-gateway

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /nself-saas-gateway . && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /nself-saas-seed ./cmd/seed

# Stage 2: runtime
FROM alpine:3.19

# ca-certificates: TLS to Postgres/upstreams when not on loopback.
# wget: used by HEALTHCHECK.
RUN apk add --no-cache ca-certificates wget && \
    addgroup -S nself && \
    adduser -S -G nself nself

COPY --from=builder /nself-saas-gateway /usr/local/bin/nself-saas-gateway
COPY --from=builder /nself-saas-seed /usr/local/bin/nself-saas-seed

USER nself

EXPOSE 3848

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:3848/health || exit 1

ENTRYPOINT ["/usr/local/bin/nself-saas-gateway"]
