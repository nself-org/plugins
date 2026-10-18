FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-w -s" -o /plugin-tenant-controller ./cmd/

FROM alpine:3.19
# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN addgroup -S nself && adduser -S nself -G nself
COPY --from=builder /plugin-tenant-controller /plugin-tenant-controller
USER nself
EXPOSE 3750
ENTRYPOINT ["/plugin-tenant-controller"]
