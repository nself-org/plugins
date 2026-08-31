# event-bus plugin

NATS JetStream-backed event bus for nSelf. Provides pub/sub and request/reply
across all plugins and custom services via a common subject namespace.

## Quick start

```bash
nself plugin install event-bus
nself build && nself start
```

The embedded NATS server starts automatically. No external broker needed by default.

## Broker options

Set `EVENT_BUS` to choose your broker:

| Value | Broker | Notes |
|---|---|---|
| `nats` (default) | NATS JetStream | Embedded (`NATS_EMBEDDED=true`) or external |
| `redpanda` | Redpanda | Kafka-protocol compatible |
| `kafka` | Apache Kafka | SASL/PLAIN supported |

## Subject naming

All subjects must begin with `nself.`:

```
nself.<plugin>.<entity>.<operation>
nself.cdc.np_users.insert
nself.claw.memory.update
nself.webhook.delivery.failed
nself.custom.<user-defined>
```

Wildcard subscriptions: `nself.cdc.*` receives all CDC events.

## API

| Method | Path | Description |
|---|---|---|
| `GET` | `/event-bus/status` | Broker type + subject count |
| `GET` | `/event-bus/subjects` | Active subjects with message counts |
| `POST` | `/event-bus/publish` | Test publish (admin only) |
| `GET` | `/event-bus/consumers` | Active consumer list |
| `POST` | `/event-bus/purge/:subject` | Purge subject messages (admin only) |
| `GET` | `/health` | Health check |

## Plugin SDK

```go
import sdk "github.com/nself-org/nself-sdk"

bus, err := sdk.NewEventBus(sdk.EventBusConfig{
    Endpoint: "http://127.0.0.1:8212",
})

// Publish
err = bus.Publish(ctx, sdk.SubjectName("cdc", "np_users", "insert"), payload)

// Subscribe (durable — survives consumer restart)
sub, err := bus.Subscribe("nself.cdc.*", func(msg sdk.EventBusMessage) error {
    // process msg.Payload
    return msg.Ack()
})
```

## Integration with CDC plugin

When both plugins are installed, the CDC plugin automatically publishes all
WAL events to `nself.cdc.<table>.<operation>`. The warehouse plugin subscribes
to `nself.cdc.*` to drive incremental exports.

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `EVENT_BUS` | `nats` | Broker: `nats` / `redpanda` / `kafka` |
| `NATS_EMBEDDED` | `true` | Start embedded NATS (only for `EVENT_BUS=nats`) |
| `NATS_URL` | — | External NATS URL when `NATS_EMBEDDED=false` |
| `REDPANDA_BROKERS` | — | `host:port,...` for Redpanda |
| `KAFKA_BROKERS` | — | `host:port,...` for Kafka |
| `KAFKA_SASL_USERNAME` | — | Kafka SASL username |
| `KAFKA_SASL_PASSWORD` | — | Kafka SASL password |
| `EVENT_BUS_RETENTION_MS` | `86400000` | Stream retention (24 h) |
| `EVENT_BUS_MAX_BYTES` | `1073741824` | Max stream storage (1 GB) |
