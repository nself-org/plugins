FROM rust:1.76-alpine AS builder
RUN apk add --no-cache musl-dev pkgconfig openssl-dev
WORKDIR /app
COPY Cargo.toml ./
RUN mkdir src && echo 'fn main() {}' > src/main.rs && cargo build --release && rm -rf src
COPY src ./src
RUN touch src/main.rs && cargo build --release

FROM alpine:3.19
RUN apk add --no-cache ffmpeg imagemagick ca-certificates && addgroup -S nself && adduser -S nself -G nself
COPY --from=builder /app/target/release/file-processing /app/file-processing
USER nself
EXPOSE 3089
ENTRYPOINT ["/app/file-processing"]
