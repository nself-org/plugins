# Network-free build (conformance rule build, D-0815): every crate is vendored under vendor/ (.cargo/config.toml
# points cargo at it), the toolchain image carries the C linker, and the runtime stage installs nothing:
# the CA bundle and a static busybox wget (for the compose healthcheck) are copied in.
FROM rust:1.90-bookworm AS builder
WORKDIR /app
COPY Cargo.toml Cargo.lock ./
COPY .cargo ./.cargo
COPY vendor ./vendor
RUN mkdir src && echo 'fn main() {}' > src/main.rs && cargo build --release --locked && rm -rf src
COPY src ./src
RUN touch src/main.rs && cargo build --release --locked

FROM busybox:1.37-uclibc AS busybox

FROM debian:bookworm-slim
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=busybox /bin/busybox /usr/local/bin/wget
RUN useradd --system --no-create-home --user-group nself
COPY --from=builder /app/target/release/file-processing /app/file-processing
USER nself
EXPOSE 3089
ENTRYPOINT ["/app/file-processing"]
