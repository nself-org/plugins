FROM golang:1.23-alpine AS builder

WORKDIR /build

# Copy SDK dependency first for caching
COPY ../../sdk /build/sdk

# Copy plugin source
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /feature-flags ./cmd/main.go

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /feature-flags /usr/local/bin/feature-flags

EXPOSE 3305

CMD ["feature-flags"]
