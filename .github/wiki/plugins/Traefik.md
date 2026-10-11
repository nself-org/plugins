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

The renderer exits 1, names the route id and field, and writes nothing when the model holds something it cannot express: a route with `unmodelled` or `shadowed_by`, a non-empty `unmodelled_global`, an unknown rate zone key, `limit_req` without `nodelay`, a send timeout, a header value holding an nginx variable, a redirect target other than `https://$host$request_uri`, a certificate lineage that is missing, a rate zone of `0r/s` or a negative burst, body limit or `conn_limit` (Traefik reads zero as no limit), a rate zone keyed on `$http_authorization` or `$http_x_tenant_id` (nginx skips requests without the header, Traefik would share one bucket among them), a second route serving the same server name on the same entrypoint (names compare case-insensitively, and `.x` counts as both `x` and `*.x`), a `return` status outside 200-599 or one of 204, 205, 304 and 444, a `health_probe` location restricted to a method other than `GET` or `HEAD`, a location path long enough to rank above the next host tier, two route ids that would produce the same router name, a route with neither `http` nor `https`, a wildcard server name with non-default TLS protocols or ciphers (Traefik picks TLS options by SNI from `Host()` rules only), a cipher list that leaves no cipher Go implements, a header name that is not a token or a value holding a control character, or any `routes.json` field the renderer does not model (a new restriction must never render as an unrestricted route, so the plugin is updated together with the contract). The previous `dynamic.yml` stays in place, so a running Traefik keeps serving the last good configuration after a refusal; watch the `traefik-config` exit code.

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
| Encoded slash in the path (`/a%2Fb`, `/x/..%2f.env`) | decoded and merged before location matching: `/a%2Fb` is served, the second form is a 404 | `400`. The entrypoints set `encodedCharacters.allowEncodedSlash: false`, because Traefik matches the still-encoded path and would let `..%2f.env` skip a blocked path or a `deny_all` prefix. A legitimate `%2F` in a path is the accepted cost. |
| Host precedence | exact server name, then the longest wildcard, then the location | same. Each route is rendered once per class of its names: exact names get the top router priority tier, each wildcard suffix gets a tier that grows with the suffix length, and a location priority never crosses a tier step. A route that mixes plain and wildcard names keeps its exact names above any other route's wildcard. A server name or location path long enough to cross a tier step (an exact path over about 19000 characters, a prefix path over about 48000) is refused |
| Server names and case | lowercased, the first of two equal names wins | lowercased; a second route with the same name in another case is refused (see Refusals) |
| A path no location of a server matches | 404 from that server | 404: every route carries a catch-all router in its own tier, so an exact host never falls through to a wildcard route or the default server |
| Leading-dot server name (`.example.test`) | the name plus every subdomain | rendered as `Host(example.test)` or a subdomain `HostRegexp`; counts as a wildcard route, and refused with non-default TLS options |
| Request and idle timeouts | `client_header_timeout 60s` closes a request whose headers are incomplete; `client_body_timeout` times the gap between reads; `keepalive_timeout 65s` | the entrypoints set `readTimeout: 60s`, `idleTimeout: 65s`, `writeTimeout: 0s`. Traefik v3 has no separate header timeout, so a finite `readTimeout` is the only slowloris guard (0s would keep a half-sent request open forever). It is a total deadline for headers plus body, where nginx times each gap: an upload that takes longer than 60 s is cut on Traefik and survives on nginx (accepted difference). A websocket loses the deadline once upgraded, so an idle websocket survives (parity: 75 s idle on both; a partial header is closed at about 60 s on both; a 40 s trickle upload passes on both; an 80 s one passes on nginx and is cut on Traefik) |
| `access_log off` on a location | not logged | rendered as `observability.accessLogs: false` on that router |
| Response buffering | streams as its buffers fill | the `buffering` middleware (needed for the 413 body limit) holds the whole upstream response before sending, so SSE, long-poll and large downloads on a proxied location wait for the upstream to finish. Accepted for 413 parity; not tested with a streaming backend |
| `Upgrade` on a location that does not set it | dropped | passed through. Every conf `nself build` writes sets `Upgrade` on all proxied locations (websocket or not), so both proxies upgrade there (checked in the parity websocket cases). Plugin snippets that omit the standard headers differ; the route model cannot tell them apart (cli debt) |
| `health_probe` locations | forces `GET` upstream and drops the request body, for any client method | only `GET` and `HEAD` are proxied; every other method is a 403 at the edge (the method-restriction router), so no body reaches the upstream. A health probe whose own `methods` list holds anything but `GET` or `HEAD` is refused |
| `return` with a fixed status | any code | 200-599 except 204, 205, 304 and 444 (no body, or the connection is closed); other codes are refused |
| TLS options on a wildcard server name | applied | Traefik selects TLS options by SNI from `Host()` rules only, so the renderer refuses a wildcard route with non-default protocols or ciphers |
| `limit_conn` | counted per client address | counted per client address (`sourceCriterion.ipStrategy.depth: 0`); without it Traefik counts per Host for all clients together |

Routes the model does not carry (for example the `api-docs` server nginx generates) are not rendered.

## Files

`free/traefik/`: `cmd/render` (renderer and helper), `internal/render` (model to dynamic.yml), `internal/static` (webroot and fixed answers), `docker-compose.plugin.yml`, `config/traefik.yml`, `tests/` (fragment policy test, `parity.sh`, `ptool`), `testdata/` (cli fixtures and goldens).
