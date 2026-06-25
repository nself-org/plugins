# nself ci plugin — multi-stage Docker build
# Stage 1: Build the Go binary
# Stage 2: Minimal runtime image

# ----- builder -----
FROM golang:1.22-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /ci-plugin ./cmd

# ----- runtime -----
FROM alpine:3.19

# gitleaks is required for the secret-scan gate
RUN apk add --no-cache ca-certificates gitleaks git

COPY --from=builder /ci-plugin /usr/local/bin/ci-plugin

ENTRYPOINT ["/usr/local/bin/ci-plugin"]
