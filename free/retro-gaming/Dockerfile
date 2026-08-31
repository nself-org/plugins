FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o /plugin ./cmd/

FROM alpine:3.19
RUN addgroup -S nself && adduser -S nself -G nself
COPY --from=builder /plugin /plugin
USER nself
EXPOSE 3124
ENTRYPOINT ["/plugin"]
