# mdns

mDNS/Bonjour service discovery for zero-config LAN advertising. Advertises named services on the local network and discovers services broadcast by other hosts, storing all results in PostgreSQL. Useful for nself-tv and other LAN-connected apps that need to find each other without manual IP configuration.

## Installation

```bash
nself plugin install mdns
```

## Features

- Advertise named services on the local network via multicast DNS (mDNS/Bonjour)
- Discover services on the LAN using real multicast DNS queries (PTR, SRV, A/AAAA, TXT records)
- Configurable service type, instance name, and domain (default `_ntv._tcp.local`)
- TXT record support for advertising arbitrary key-value metadata alongside a service
- Discovery log persisted to PostgreSQL — previously seen services remain queryable after a scan
- Filter services by type, advertised state, or availability
- Per-request start/stop of advertising on individual service records
- Multi-app isolation via `source_account_id`
- API key authentication and rate limiting
- Statistics endpoint with counts of total, advertised, active, and discovered services

## Configuration

| Name | Required | Default | Description |
| ---- | -------- | ------- | ----------- |
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `MDNS_PLUGIN_PORT` | No | `3216` | HTTP server port |
| `MDNS_SERVICE_TYPE` | No | `_ntv._tcp` | Default mDNS service type for advertising and discovery |
| `MDNS_INSTANCE_NAME` | No | `nself-server` | Default instance name used when advertising |
| `MDNS_DOMAIN` | No | `local` | mDNS domain |
| `MDNS_API_KEY` | No | — | API key required on all requests (if set) |
| `MDNS_RATE_LIMIT_MAX` | No | `200` | Inbound API rate limit — requests per window |
| `MDNS_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |

### Backend variable mapping

When running under an nself backend `.env.dev`, map variables as follows:

| Backend Variable | Plugin Variable | Description |
| ---------------- | --------------- | ----------- |
| `MDNS_PLUGIN_ENABLED` | — | Enable plugin (backend only) |
| `MDNS_PLUGIN_PORT` | `MDNS_PLUGIN_PORT` | Server port |
| `DATABASE_URL` | `DATABASE_URL` | PostgreSQL connection URL |
| `MDNS_SERVICE_TYPE` | `MDNS_SERVICE_TYPE` | Default mDNS service type |
| `MDNS_INSTANCE_NAME` | `MDNS_INSTANCE_NAME` | Instance name for advertising |
| `MDNS_DOMAIN` | `MDNS_DOMAIN` | mDNS domain |

## API Reference

### Health and Status

#### GET /health

Returns `{ status: "ok", plugin: "mdns", timestamp }`. No authentication required.

#### GET /ready

Returns `{ ready: true }` when the database is reachable. Returns `503` when it is not.

#### GET /live

Returns uptime, memory usage, and a summary of total services, advertised services, and total discovered services.

#### GET /api/stats

Returns full statistics for the current source account.

Response:

```json
{
  "plugin": "mdns",
  "version": "1.0.0",
  "stats": {
    "total_services": 3,
    "advertised_services": 1,
    "active_services": 3,
    "total_discovered": 12,
    "available_discovered": 8,
    "last_discovery_at": "2026-02-21T10:00:00Z"
  },
  "timestamp": "2026-02-21T10:05:00Z"
}
```

### Service Management

Services in this context are entries you register and advertise outbound. Discovery results are separate (see Discovery section).

#### POST /api/services

Registers a new service record. The service is stored but not yet advertised. Call `/api/services/:id/advertise` to start advertising.

Request body:

```json
{
  "service_name": "my-media-server",
  "service_type": "_ntv._tcp",
  "port": 8080,
  "host": "localhost",
  "domain": "local",
  "txt_records": {
    "version": "1.0",
    "path": "/api"
  }
}
```

`service_type` and `domain` default to the values set in configuration. `host` defaults to `localhost`.

Response: the created service record with `id`, `is_advertised: false`, `is_active: true`, and timestamps.

#### GET /api/services

Lists all registered services. Supports filtering by query parameters.

Query parameters:

| Parameter | Description |
| --------- | ----------- |
| `service_type` | Filter to a specific service type |
| `is_advertised` | `true` or `false` — filter by advertising state |
| `is_active` | `true` or `false` — filter by active state |
| `limit` | Maximum records to return (default 200) |
| `offset` | Pagination offset |

Response:

```json
{
  "services": [ { "id": "uuid", "service_name": "my-media-server", "is_advertised": false, ... } ],
  "count": 1
}
```

#### GET /api/services/:id

Returns a single service record by ID. Returns `404` if not found.

#### PUT /api/services/:id

Updates a service record. Accepted fields: `service_name`, `service_type`, `port`, `host`, `domain`, `is_advertised`, `is_active`, `txt_records`, `metadata`.

#### DELETE /api/services/:id

Deletes a service record. Returns `404` if not found.

### Advertising

#### POST /api/services/:id/advertise

Marks a service as actively advertising. Sets `is_advertised: true` on the record. The nself infrastructure layer picks up advertised services and broadcasts them via multicast DNS.

Response:

```json
{ "success": true, "service": { "id": "uuid", "is_advertised": true, ... } }
```

#### POST /api/services/:id/stop

Stops advertising a service. Sets `is_advertised: false`.

Response:

```json
{ "success": true, "service": { "id": "uuid", "is_advertised": false, ... } }
```

### Discovery

#### POST /api/discover

Triggers a live mDNS scan on the local network and stores all discovered services. The scan sends a PTR query and collects responses for the configured timeout period.

Request body:

```json
{
  "service_type": "_ntv._tcp",
  "timeout": 5000,
  "domain": "local"
}
```

`service_type` and `domain` default to configured values. `timeout` is in milliseconds (default 5000).

Response:

```json
{
  "services": [
    {
      "service_type": "_ntv._tcp",
      "service_name": "nself-server._ntv._tcp.local",
      "host": "server.local",
      "port": 3010,
      "addresses": ["192.168.1.100"],
      "txt_records": { "version": "0.9.9" },
      "is_available": true
    }
  ],
  "count": 1,
  "scan_duration_ms": 5012
}
```

Discovered services are upserted into `np_mdns_discovery_log`. Subsequent calls update `last_seen_at` and refresh addresses and TXT records.

#### GET /api/discovered

Returns previously discovered services from the discovery log. Does not trigger a new scan.

Query parameters:

| Parameter | Description |
| --------- | ----------- |
| `service_type` | Filter to a specific service type |
| `is_available` | `true` or `false` — filter by availability state |
| `limit` | Maximum records to return (default 200) |
| `offset` | Pagination offset |

Response:

```json
{
  "discoveries": [
    {
      "id": "uuid",
      "service_type": "_ntv._tcp",
      "service_name": "nself-server._ntv._tcp.local",
      "host": "server.local",
      "port": 3010,
      "addresses": ["192.168.1.100"],
      "txt_records": {},
      "discovered_at": "2026-02-21T09:00:00Z",
      "last_seen_at": "2026-02-21T10:00:00Z",
      "is_available": true
    }
  ],
  "count": 1
}
```

## CLI Commands

```bash
# Initialize database schema
nself-mdns init

# Start the HTTP API server
nself-mdns server --port 3216 --host 0.0.0.0

# Advertise a service on the local network
nself-mdns advertise --name "my-server" --port 8080
nself-mdns advertise --name "my-server" --type "_http._tcp" --port 8080 --host myhost.local

# List all advertised services
nself-mdns services
nself-mdns services --type "_ntv._tcp"
nself-mdns services --advertised-only

# Discover services on the network (reads from DB, does not scan)
nself-mdns discover
nself-mdns discover --type "_http._tcp"

# Show statistics
nself-mdns stats
nself-mdns status
```

## Database Tables

| Table | Purpose |
| ----- | ------- |
| `np_mdns_services` | Services registered by this instance for outbound advertising. Tracks name, type, port, host, domain, TXT records, and advertised state. |
| `np_mdns_discovery_log` | Services discovered on the LAN. Upserted on each scan. Tracks addresses, TXT records, last seen time, and availability. |

Both tables include `source_account_id` for multi-app isolation and unique constraints on `(source_account_id, service_name, service_type)` and `(source_account_id, service_name, service_type, host)` respectively.

## Usage Examples

### Register and advertise a service

```typescript
// Register
const createRes = await fetch('http://localhost:3216/api/services', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    service_name: 'nself-tv',
    service_type: '_ntv._tcp',
    port: 3010,
    txt_records: { version: '1.0' },
  }),
});
const service = await createRes.json();

// Start advertising
await fetch(`http://localhost:3216/api/services/${service.id}/advertise`, {
  method: 'POST',
});
```

### Trigger a discovery scan and retrieve results

```typescript
// Scan the network for 3 seconds
await fetch('http://localhost:3216/api/discover', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ service_type: '_ntv._tcp', timeout: 3000 }),
});

// Read discovered services
const res = await fetch('http://localhost:3216/api/discovered?service_type=_ntv._tcp&is_available=true');
const { discoveries } = await res.json();
```

### Stop advertising a service

```typescript
await fetch(`http://localhost:3216/api/services/${serviceId}/stop`, {
  method: 'POST',
});
```

## Integration

This plugin is used by **nself-tv** to advertise the TV server on the local network and discover other nself-tv instances. Clients on the same LAN can find the server without any manual IP configuration by querying for `_ntv._tcp.local` services.

It integrates alongside the nself auth and Nginx services: services are advertised on their internal ports, and Nginx handles external routing.

## Changelog

### v1.0.0

- Initial release
- Real multicast DNS discovery using the `multicast-dns` package (PTR, SRV, A/AAAA, TXT record parsing)
- Service registry with per-service advertise/stop controls
- Discovery log with upsert on repeat scans
- Filtering by service type, advertised state, and availability
- Statistics endpoint
- Multi-app isolation via `source_account_id`
- API key authentication and rate limiting
