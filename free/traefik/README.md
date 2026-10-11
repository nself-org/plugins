# traefik (nSelf plugin)

Free, experimental. Renders `.nself/generated/routes.json` (`contract:cli.proxy-routes` v1) into Traefik file-provider
configuration and runs Traefik in place of nginx. File provider only: no Docker socket, no ACME resolver; certificates
come from `ssl/` (read-only). Details: `.github/wiki/plugins/Traefik.md`.

```
render --routes /nself/routes.json --out /dynamic --ssl /nself/ssl   # exit 0 ok, 1 refusal or invalid input, 2 I/O
render serve-static --root /acme-webroot --listen :8080
```

Development

```
cd free/traefik && CGO_ENABLED=0 go test -count=1 ./... && go vet ./...
go test ./internal/render -update        # rewrite goldens after an intended change
bash tests/parity.sh                     # Docker, Linux containers: nginx vs Traefik on the shared fixtures
```

`testdata/fixtures` holds routes.json and the nginx tree produced by `nself build` (see `testdata/fixtures/SOURCE`).
