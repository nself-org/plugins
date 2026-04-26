FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY ../../sdk /build/sdk
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /content-progress ./cmd/main.go

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /content-progress /usr/local/bin/content-progress
EXPOSE 3022
CMD ["content-progress"]
