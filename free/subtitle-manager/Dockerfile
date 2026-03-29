FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY ../../sdk /build/sdk
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /subtitle-manager ./cmd/main.go

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata python3 py3-pip
COPY --from=builder /subtitle-manager /usr/local/bin/subtitle-manager
EXPOSE 3204
CMD ["subtitle-manager"]
