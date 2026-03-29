FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY ../../sdk /build/sdk
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /torrent-manager ./cmd/main.go

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /torrent-manager /usr/local/bin/torrent-manager
EXPOSE 3201
CMD ["torrent-manager"]
