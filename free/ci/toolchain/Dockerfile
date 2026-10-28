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
ARG PYTHON3_VERSION=3.13.16
ARG PYTHON3_URL_AMD64=https://github.com/astral-sh/python-build-standalone/releases/download/20261013/cpython-3.13.16%2B20261013-x86_64-unknown-linux-gnu-install_only.tar.gz
ARG PYTHON3_SHA256_AMD64=e588e50db882fa6a89f926d9a9ad64e445183af0eda5bda226a2c151b17db8a6
ARG PYTHON3_URL_ARM64=https://github.com/astral-sh/python-build-standalone/releases/download/20261013/cpython-3.13.16%2B20261013-aarch64-unknown-linux-gnu-install_only.tar.gz
ARG PYTHON3_SHA256_ARM64=0fce0605da7b6532d5bdaec185336ffaaddd02ec04e27382014952ca4857ed6d
ARG UV_VERSION=0.13.1
ARG UV_URL_AMD64=https://github.com/astral-sh/uv/releases/download/0.13.1/uv-x86_64-unknown-linux-gnu.tar.gz
ARG UV_SHA256_AMD64=eadd9d27c134ab5ac1ffd4473b97a741407f3ea238288f03121fc4deddf90220
ARG UV_URL_ARM64=https://github.com/astral-sh/uv/releases/download/0.13.1/uv-aarch64-unknown-linux-gnu.tar.gz
ARG UV_SHA256_ARM64=9041c73c99fdc5892cf128140beb354eca9231d502068cc36e6cb53a8e59ad14
ARG RUFF_VERSION=0.17.1
ARG RUFF_URL_AMD64=https://github.com/astral-sh/ruff/releases/download/0.17.1/ruff-x86_64-unknown-linux-gnu.tar.gz
ARG RUFF_SHA256_AMD64=a293ac937715ae6fb6b64da86761598379e6149f40fbc00fde9ba8a2a562ac65
ARG RUFF_URL_ARM64=https://github.com/astral-sh/ruff/releases/download/0.17.1/ruff-aarch64-unknown-linux-gnu.tar.gz
ARG RUFF_SHA256_ARM64=d3ead1f0ed779e777a034ccd9286be6bd9bd3cdaba732ff9797f8288a283e9e2
ARG GOVULNCHECK_VERSION=1.9.0
ARG GOVULNCHECK_URL_ANY=https://proxy.golang.org/golang.org/x/vuln/@v/v1.9.0.zip
ARG GOVULNCHECK_SHA256_ANY=e4bcdca4303ab3e57e913d0b086bbf7916b97ed4d0b069c8e0eede39e8782c2c
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
# hadolint ignore=DL3008
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl unzip xz-utils \
 && rm -rf /var/lib/apt/lists/*
COPY fetch-verified.sh /usr/local/bin/fetch-verified.sh
RUN mkdir -p /out/go/usr/local /out/node/usr/local /out/pnpm/usr/local/lib/pnpm /out/rust/usr/local /out/tools/usr/local/bin /out/python/usr/local /out/pytools/usr/local/bin /tmp/dl

# Go for the target platform, plus a Go for the build platform to compile govulncheck.
RUN fetch-verified.sh go "$TARGETARCH" /tmp/dl/go.tgz \
 && tar -C /out/go/usr/local -xzf /tmp/dl/go.tgz
RUN if [ "$BUILDARCH" = "$TARGETARCH" ]; then ln -s /out/go/usr/local/go /opt/go-build; \
 else fetch-verified.sh go "$BUILDARCH" /tmp/dl/gob.tgz && mkdir -p /opt/gob && tar -C /opt/gob -xzf /tmp/dl/gob.tgz && ln -s /opt/gob/go /opt/go-build; fi

RUN fetch-verified.sh node "$TARGETARCH" /tmp/dl/node.txz \
 && tar -C /out/node/usr/local --strip-components=1 -xJf /tmp/dl/node.txz
RUN fetch-verified.sh pnpm "$TARGETARCH" /tmp/dl/pnpm.tgz \
 && tar -C /out/pnpm/usr/local/lib/pnpm -xzf /tmp/dl/pnpm.tgz

RUN fetch-verified.sh rust "$TARGETARCH" /tmp/dl/rust.txz \
 && mkdir /tmp/rust && tar -C /tmp/rust --strip-components=1 -xJf /tmp/dl/rust.txz \
 && case "$TARGETARCH" in amd64) triple=x86_64-unknown-linux-gnu;; arm64) triple=aarch64-unknown-linux-gnu;; *) exit 1;; esac \
 && /tmp/rust/install.sh --prefix=/out/rust/usr/local/rust --disable-ldconfig \
      --components="rustc,cargo,rust-std-$triple,rustfmt-preview,clippy-preview"

RUN fetch-verified.sh python3 "$TARGETARCH" /tmp/dl/python3.tgz \
 && tar -C /out/python/usr/local -xzf /tmp/dl/python3.tgz
RUN fetch-verified.sh uv "$TARGETARCH" /tmp/dl/uv.tgz \
 && tar -C /out/pytools/usr/local/bin --strip-components=1 -xzf /tmp/dl/uv.tgz
RUN fetch-verified.sh ruff "$TARGETARCH" /tmp/dl/ruff.tgz \
 && tar -C /out/pytools/usr/local/bin --strip-components=1 -xzf /tmp/dl/ruff.tgz

RUN fetch-verified.sh gitleaks "$TARGETARCH" /tmp/dl/gitleaks.tgz \
 && tar -C /out/tools/usr/local/bin -xzf /tmp/dl/gitleaks.tgz gitleaks
RUN fetch-verified.sh trivy "$TARGETARCH" /tmp/dl/trivy.tgz \
 && tar -C /out/tools/usr/local/bin -xzf /tmp/dl/trivy.tgz trivy
RUN fetch-verified.sh syft "$TARGETARCH" /tmp/dl/syft.tgz \
 && tar -C /out/tools/usr/local/bin -xzf /tmp/dl/syft.tgz syft
RUN fetch-verified.sh hadolint "$TARGETARCH" /out/tools/usr/local/bin/hadolint \
 && chmod 0755 /out/tools/usr/local/bin/hadolint

# govulncheck: the pinned module zip, served to `go install` from a file GOPROXY so the binary is
# stamped with the pinned version (a build from the unpacked zip reports v0.0.0). The build-platform
# Go compiles it for the target platform. Its dependencies come from the public proxy and are checked
# against the Go checksum database, as is the zip itself. A cross build installs under bin/linux_<arch>.
RUN fetch-verified.sh govulncheck any /tmp/dl/vuln.zip \
 && proxy=/tmp/goproxy/golang.org/x/vuln/@v \
 && mkdir -p "$proxy" \
 && cp /tmp/dl/vuln.zip "$proxy/v$GOVULNCHECK_VERSION.zip" \
 && unzip -p /tmp/dl/vuln.zip "golang.org/x/vuln@v$GOVULNCHECK_VERSION/go.mod" > "$proxy/v$GOVULNCHECK_VERSION.mod" \
 && printf '{"Version":"v%s","Time":"2000-01-01T00:00:00Z"}\n' "$GOVULNCHECK_VERSION" > "$proxy/v$GOVULNCHECK_VERSION.info" \
 && printf 'v%s\n' "$GOVULNCHECK_VERSION" > "$proxy/list" \
 && GOPATH=/tmp/gopath GOROOT=/opt/go-build CGO_ENABLED=0 GOOS=linux GOARCH="$TARGETARCH" GOFLAGS=-trimpath GOTOOLCHAIN=local \
    GOPROXY=file:///tmp/goproxy,https://proxy.golang.org \
    /opt/go-build/bin/go install "golang.org/x/vuln/cmd/govulncheck@v$GOVULNCHECK_VERSION" \
 && install -m 0755 "$(find /tmp/gopath/bin -name govulncheck -type f)" /out/tools/usr/local/bin/govulncheck

FROM docker.io/library/debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f AS toolchain
LABEL org.opencontainers.image.source="https://github.com/nself-org/plugins" \
      org.opencontainers.image.description="Pinned toolchain of nself ci --container" \
      org.opencontainers.image.licenses="MIT"
# hadolint ignore=DL3008
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates git gcc libc6-dev \
 && rm -rf /var/lib/apt/lists/*
# One COPY per stack, so a single pin bump re-pulls one layer, not the whole toolchain.
COPY --from=fetch /out/go/ /
COPY --from=fetch /out/node/ /
COPY --from=fetch /out/pnpm/ /
COPY --from=fetch /out/rust/ /
COPY --from=fetch /out/python/ /
COPY --from=fetch /out/pytools/ /
COPY --from=fetch /out/tools/ /
# The pinned Go never fetches another toolchain (a go.mod `toolchain` line fails loudly instead).
ENV GOTOOLCHAIN=local
# Python projects use the pinned interpreter; uv never downloads another one.
ENV UV_PYTHON_DOWNLOADS=never
ENV PATH=/usr/local/go/bin:/usr/local/rust/bin:/usr/local/lib/pnpm:/usr/local/python/bin:$PATH
RUN go version && node --version && npm --version && pnpm --version \
 && cargo --version && cargo clippy --version && rustfmt --version \
 && python3 --version && uv --version && ruff --version \
 && gitleaks version && govulncheck -version && hadolint --version && trivy --version && syft version
WORKDIR /work
