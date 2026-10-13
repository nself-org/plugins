FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -tags timetzdata -mod=vendor -o /devices ./cmd/main.go

FROM alpine:3.19
# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage and time zone data is compiled in (-tags timetzdata).
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

WORKDIR /app
COPY --from=builder /devices .

EXPOSE 3603

ENTRYPOINT ["/app/devices"]
