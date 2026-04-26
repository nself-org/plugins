FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
# Copy the plugin SDK into the Docker build context.
# The go.mod replace directive maps to a local path that must exist during build.
COPY sdk/ /sdk/
RUN sed -i 's|=> ../../../plugins/sdk|=> /sdk|' go.mod && go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /nself-paypal ./cmd/

FROM alpine:3.19
RUN apk --no-cache add ca-certificates wget
COPY --from=builder /nself-paypal /usr/local/bin/nself-paypal
EXPOSE 3071
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://localhost:3071/health || exit 1
CMD ["nself-paypal"]
