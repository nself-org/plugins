FROM golang:1.26-alpine AS builder
ENV GOTOOLCHAIN=local
WORKDIR /app

COPY go.mod go.sum ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /auth-enterprise ./cmd

FROM alpine:3.19

# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

RUN addgroup -S nself && adduser -S nself -G nself
COPY --from=builder /auth-enterprise /auth-enterprise
USER nself
EXPOSE 3826
ENTRYPOINT ["/auth-enterprise"]
