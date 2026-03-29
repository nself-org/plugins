# nself-cron — multi-stage Go build
# Build context: plugins/ root (docker build -f free/cron/Dockerfile .)

# Stage 1: builder
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

# Copy SDK first for caching.
COPY sdk/ sdk/

# Copy plugin source.
COPY free/cron/ free/cron/

WORKDIR /build/free/cron
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /nself-cron ./cmd/main.go

# Stage 2: runtime
FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget

COPY --from=builder /nself-cron /usr/local/bin/nself-cron

ENV PORT=3051
EXPOSE 3051

CMD ["/usr/local/bin/nself-cron"]
