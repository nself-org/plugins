FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy SDK dependency first for caching
COPY ../../sdk /build/sdk

# Copy plugin source
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /mdns ./cmd/main.go

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /mdns /usr/local/bin/mdns

EXPOSE 3216

CMD ["mdns"]
