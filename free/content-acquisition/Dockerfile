FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY ../../sdk /build/sdk
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /content-acquisition ./cmd/main.go

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /content-acquisition /usr/local/bin/content-acquisition
EXPOSE 3202
CMD ["content-acquisition"]
