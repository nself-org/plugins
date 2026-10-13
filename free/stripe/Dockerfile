FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o /nself-stripe ./cmd/main.go

FROM alpine:3.19

# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /zoneinfo.zip
ENV ZONEINFO=/zoneinfo.zip

COPY --from=builder /nself-stripe /usr/local/bin/nself-stripe

EXPOSE 3070

ENTRYPOINT ["nself-stripe"]
