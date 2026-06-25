# nself ollama plugin — multi-stage Docker build
# Stage 1: Build the Go lifecycle-hook binary
# Stage 2: Minimal runtime image

# ----- builder -----
FROM golang:1.22-alpine AS builder

WORKDIR /build

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /ollama-plugin ./cmd

# ----- runtime -----
FROM alpine:3.19

RUN apk add --no-cache ca-certificates docker-cli

COPY --from=builder /ollama-plugin /usr/local/bin/ollama-plugin

# Default action is "start" — override with PLUGIN_ACTION=install for first-run
ENV PLUGIN_ACTION=start
ENV OLLAMA_MODEL=gemma-3-4b
ENV OLLAMA_CONTAINER=nself-ollama
ENV OLLAMA_HOST=http://localhost:11434

ENTRYPOINT ["/usr/local/bin/ollama-plugin"]
