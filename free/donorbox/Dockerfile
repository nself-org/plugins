FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.* ./
COPY ../../sdk /sdk
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /donorbox ./cmd/

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata wget
COPY --from=builder /donorbox /usr/local/bin/donorbox
EXPOSE 3074
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://localhost:3074/health || exit 1
CMD ["donorbox"]
