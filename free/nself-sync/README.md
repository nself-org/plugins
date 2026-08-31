# nself-sync

Event-log sync engine for ɳClaw. Manages multi-device state synchronization via hybrid logical clocks (HLC) and last-write-wins (LWW) conflict resolution.

## Endpoints

- `POST /sync/push` — Accept batched events; dedupe by event_id; persist with HLC timestamp; return acks
- `POST /sync/pull` — Return queued events since cursor (device state machine pulls on interval)
- `GET /sync/subscribe` — WebSocket upgrade for real-time event streaming (pending)
- `GET /sync/snapshot` — Full state snapshot for new device initialization

## Schema

- `np_sync_events` — Event log (user_id, entity_type, entity_id, HLC clock, signature, payload)
- `np_devices` — Device registry (pubkey, platform, last_seen, revoked status)
- `np_sync_cursors` — Per-device cursor (HLC wall + lamport) for pull-based sync

All tables include tenant_id for multi-tenancy. Hasura row-level security enforces user isolation.

## Port

3844 (reserved in PPI port registry)

## Version

1.1.1 (matches ɳClaw bundle)
