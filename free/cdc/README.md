# cdc — Change Data Capture Plugin

Streams Postgres WAL change events (INSERT/UPDATE/DELETE) to Kafka, Redpanda, or RabbitMQ
using a Debezium-compatible envelope format.

## Quick Start

```bash
nself license set nself_pro_...
nself plugin install cdc
```

Set required environment variables:

```env
CDC_BROKER=kafka
CDC_BROKER_URLS=kafka:9092
CDC_TOPIC_PREFIX=nself
CDC_SLOT_NAME=nself_cdc
CDC_PUBLICATION_NAME=nself_pub
CDC_BATCH_SIZE=100
CDC_FLUSH_MS=50
```

## Broker Support

| Broker     | `CDC_BROKER` value | Protocol   |
|------------|-------------------|------------|
| Apache Kafka | `kafka`          | Kafka      |
| Redpanda     | `redpanda`       | Kafka      |
| RabbitMQ     | `rabbitmq`       | AMQP 0.9.1 |

## Topic / Routing Key Format

`<CDC_TOPIC_PREFIX>.<table_name>.<operation>`

Examples: `nself.np_users.insert`, `nself.np_orders.delete`

## Event Envelope (Debezium format)

```json
{
  "op": "c",
  "before": null,
  "after": { "id": "1", "email": "alice@example.com" },
  "ts_ms": 1714000000000,
  "source": { "table": "np_users", "lsn": "0/1A2B3C" }
}
```

| `op` | Meaning |
|------|---------|
| `c`  | INSERT  |
| `u`  | UPDATE  |
| `d`  | DELETE  |

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/cdc/status` | Slot lag, events/s, broker state |
| GET | `/cdc/topics` | Active topics list |
| POST | `/cdc/snapshot?table=np_users` | Full-table initial snapshot |
| POST | `/cdc/pause` | Pause streaming (slot stays open) |
| POST | `/cdc/resume` | Resume streaming + drain buffer |
| DELETE | `/cdc/slot?confirm=true` | Drop replication slot (DANGER) |

## Back-pressure

When the broker is unavailable, events are written to `np_cdc_events` with `brokered=false`.
On reconnect the buffer is drained before live CDC resumes.

Hard stop: WAL reader pauses automatically when unbrokered buffer exceeds 100 000 rows.

## Uninstall

`nself plugin uninstall cdc` drops the replication slot cleanly.

## Documentation

Full guide: `nself.org/docs/plugins/cdc`
