# nself-post (Go) — multi-stage build
# Build context: paid/post/ directory

# Stage 1: builder
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /nself-post ./cmd/

# Stage 2: runtime
FROM alpine:3.19

RUN apk add --no-cache ca-certificates wget && \
    addgroup -S nself && \
    adduser -S -G nself nself

COPY --from=builder /nself-post /usr/local/bin/nself-post

USER nself

EXPOSE 3129

CMD ["/usr/local/bin/nself-post"]
