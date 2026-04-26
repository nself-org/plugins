FROM golang:1.25-alpine AS builder
WORKDIR /app
# Copy module files first for layer caching.
COPY go.mod go.sum ./
# Copy the SDK so the replace directive resolves during the build.
COPY ../../sdk /sdk
RUN go mod download
# Copy source.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /plugin ./cmd/

FROM alpine:3.19
RUN apk --no-cache add ca-certificates wget
COPY --from=builder /plugin /plugin
EXPOSE 3308
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://localhost:3308/health || exit 1
CMD ["/plugin"]
