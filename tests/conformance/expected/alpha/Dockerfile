# alpha fixture: multi-stage Go build. Context is the plugin's own directory.
FROM golang:1.26.6 AS builder
WORKDIR /build
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o /out/alpha ./cmd

FROM alpine:latest
COPY --from=builder /out/alpha /usr/local/bin/alpha
ENV PORT=3901
EXPOSE 3901
HEALTHCHECK --interval=30s --timeout=5s CMD wget -q -O /dev/null http://127.0.0.1:3901/health || exit 1
CMD ["/usr/local/bin/alpha"]
