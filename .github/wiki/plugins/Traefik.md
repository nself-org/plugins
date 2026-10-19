# Traefik Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.7 |
| **Install** | `nself plugin install traefik` |
| **Requirements** | nSelf >=1.4.12 |

<!-- END GENERATED:catalog -->

> Swaps the nSelf reverse proxy from nginx to Traefik with route, TLS and upstream parity. **Free.**

## Install

```bash
nself add traefik
```

No license key. `nself remove traefik` restores the nginx setup.

## What it does

`nself build` always writes `.nself/generated/routes.json` (the provider-neutral route model, `contract:cli.proxy-routes` v1). With this plugin installed the stack runs three services instead of `nginx`:

| Service | Role |
|---|---|
| `traefik-config` | One-shot Go renderer. Reads `routes.json` and `ssl/` (read-only) and writes `.nself/generated/traefik/dynamic.yml` and the static configuration `.nself/generated/traefik-static/traefik.yml` (the embedded `config/traefik.yml`, plus the `upload` entrypoint when routes are named) atomically, each with a `.nself-generated` marker. It runs as `0:0` (the one documented exception: a one-shot writer into bind directories that Docker creates root-owned on Linux; read-only root, no capabilities, no listener). |
| `traefik-acme` | Same binary in `serve-static` mode. Serves the ACME HTTP-01 webroot (GET and HEAD, one bare token, no listing, no symlinks) and answers the fixed statuses the rendered routes point at. The image runs as the distroless non-root user `65532:65532`, so the webroot must be readable by others (directories 0755, files 0644). |
| `traefik` | The official image, pinned by tag and digest (`images.d/traefik.json`). File provider only, entrypoints `web` :80 and `websecure` :443 (and `upload` :8444 when routes are named). Reads the rendered static configuration from `.nself/generated/traefik-static` (read-only at `/etc/traefik`); read-only root with a tmpfs on `/tmp` for request bodies over 1 MiB. Hardening: runs as the operator uid (`user: NSELF_HOST_UID:NSELF_HOST_GID`, default 0:0 until the core exports them), `cap_drop: [ALL]`, `cap_add: [NET_BIND_SERVICE]`, `no-new-privileges`, no Docker socket. Core writes `ssl/**/privkey.pem` as 0600 owned by the operator uid, so a root process with no capabilities cannot open it on Linux (TLS failed with permission denied); running as the key owner reads it with no extra privilege and the key keeps its mode and owner. `DAC_READ_SEARCH` is deliberately not used: it also allows `open_by_handle_at`, which reads host files outside the container, and a public proxy must not hold it. `NET_BIND_SERVICE` gives a non-root process no effective capability; binding :80/:443 works because the fragment sets the namespaced sysctl `net.ipv4.ip_unprivileged_port_start=0` for this container only. Set `NSELF_HOST_UID` and `NSELF_HOST_GID` to the uid and gid that own `ssl/`; the default is 0:0 (root without capabilities, which reads root-owned keys) until the core exports the operator uid |

### Upload entrypoint

Traefik's `readTimeout` is per entrypoint and is one total deadline (also the only header deadline), so the 60 s of `web` and `websecure` cuts a long upload that nginx lets through. The owner ruling keeps 60 s there. The knob is a separate entrypoint `upload` (container port :8444) whose `readTimeout` is configurable and which carries routers only for the routes you name. Traefik's static file cannot read environment variables, so the renderer renders it (`--static-out`).

| Variable | Default | Meaning |
|---|---|---|
| `TRAEFIK_UPLOAD_ROUTES` | empty (no upload entrypoint) | comma-separated route ids (for example `core:storage`); an id no route has is a refusal (exit 1, nothing written) |
| `TRAEFIK_UPLOAD_READ_TIMEOUT` | `600s` | `readTimeout` of the upload entrypoint, 60 s to 24 h in whole seconds; anything else exits 1 with nothing written |
| `TRAEFIK_UPLOAD_PUBLISH` | `127.0.0.1:` | host side of the :8444 mapping. The default publishes on a loopback ephemeral port, so nothing public opens; set `8444` (or `0.0.0.0:8444`) to expose it |

Each named route is served on `upload` with the same routers, chains, rules and priorities as on its normal listener (the secure listener when the route has one, else the plain one). The default server has no routers on `upload`: an unknown Host there gets Traefik's own 404, and so does a route that was not named. The static configuration is read at start, so changing any of these needs the stack restarted (`nself restart`), not only a re-render. Parity: an 80 s trickle upload to the named route passes on `upload` (120 s), is still cut on :80, and every other route answers 404 on :8444.

## Parity scope

Rendered from the model: routers per (route, location) with nginx longest-prefix and exact-first ordering, security headers, blocked paths (404), `deny_all` (403), method restriction (403), redirects, per-zone rate limits (429) and connection limits (429), body limit (413), gzip, upstream timeouts, TLS protocols and ciphers, the default server (unknown Host on 80 redirects or answers `OK`; on 443 an empty 404 with no upstream reached) and the ACME webroot.

`tests/parity.sh` proves it in Linux containers: the same requests go through nginx (the cli golden confs) and through Traefik (the rendered file) and must return equal status, headers (minus Date and Server), body digest and TLS SAN list. Websocket upgrade is checked on both.

## Refusals

The renderer exits 1, names the route id and field, and writes nothing when the model holds something it cannot express: a route with `unmodelled` or `shadowed_by`, a non-empty `unmodelled_global`, an unknown rate zone key, `limit_req` without `nodelay`, a send timeout, a header value holding an nginx variable, a redirect target other than `https://$host$request_uri`, a certificate lineage that is missing, a rate zone of `0r/s` or a negative burst, body limit or `conn_limit` (Traefik reads zero as no limit), a rate zone keyed on `$http_authorization` or `$http_x_tenant_id` (nginx skips requests without the header, Traefik would share one bucket among them), a second route serving the same server name on the same entrypoint (names compare case-insensitively, and `.x` counts as both `x` and `*.x`), a server name that is not ASCII as written (checked before lowercasing), more than 19999 `blocked_paths` in one route, a `return` status outside 200-599 or one of 204, 205, 304 and 444, a `health_probe` location restricted to a method other than `GET` or `HEAD`, a location path too long to rank below the next priority class (see Host precedence), a server name with an empty label or a last character that is not a letter or digit (`a..example.test`, `a.example.test.`: Traefik ignores a trailing dot, nginx keeps it, so the two names would be one host here), a location that sets more than one of `static_root`, `deny_all`, `return` and an upstream, a `deny_all`, `return` or `static_root` location that sets `health_probe`, `rate_limit`, timeouts or `websocket` (or `methods`, except on the ACME webroot, which keeps its `GET`-only restriction), two route ids that would produce the same router name, a route with neither `http` nor `https`, a wildcard server name with non-default TLS protocols or ciphers (Traefik picks TLS options by SNI from `Host()` rules only), a cipher list that leaves no cipher Go implements, a header name that is not a token or a value holding a control character, or any `routes.json` field the renderer does not model (a new restriction must never render as an unrestricted route, so the plugin is updated together with the contract). The previous `dynamic.yml` stays in place, so a running Traefik keeps serving the last good configuration after a refusal; watch the `traefik-config` exit code.

## No Docker socket, certificates from core

No service mounts `/var/run/docker.sock` or sets `DOCKER_HOST`; there is no Docker provider and no `certificatesResolvers` section. Certificates stay core-issued (ADR 0026): `dynamic.yml` points each `certFile` and `keyFile` at the resolved generation directory of the lineage (`ssl/certificates/.<dir>.gen-<n>/`), so a renewal changes the parsed configuration and the file provider reloads. `ssl/` is mounted read-only in every service. The renderer only resolves the generation link and checks that `fullchain.pem` and `privkey.pem` exist (it never reads key bytes); Traefik reads them itself.

## Known differences from nginx

| Area | nginx | Traefik |
|---|---|---|
| Unknown Host on 443 | closes the connection (`return 444`) | empty 404, no upstream reached (the one accepted equivalence) |
| Rate limit buckets | one bucket per zone and client across all routes | one bucket per router and client; token bucket, parity asserted on status. Limits are counted per router, so a route that is split by name class (exact and wildcard names, or two wildcard suffixes in one route) gets one counter per class, and each class allows the full rate |
| `Vary: Accept-Encoding` | added for every compressible type | added when the answer is compressed |
| DHE ciphers | offered | not implemented by Go; the ECDHE suites remain |
| OCSP stapling | on when the chain is trusted | not provided |
| `X-Powered-By` | hidden on generated routes | not hidden (not in the route model) |
| Upstream https | not verified | not verified (same default) |
| `X-Forwarded-*` sent by clients | appended | dropped unless the source is trusted (Traefik default) |
| Encoded slash in the path (`/a%2Fb`, `/x/..%2f.env`) | decoded and merged before location matching: `/a%2Fb` is served, the second form is a 404 | `400`. The entrypoints set `encodedCharacters.allowEncodedSlash: false`, because Traefik matches the still-encoded path and would let `..%2f.env` skip a blocked path or a `deny_all` prefix. A legitimate `%2F` in a path is the accepted cost. |
| Host precedence | exact server name, then the longest wildcard, then the location | same. Each route is rendered once per class of its names: exact names get the top router priority tier, each wildcard suffix gets a tier that grows with the suffix length, and a location priority never crosses a tier step. A route that mixes plain and wildcard names keeps its exact names above any other route's wildcard. A server name or location path long enough to cross a priority class is refused: an exact path over about 19000 characters, a prefix path over about 8998 (a longer prefix would outrank `blocked_paths`, which nginx tries first), and more than 19999 `blocked_paths` entries |
| `blocked_paths` order | the first matching regex in file order wins | same: each entry gets a priority one lower than the one before it |
| Host with a trailing dot (`x.example.test.`) | stripped, served by the same server | wildcard and leading-dot rules accept an optional trailing dot (`\.?$`) |
| Security headers on the catch-all 404 and the https redirect | added to every answer of the server | added to both (the 404 for a path no location matches and the server-level redirect) |
| Server-level https redirect status | the status of the `return` (301 or 302) | the status of the `/` location that returns it; 307 and 308 are refused because Traefik's redirect cannot send them |
| Server names and case | lowercased, the first of two equal names wins | lowercased; a second route with the same name in another case is refused (see Refusals) |
| A path no location of a server matches | 404 from that server | 404: every route carries a catch-all router in its own tier, so an exact host never falls through to a wildcard route or the default server |
| Non-GET method on the ACME prefix | `limit_except GET` answers 403 for any path, in a route and in the default server | same: a method split on both the per-route and the unknown-Host (`default-acme`) router, so `POST /.well-known/acme-challenge/a/b` is a 403 for a known and an unknown Host (parity-tested) |
| Leading-dot server name (`.example.test`) | the name plus every subdomain | rendered as `Host(example.test)` or a subdomain `HostRegexp`; counts as a wildcard route, and refused with non-default TLS options |
| Request and idle timeouts | `client_header_timeout 60s` closes a request whose headers are incomplete; `client_body_timeout` times the gap between reads; `keepalive_timeout 65s` | the entrypoints set `readTimeout: 60s`, `idleTimeout: 65s`, `writeTimeout: 0s`. Traefik v3 has no separate header timeout, so a finite `readTimeout` is the only slowloris guard (0s would keep a half-sent request open forever). It is a total deadline for headers plus body, where nginx times each gap: an upload that takes longer than 60 s is cut on Traefik and survives on nginx (accepted difference). A websocket loses the deadline once upgraded, so an idle websocket survives (parity: 75 s idle on both; a partial header is closed at about 60 s on both; a 40 s trickle upload passes on both; an 80 s one passes on nginx and is cut on Traefik). The remedy for long uploads is the upload entrypoint (`TRAEFIK_UPLOAD_ROUTES` and `TRAEFIK_UPLOAD_READ_TIMEOUT`, see above), not a longer deadline on `web`/`websecure`. A client that stops reading a response is not timed out on Traefik (`writeTimeout: 0s`); nginx closes it after 60 s (`send_timeout`). A finite total `writeTimeout` would cut large downloads (the same trade as the read deadline), so the slow-reader gap stays an accepted difference |
| `access_log off` on a location | not logged | rendered as `observability.accessLogs: false` on that router |
| Response buffering | streams as its buffers fill | the `buffering` middleware (needed for the 413 body limit) holds the whole upstream response before sending, so SSE, long-poll and large downloads on a proxied location wait for the upstream to finish. Accepted for 413 parity; not tested with a streaming backend |
| `Upgrade` on a location that does not set it | dropped | passed through. Every conf `nself build` writes sets `Upgrade` on all proxied locations (websocket or not), so both proxies upgrade there (checked in the parity websocket cases). Plugin snippets that omit the standard headers differ; the route model cannot tell them apart (cli debt) |
| `health_probe` locations | forces `GET` upstream and drops the request body, for any client method | only `GET` and `HEAD` are proxied (a probe restricted to `HEAD` stays `HEAD`-only: `GET` gets the 403); every other method is a 403 at the edge (the method-restriction router), so no body reaches the upstream. A health probe whose own `methods` list holds anything but `GET` or `HEAD` is refused |
| Body limit on locations that never read a body | `client_max_body_size` is checked against `Content-Length` in the find-config phase, before `return`, `deny` and `limit_except`: an oversized request is a 413 even on a `deny_all` path, a redirect, a blocked path or the ACME webroot | same: the body limit (the location's `max_body_bytes`, else `defaults.max_body_bytes`; none when 0) is the first middleware of every non-proxied router, the method-split 403 routers and the default server's ACME routers included. The check is on the declared `Content-Length` (parity: a large `Content-Length` to `deny_all`, `return`, a blocked path and ACME gives 413 on both). Accepted difference (not parity-tested): a chunked request with no `Content-Length` is read by Traefik up to the limit (413 when it is exceeded), while nginx never reads the body of a location that answers with `return` or `deny`, so it answers 403 or the redirect. The buffering middleware also reads an in-limit body (memory up to 1 MiB, then the tmpfs on `/tmp`, which is RAM) before answering, on every router that has a body limit, the default server included, where nginx never reads the body of a `return` or `deny` location; concurrent anonymous large bodies hold RAM for their duration. Every public hostname's proxied location already had this exposure, so it is not a new class. The tmpfs is left uncapped because a cap would fail legitimate concurrent uploads; an operator can add `size=` in a compose override. The default server's root, health and ACME routers, each route's catch-all 404 (limit: `defaults.max_body_bytes`, the server-level value) and the `http_to_https_redirect` router (limit: the `/` return location's `max_body_bytes`, else the default) lead with the body limit too (parity: an unknown Host over http and https, `POST /health` and `POST /`, a route's redirect, and a path no location matches, all over the limit, give 413 on both) |
| Forwarded `Host` on proxied locations | `proxy_set_header Host $host` sends the name lower-cased with the port stripped | passes the client's Host as sent (case and `:port` kept) when the model sets `Host: $host` or forwarded headers. The router still matches case-insensitively, so routing and access do not differ. Only an upstream that builds URLs from Host can see it |
| Body limit versus rate and connection limit on proxied locations | `client_max_body_size` is checked against `Content-Length` before `limit_req` and `limit_conn`: an oversized request is a 413 even when the zone is exhausted, and it spends no rate budget | the rate and connection limits run first, then the `buffering` middleware. An exhausted zone answers 429 where nginx answers 413, and an oversized request spends a token. Deliberate: buffering reads the whole in-limit body into memory, and the rate and connection limits are what bound how many bodies buffer at once. Buffering first would let a limited client make Traefik buffer bodies without bound. The result fails closed and differs in status only |
| `return` with a fixed status | any code | 200-599 except 204, 205, 304 and 444 (no body, or the connection is closed); other codes are refused |
| TLS options on a wildcard server name | applied | Traefik selects TLS options by SNI from `Host()` rules only, so the renderer refuses a wildcard route with non-default protocols or ciphers |
| `limit_conn` | counted per client address | counted per client address (`sourceCriterion.ipStrategy.depth: 0`); without it Traefik counts per Host for all clients together. The counter belongs to the router, so a route split by name class gets one counter per class (the same split as the rate limit rows above) |

Routes the model does not carry (for example the `api-docs` server nginx generates) are not rendered.

## Files

`free/traefik/`: `cmd/render` (renderer and helper), `internal/render` (model to dynamic.yml), `internal/staticcfg` (static configuration and the upload entrypoint), `internal/static` (webroot and fixed answers), `docker-compose.plugin.yml`, `config/traefik.yml` (hand-written base, embedded by `config/config.go`), `tests/` (fragment policy test, `parity.sh`, `ptool`), `testdata/` (cli fixtures and goldens).
