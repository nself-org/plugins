FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./

COPY cmd/ cmd/
COPY internal/ internal/
COPY migrations/ migrations/
COPY third_party/ third_party/
COPY vendor/ vendor/

RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o /nself-jobs ./cmd

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /nself-jobs /usr/local/bin/nself-jobs

EXPOSE 3105

ENTRYPOINT ["nself-jobs"]
