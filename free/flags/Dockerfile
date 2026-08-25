# CLI-proxy plugin: this builds the nself-flags binary for distribution.
# There is no runtime container/service — nself installs the binary at
# ~/.nself/plugins/bin/nself-flags and execs it directly; this Dockerfile
# exists only so CI can build+lint the plugin the same way service plugins
# are built, and to hand the resulting /plugin binary to the release pipeline.
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /plugin ./cmd/

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /plugin /plugin
ENTRYPOINT ["/plugin"]
