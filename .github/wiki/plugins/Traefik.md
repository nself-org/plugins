# Traefik Plugin

> Swaps the nSelf reverse proxy from nginx to Traefik with route, TLS and upstream parity. **Free, experimental.**

## Install

```bash
nself add traefik
```

No license key. `nself remove traefik` restores the nginx setup.

## What it does

`nself build` always writes `.nself/generated/routes.json` (the provider-neutral route model, `contract:cli.proxy-routes` v1). With this plugin installed the stack runs three services instead of `nginx`:

| Service | Role |
|---|---|
| `traefik-config` | One-shot Go renderer. Reads `routes.json` and `ssl/` (read-only) and writes `.nself/generated/traefik/dynamic.yml` atomically, plus a `.nself-generated` marker. |
| `traefik-acme` | Same binary in `serve-static` mode. Serves the ACME HTTP-01 webroot (GET and HEAD, one bare token, no listing, no symlinks) and answers the fixed statuses the rendered routes point at. |
| `traefik` | The official image, pinned by tag and digest (`images.d/traefik.json`). File provider only, entrypoints `web` :80 and `websecure` :443. |

## Parity scope

Rendered from the model: routers per (route, location) with nginx longest-prefix and exact-first ordering, security headers, blocked paths (404), `deny_all` (403), method restriction (403), redirects, per-zone rate limits (429) and connection limits (429), body limit (413), gzip, upstream timeouts, TLS protocols and ciphers, the default server (unknown Host on 80 redirects or answers `OK`; on 443 an empty 404 with no upstream reached) and the ACME webroot.

`tests/parity.sh` proves it in Linux containers: the same requests go through nginx (the cli golden confs) and through Traefik (the rendered file) and must return equal status, headers (minus Date and Server), body digest and TLS SAN list. Websocket upgrade is checked on both.

## Refusals

The renderer exits 1, names the route id and field, and writes nothing when the model holds something it cannot express: a route with `unmodelled` or `shadowed_by`, a non-empty `unmodelled_global`, an unknown rate zone key, `limit_req` without `nodelay`, a send timeout, a header value holding an nginx variable, a redirect target other than `https://$host$request_uri`, a certificate lineage that is missing, or any `routes.json` field the renderer does not model (a new restriction must never render as an unrestricted route, so the plugin is updated together with the contract). The previous `dynamic.yml` stays in place, so a running Traefik keeps serving the last good configuration after a refusal; watch the `traefik-config` exit code.

## No Docker socket, certificates from core

No service mounts `/var/run/docker.sock` or sets `DOCKER_HOST`; there is no Docker provider and no `certificatesResolvers` section. Certificates stay core-issued (ADR 0026): `dynamic.yml` points each `certFile` and `keyFile` at the resolved generation directory of the lineage (`ssl/certificates/.<dir>.gen-<n>/`), so a renewal changes the parsed configuration and the file provider reloads. `ssl/` is mounted read-only in every service. The renderer only resolves the generation link and checks that `fullchain.pem` and `privkey.pem` exist (it never reads key bytes); Traefik reads them itself.

## Known differences from nginx

| Area | nginx | Traefik |
|---|---|---|
| Unknown Host on 443 | closes the connection (`return 444`) | empty 404, no upstream reached (the one accepted equivalence) |
| Rate limit buckets | one bucket per zone and client across all routes | one bucket per router and client; token bucket, parity asserted on status |
| `Vary: Accept-Encoding` | added for every compressible type | added when the answer is compressed |
| DHE ciphers | offered | not implemented by Go; the ECDHE suites remain |
| OCSP stapling | on when the chain is trusted | not provided |
| `X-Powered-By` | hidden on generated routes | not hidden (not in the route model) |
| Upstream https | not verified | not verified (same default) |
| `X-Forwarded-*` sent by clients | appended | dropped unless the source is trusted (Traefik default) |

Routes the model does not carry (for example the `api-docs` server nginx generates) are not rendered.

## Files

`free/traefik/`: `cmd/render` (renderer and helper), `internal/render` (model to dynamic.yml), `internal/static` (webroot and fixed answers), `docker-compose.plugin.yml`, `config/traefik.yml`, `tests/` (fragment policy test, `parity.sh`, `ptool`), `testdata/` (cli fixtures and goldens).
