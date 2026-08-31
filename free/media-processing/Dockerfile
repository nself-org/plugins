FROM rust:1.76-alpine AS builder
RUN apk add --no-cache musl-dev pkgconfig openssl-dev
WORKDIR /app
COPY Cargo.toml ./
RUN mkdir src && echo 'fn main() {}' > src/main.rs && cargo build --release && rm -rf src
COPY src ./src
RUN touch src/main.rs && cargo build --release

FROM alpine:3.19
RUN apk add --no-cache ffmpeg ca-certificates && addgroup -S nself && adduser -S nself -G nself
COPY --from=builder /app/target/release/media-processing /app/media-processing
USER nself
EXPOSE 3088
ENTRYPOINT ["/app/media-processing"]
