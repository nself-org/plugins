FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./

COPY cmd/ cmd/
COPY internal/ internal/
COPY migrations/ migrations/
COPY third_party/ third_party/
COPY vendor/ vendor/

RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o /nself-jobs ./cmd

FROM alpine:3.20

# No apk at build time (the tarball build runs with the network off): the CA bundle and zoneinfo come from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /usr/local/share/zoneinfo.zip
ENV ZONEINFO=/usr/local/share/zoneinfo.zip

COPY --from=builder /nself-jobs /usr/local/bin/nself-jobs

EXPOSE 3105

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:${JOBS_PORT:-3105}/health || exit 1

ENTRYPOINT ["nself-jobs"]
