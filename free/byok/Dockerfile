FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go/go.mod go/go.sum ./
RUN go mod download

COPY go/ .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o byok ./cmd/main.go

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/byok .

EXPOSE 3743

USER nobody

ENTRYPOINT ["./byok"]
