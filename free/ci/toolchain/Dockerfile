# Pinned multi-arch toolchain image of `nself ci --container` (P7-CI-43, ADR 0030).
# Source of truth: pins/*.yaml. Every ARG below repeats a pin; the drift test in
# internal/toolchain fails when a version, URL, sha256 or digest here differs from the pins.
# Every FROM is pinned by digest and every download is verified by fetch-verified.sh.
# Build (no push): docker buildx build --platform linux/amd64,linux/arm64 free/ci/toolchain

FROM --platform=$BUILDPLATFORM docker.io/library/debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f AS fetch
ARG BUILDARCH
ARG TARGETARCH
ARG GO_VERSION=1.27.2
ARG GO_URL_AMD64=https://go.dev/dl/go1.27.2.linux-amd64.tar.gz
ARG GO_SHA256_AMD64=ecbadb99091a3f46e31f5f934b068b1864eafa7995211b39eaddf76996045fe5
ARG GO_URL_ARM64=https://go.dev/dl/go1.27.2.linux-arm64.tar.gz
ARG GO_SHA256_ARM64=94f3e30b8e374bc285e7dadc11e0865726b9bc6e85b841ccceaabc0214c6b7c8
ARG NODE_VERSION=24.22.0
ARG NODE_URL_AMD64=https://nodejs.org/dist/v24.22.0/node-v24.22.0-linux-x64.tar.xz
ARG NODE_SHA256_AMD64=cc2f601cee82d9a5a233302fe307abc600727035fd7d2c3a3571755989e95008
ARG NODE_URL_ARM64=https://nodejs.org/dist/v24.22.0/node-v24.22.0-linux-arm64.tar.xz
ARG NODE_SHA256_ARM64=fef00bb4d36e1c50104a9775a10359beee98f162b4104dd12472c75065c6a143
ARG PNPM_VERSION=12.14.0
ARG PNPM_URL_AMD64=https://github.com/pnpm/pnpm/releases/download/v12.14.0/pnpm-linux-x64.tar.gz
ARG PNPM_SHA256_AMD64=3910c3852136bed5bc1511b9137a45fd4357ed6b3970d9d08893641e9005867e
ARG PNPM_URL_ARM64=https://github.com/pnpm/pnpm/releases/download/v12.14.0/pnpm-linux-arm64.tar.gz
ARG PNPM_SHA256_ARM64=cfc780b9e690549e6633e1045fb1fb6ef84e390711e2099e6ccbae91c32456ea
ARG RUST_VERSION=1.99.1
ARG RUST_URL_AMD64=https://static.rust-lang.org/dist/rust-1.99.1-x86_64-unknown-linux-gnu.tar.xz
ARG RUST_SHA256_AMD64=231fcae47ee950621374725b5fc3b5441d2fbda2955ac45dd7dc816d8876cfe7
ARG RUST_URL_ARM64=https://static.rust-lang.org/dist/rust-1.99.1-aarch64-unknown-linux-gnu.tar.xz
ARG RUST_SHA256_ARM64=a48fb1e853be56ce9b8e2751f5464dbd5e16440cd81e237920f3a5e7e806333e
ARG GITLEAKS_VERSION=8.30.1
ARG GITLEAKS_URL_AMD64=https://github.com/gitleaks/gitleaks/releases/download/v8.30.1/gitleaks_8.30.1_linux_x64.tar.gz
ARG GITLEAKS_SHA256_AMD64=551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb
ARG GITLEAKS_URL_ARM64=https://github.com/gitleaks/gitleaks/releases/download/v8.30.1/gitleaks_8.30.1_linux_arm64.tar.gz
ARG GITLEAKS_SHA256_ARM64=e4a487ee7ccd7d3a7f7ec08657610aa3606637dab924210b3aee62570fb4b080
ARG HADOLINT_VERSION=2.15.1
ARG HADOLINT_URL_AMD64=https://github.com/hadolint/hadolint/releases/download/v2.15.1/hadolint-linux-x86_64
ARG HADOLINT_SHA256_AMD64=c7187db94eeeeca956519a6af171adc31453941a1e777961f6e680f697c8c507
ARG HADOLINT_URL_ARM64=https://github.com/hadolint/hadolint/releases/download/v2.15.1/hadolint-linux-arm64
ARG HADOLINT_SHA256_ARM64=f6198ef8090f404dbb771abfee086eb8c48ac177f30da7fd3510aca35b344b5d
ARG TRIVY_VERSION=0.76.0
ARG TRIVY_URL_AMD64=https://github.com/aquasecurity/trivy/releases/download/v0.76.0/trivy_0.76.0_Linux-64bit.tar.gz
ARG TRIVY_SHA256_AMD64=e24b1c51293c1b15e4ba8d45bbc7891ba053f90278ec2698f3562c993e145fe7
ARG TRIVY_URL_ARM64=https://github.com/aquasecurity/trivy/releases/download/v0.76.0/trivy_0.76.0_Linux-ARM64.tar.gz
ARG TRIVY_SHA256_ARM64=aab21e677a7b7071fc79f1711480840754c18a1b9d9cefa7ccd06af63a0e8f4c
ARG SYFT_VERSION=1.54.1
ARG SYFT_URL_AMD64=https://github.com/anchore/syft/releases/download/v1.54.1/syft_1.54.1_linux_amd64.tar.gz
ARG SYFT_SHA256_AMD64=c069905b391cc4c20a5ba65ad5c10be2a7ba074f8ea6ad203e24d14e303dad47
ARG SYFT_URL_ARM64=https://github.com/anchore/syft/releases/download/v1.54.1/syft_1.54.1_linux_arm64.tar.gz
ARG SYFT_SHA256_ARM64=dfdf0537610113edbefe1f1fc6548bc957b2d77439636ec824fcf0e10d46d054
ARG SYFT_URL_AMD64=https://github.com/anchore/syft/releases/download/v1.54.1/syft_1.54.1_darwin_amd64.tar.gz
ARG SYFT_SHA256_AMD64=2956322838b2f64e470eea474495f0cd96be4f222b1ac037258cdc47d965064e
ARG SYFT_URL_ARM64=https://github.com/anchore/syft/releases/download/v1.54.1/syft_1.54.1_darwin_arm64.tar.gz
ARG SYFT_SHA256_ARM64=b4319c3abaa87a0170ab76ee83ea2260ca34b53aecfa3ab0dd5428d2319d744f
ARG GOVULNCHECK_VERSION=1.9.0
ARG GOVULNCHECK_URL_ANY=https://proxy.golang.org/golang.org/x/vuln/@v/v1.9.0.zip
ARG GOVULNCHECK_SHA256_ANY=e4bcdca4303ab3e57e913d0b086bbf7916b97ed4d0b069c8e0eede39e8782c2c
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
# hadolint ignore=DL3008
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl unzip xz-utils \
 && rm -rf /var/lib/apt/lists/*
COPY fetch-verified.sh /usr/local/bin/fetch-verified.sh
RUN mkdir -p /out/usr/local/bin /out/usr/local/lib/pnpm /tmp/dl

# Go for the target platform, plus a Go for the build platform to compile govulncheck.
RUN fetch-verified.sh go "$TARGETARCH" /tmp/dl/go.tgz \
 && tar -C /out/usr/local -xzf /tmp/dl/go.tgz
RUN if [ "$BUILDARCH" = "$TARGETARCH" ]; then ln -s /out/usr/local/go /opt/go-build; \
 else fetch-verified.sh go "$BUILDARCH" /tmp/dl/gob.tgz && mkdir -p /opt/gob && tar -C /opt/gob -xzf /tmp/dl/gob.tgz && ln -s /opt/gob/go /opt/go-build; fi

RUN fetch-verified.sh node "$TARGETARCH" /tmp/dl/node.txz \
 && tar -C /out/usr/local --strip-components=1 -xJf /tmp/dl/node.txz
RUN fetch-verified.sh pnpm "$TARGETARCH" /tmp/dl/pnpm.tgz \
 && tar -C /out/usr/local/lib/pnpm -xzf /tmp/dl/pnpm.tgz

RUN fetch-verified.sh rust "$TARGETARCH" /tmp/dl/rust.txz \
 && mkdir /tmp/rust && tar -C /tmp/rust --strip-components=1 -xJf /tmp/dl/rust.txz \
 && case "$TARGETARCH" in amd64) triple=x86_64-unknown-linux-gnu;; arm64) triple=aarch64-unknown-linux-gnu;; *) exit 1;; esac \
 && /tmp/rust/install.sh --prefix=/out/usr/local/rust --disable-ldconfig \
      --components="rustc,cargo,rust-std-$triple,rustfmt-preview,clippy-preview"

RUN fetch-verified.sh gitleaks "$TARGETARCH" /tmp/dl/gitleaks.tgz \
 && tar -C /out/usr/local/bin -xzf /tmp/dl/gitleaks.tgz gitleaks
RUN fetch-verified.sh trivy "$TARGETARCH" /tmp/dl/trivy.tgz \
 && tar -C /out/usr/local/bin -xzf /tmp/dl/trivy.tgz trivy
RUN fetch-verified.sh syft "$TARGETARCH" /tmp/dl/syft.tgz \
 && tar -C /out/usr/local/bin -xzf /tmp/dl/syft.tgz syft
RUN fetch-verified.sh hadolint "$TARGETARCH" /out/usr/local/bin/hadolint \
 && chmod 0755 /out/usr/local/bin/hadolint

# govulncheck: the pinned module zip, built with the build-platform Go for the target platform.
# Its dependencies are checked against the module's own go.sum.
RUN fetch-verified.sh govulncheck any /tmp/dl/vuln.zip \
 && unzip -q /tmp/dl/vuln.zip -d /tmp/vuln \
 && cd "/tmp/vuln/golang.org/x/vuln@v$GOVULNCHECK_VERSION" \
 && GOROOT=/opt/go-build CGO_ENABLED=0 GOOS=linux GOARCH="$TARGETARCH" GOFLAGS=-trimpath GOTOOLCHAIN=local \
    /opt/go-build/bin/go build -o /out/usr/local/bin/govulncheck ./cmd/govulncheck

FROM docker.io/library/debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f AS toolchain
LABEL org.opencontainers.image.source="https://github.com/nself-org/plugins" \
      org.opencontainers.image.description="Pinned toolchain of nself ci --container" \
      org.opencontainers.image.licenses="MIT"
# hadolint ignore=DL3008
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates git gcc libc6-dev \
 && rm -rf /var/lib/apt/lists/*
COPY --from=fetch /out/usr/local/ /usr/local/
ENV PATH=/usr/local/go/bin:/usr/local/rust/bin:/usr/local/lib/pnpm:$PATH
RUN go version && node --version && npm --version && pnpm --version \
 && cargo --version && cargo clippy --version && rustfmt --version \
 && gitleaks version && govulncheck -version && hadolint --version && trivy --version && syft version
WORKDIR /work
