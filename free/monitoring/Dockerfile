# nself monitoring plugin — multi-stage Docker build
# Stage 1: Build the Go config-render binary
# Stage 2: Runtime image with rendered configs and compose data

# ----- builder -----
FROM golang:1.22-alpine AS builder

WORKDIR /build

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /monitoring-render ./cmd

# ----- runtime -----
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

# Copy rendered config sources into image
COPY --from=builder /build/configs /monitoring/configs
COPY --from=builder /monitoring-render /usr/local/bin/monitoring-render

ENV MONITORING_CONFIGS_SRC=/monitoring/configs
ENV MONITORING_CONFIGS_OUT=/monitoring/out

# Run as init container: renders configs then exits 0
ENTRYPOINT ["/usr/local/bin/monitoring-render"]
