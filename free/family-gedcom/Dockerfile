# nself family-gedcom plugin — multi-stage Docker build
# Stage 1: Build the Go GEDCOM parser binary
# Stage 2: Minimal runtime image

# ----- builder -----
FROM golang:1.22-alpine AS builder

WORKDIR /build

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /family-gedcom ./cmd

# ----- runtime -----
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /family-gedcom /usr/local/bin/family-gedcom

# GEDCOM_FILE must be set to the path of the GEDCOM file to parse
ENV GEDCOM_FILE=""

ENTRYPOINT ["/usr/local/bin/family-gedcom"]
