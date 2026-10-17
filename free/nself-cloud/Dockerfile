# Build context: this plugin's own directory (docker build .). Every Go dependency is vendored (vendor/, including
# shared-utils and the migrate package under third_party/cli-sdk-go), so the build needs no network and never reaches
# outside its own context.
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o /plugin .

FROM alpine:3.19
# No apk at build time (the tarball build runs with the network off): the CA bundle comes from the builder stage.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /plugin /plugin
USER nobody
ENV PORT=3855
EXPOSE 3855
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:3855/health || exit 1
CMD ["/plugin"]
