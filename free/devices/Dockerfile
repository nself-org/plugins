FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o /devices ./cmd/main.go

FROM alpine:3.19
# No apk at build time (the tarball build runs with the network off): CA bundle and zoneinfo come from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /usr/local/share/zoneinfo.zip
ENV ZONEINFO=/usr/local/share/zoneinfo.zip

WORKDIR /app
COPY --from=builder /devices .

EXPOSE 3603

ENTRYPOINT ["/app/devices"]
