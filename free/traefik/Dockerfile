# traefik plugin renderer: the one binary behind `traefik-config` (render) and `traefik-acme` (serve-static).
# Self-buildable (build context = this directory, vendored module, no network for RUN). Never published here.
FROM golang:1.25-alpine@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -mod=vendor -trimpath -ldflags="-s -w" -o /render ./cmd/render

FROM gcr.io/distroless/static-debian12@sha256:d75cdd72874d4790092fcb1b058493ecf6bb5bf2b2b897045b00ff01d91843f2
COPY --from=build /render /render
ENTRYPOINT ["/render"]
