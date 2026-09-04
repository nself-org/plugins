# plugin-clawde

ClawDE daemon integration backend for nSelf. Manages session lifecycle, tracks daemon
health, and provides an append-only event log for the ClawDE AI development environment.

**Port:** 3847 | **License:** Free (MIT) | **Bundle:** ClawDE

---

## Session Lifecycle

```
POST /sessions           → create session (status: active)
POST /sessions/:id/heartbeat → keep alive (update last_heartbeat)
POST /sessions/:id/events    → append event to log
DELETE /sessions/:id         → close session (status: closed)
```

Sessions expire if no heartbeat is received within `NSELF_CLAWDE_SESSION_TTL_MINUTES` (default 60).

---

## API

### GET /health

Returns `{"status":"ok"}` (200) or `{"status":"unhealthy"}` (503).

### POST /sessions

```json
{"id": "sess-abc", "source_account_id": "primary"}
```

Returns the created session object (201).

### POST /sessions/:id/heartbeat

Query: `?source_account_id=primary`. Updates `last_heartbeat`. Returns `{"status":"ok"}`.

### POST /sessions/:id/events

```json
{"event_type": "tool_call", "payload": "...", "source_account_id": "primary"}
```

Appends to `np_clawde_events`. Returns `{"status":"recorded"}` (201).

### DELETE /sessions/:id

Query: `?source_account_id=primary`. Sets `status=closed`. Returns `{"status":"closed"}`.

---

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `NSELF_DB_URL` | Yes | — | PostgreSQL connection string |
| `NSELF_LICENSE_KEY` | No | — | Unused; reserved for future entitlement checks |
| `NSELF_CLAWDE_PORT` | No | `3847` | HTTP server port |
| `NSELF_CLAWDE_DAEMON_ADDR` | No | `localhost:3848` | ClawDE PTY bridge address |
| `NSELF_CLAWDE_SESSION_TTL_MINUTES` | No | `60` | Session expiry with no heartbeat |

---

## Database Tables

| Table | Purpose |
|-------|---------|
| `np_clawde_sessions` | Session lifecycle state (active/closed/expired) |
| `np_clawde_events` | Append-only event log per session |

Both use `source_account_id` (Multi-App Isolation Convention).

---

## Installation

```bash
nself plugin install plugin-clawde
```

---

## License

MIT license — no purchase required.
