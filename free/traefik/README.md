# traefik (nSelf plugin)

Free, experimental. Renders `.nself/generated/routes.json` (`contract:cli.proxy-routes` v1) into Traefik file-provider
configuration and runs Traefik in place of nginx. File provider only: no Docker socket, no ACME resolver; certificates
come from `ssl/` (read-only). Details: `.github/wiki/plugins/Traefik.md`.

```
render --routes /nself/routes.json --out /dynamic --ssl /nself/ssl   # exit 0 ok, 1 refusal or invalid input, 2 I/O
render --routes ... --out /dynamic --static-out /static [--upload-routes ID,ID] [--upload-read-timeout 600s]
render serve-static --root /acme-webroot --listen :8080
```

Upload entrypoint (optional). Traefik's `readTimeout` is a total deadline per entrypoint (60s on web/websecure). To let
named routes take long uploads, set in the stack environment: `TRAEFIK_UPLOAD_ROUTES` (comma-separated route ids, empty =
off), `TRAEFIK_UPLOAD_READ_TIMEOUT` (default `600s`, 60s to 24h) and `TRAEFIK_UPLOAD_PUBLISH` (default `127.0.0.1:`, a
loopback ephemeral port; set `8444` to open it). Container port :8444 carries only the named routes (an unknown Host there
is Traefik's own 404, there is no default server). The static configuration is read at start: restart the stack after
changing them. The image runs as the non-root user 65532. The one-shot `traefik-config` runs as
`NSELF_HOST_UID:NSELF_HOST_GID`, exported by the CLI. On Linux, the CLI must pre-create
`.nself/generated/traefik` and `.nself/generated/traefik-static` as the host operator before starting the
containers. This keeps rendered files owned by the operator so restoring nginx can remove them without sudo.

Development

```
cd free/traefik && CGO_ENABLED=0 go test -count=1 ./... && go vet ./...
go test ./internal/render -update        # rewrite goldens after an intended change
bash tests/parity.sh                     # Docker, Linux containers: nginx vs Traefik on the shared fixtures
```

`testdata/fixtures` holds routes.json and the nginx tree produced by `nself build` (see `testdata/fixtures/SOURCE`).

The static helper serves `core:api-docs` aliases from the read-only `.dist` mount:
`docs.<base>/api-docs` returns `openapi.json`, and `/docs` returns the Scalar page
`scalar.html`. Only these two files are exposed. Location response headers from
`routes.json`, including content type, CORS and cache policy, use headers middleware.
