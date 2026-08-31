# crdt — CRDT Offline-First Plugin

Self-hosted CRDT sync server for nSelf. Provides Yjs (y-websocket protocol) and
automerge HTTP sync, backed by Postgres. No extra infra required.

## What It Does

- **Yjs:** Connect standard Yjs clients using the `y-websocket` package. Two clients
  editing the same document converge in real time.
- **automerge:** Send binary sync messages via HTTP. No client library changes needed.
- **Offline-first:** Clients go offline, accumulate changes, reconnect — changes merge
  without conflict errors.
- **Postgres persistence:** Document state is persisted immediately on every update.
  Service restarts are transparent to clients.

## Install

```bash
nself license set nself_pro_...
nself plugin install crdt
nself build
```

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | required | Postgres connection string |
| `CRDT_ENGINE` | `both` | `yjs`, `automerge`, or `both` |
| `CRDT_MAX_DOC_SIZE_MB` | `10` | Max document state size (MB) |
| `CRDT_RETENTION_DAYS` | `90` | Days to keep update history |
| `CRDT_PORT` | `8211` | HTTP/WS listen port |
| `CRDT_REDIS_URL` | — | Optional: Redis for multi-instance awareness |

## API

| Method | Path | Description |
|---|---|---|
| `WS` | `/crdt/yjs/:doc_id` | Yjs y-websocket endpoint |
| `POST` | `/crdt/sync/:doc_id` | automerge binary sync |
| `GET` | `/crdt/doc/:doc_id` | Get document state |
| `DELETE` | `/crdt/doc/:doc_id` | Delete document |
| `GET` | `/crdt/docs` | List documents (admin) |
| `POST` | `/crdt/compact/:doc_id` | Force history compaction (admin) |

## Yjs Client Example

```javascript
import * as Y from 'yjs'
import { WebsocketProvider } from 'y-websocket'

const doc = new Y.Doc()
const provider = new WebsocketProvider(
  'wss://api.example.com',
  'my-document-id',
  doc
)
```

## automerge Client Example

```javascript
import * as Automerge from '@automerge/automerge'

// Send local sync message
const [newDoc, msg] = Automerge.generateSyncMessage(doc, syncState)
if (msg) {
  const res = await fetch('/crdt/sync/my-doc', {
    method: 'POST',
    headers: { 'Content-Type': 'application/octet-stream' },
    body: msg,
  })
  const reply = await res.arrayBuffer()
  // Apply server reply to local doc
}
```

## License

MIT. Requires nSelf Pro license.
