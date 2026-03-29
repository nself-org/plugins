FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.* ./
COPY ../../sdk /sdk
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /plugin ./cmd/

FROM alpine:3.19
RUN apk --no-cache add ca-certificates wget
COPY --from=builder /plugin /plugin
EXPOSE 3060
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://localhost:3060/health || exit 1
CMD ["/plugin"]
