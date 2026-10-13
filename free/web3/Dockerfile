FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -ldflags="-w -s" -o /plugin ./cmd/

FROM alpine:3.19
RUN addgroup -S nself && adduser -S nself -G nself
COPY --from=builder /plugin /plugin
USER nself
EXPOSE 3128
ENTRYPOINT ["/plugin"]
