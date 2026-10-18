FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /audit-analytics .

FROM alpine:3.19

# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

RUN addgroup -S nself && adduser -S nself -G nself
COPY --from=builder /audit-analytics /audit-analytics
USER nself
EXPOSE 3218
ENTRYPOINT ["/audit-analytics"]
