# plugin-pty

PTY bridge for ClawDE AI sessions. Spawns, manages, and relays pseudo-terminal processes
with per-tenant resource limits and WebSocket I/O.

**Port:** 9100 | **Bundle:** ClawDE | **License:** Free (MIT)

---

## Overview

plugin-pty manages PTY (pseudo-terminal) process lifecycles for ClawDE sessions. The ClawDE
desktop client spawns a PTY session via REST, then connects a WebSocket for bidirectional
terminal I/O. Sessions are tracked in Postgres and scoped by `source_account_id`.

Security model:
- Max concurrent PTY sessions per tenant enforced (default 5, returns 429 on exceed)
- Cross-tenant session access returns 403 at handler level
- PTY I/O is not logged; only lifecycle events (spawn/close/error) go to the audit log
- No outbound HTTP calls — no SSRF surface

---

## Session Lifecycle

```
POST /sessions                        → spawn PTY, return session_id + ws_url
  │
  ▼
GET /sessions/{id}/ws (WebSocket)     → bidirectional I/O relay
  │
  ▼
DELETE /sessions/{id}                 → close PTY, mark session closed
```

---

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness check |
| POST | `/sessions` | Spawn a PTY session |
| DELETE | `/sessions/{id}` | Close a PTY session |
| GET | `/sessions/{id}/ws` | WebSocket I/O relay (upgrade required) |

### POST /sessions

Request:
```json
{ "client_id": "clawde-daemon-abc", "cols": 120, "rows": 30 }
```

Response (201):
```json
{ "session_id": "uuid", "ws_url": "/sessions/uuid/ws", "status": "active" }
```

Errors:
- `400` — missing client_id
- `429` — max concurrent sessions per tenant exceeded

### GET /sessions/{id}/ws

WebSocket upgrade. Requires `X-Hasura-Source-Account-Id` header matching the session owner.
Sends binary frames from PTY output. Accepts binary frames for PTY input.

---

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `NSELF_DB_URL` | Yes | — | PostgreSQL connection string |
| `NSELF_LICENSE_KEY` | No | — | Unused; reserved for future entitlement checks |
| `PTY_MAX_PER_TENANT` | No | `5` | Max concurrent PTY sessions per tenant |
| `PTY_SESSION_TIMEOUT_SECS` | No | `3600` | Session idle timeout |
| `PTY_PORT` | No | `9100` | HTTP server port |

---

## Database Tables

| Table | Purpose |
|-------|---------|
| `np_pty_sessions` | Session registry (active/closed/expired) |
| `np_pty_audit_log` | Lifecycle events: spawn, close, resize, error |

Both tables include `source_account_id TEXT NOT NULL DEFAULT 'primary'` with Hasura row
filters scoping all queries to the requesting account.

---

## Quickstart

```bash
nself plugin install plugin-pty

# Verify
curl http://localhost:9100/health
# {"status":"ok"}

# Spawn a session (ClawDE does this automatically)
curl -X POST http://localhost:9100/sessions \
  -H "Content-Type: application/json" \
  -H "X-Hasura-Source-Account-Id: my-account" \
  -d '{"client_id":"clawde-1","cols":120,"rows":30}'

# Response: {"session_id":"...","ws_url":"/sessions/.../ws","status":"active"}
# Connect WebSocket to ws_url for I/O relay
```

---

## Security Notes

- PTY processes are isolated per session; no cross-session I/O access
- Cross-tenant session access blocked at handler (403) and Hasura row-filter layers
- Resource limit (429) prevents runaway PTY spawning per tenant
- Audit log records all lifecycle events for compliance
- Free (MIT), no license key required to run
