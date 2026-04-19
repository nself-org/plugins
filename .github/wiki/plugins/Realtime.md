# Realtime Plugin

Real-time communication engine with WebSocket rooms, presence tracking, typing indicators, and live messaging via Socket.io and Redis.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [Socket.io Events](#socketio-events)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Features](#features)
- [Troubleshooting](#troubleshooting)

---

## Overview

| Field | Value |
|-------|-------|
| **Version** | 1.0.0 |
| **Category** | infrastructure |
| **Port** | 3101 |
| **License** | Source-Available |
| **Min nself Version** | 0.4.8 |
| **Multi-App** | Yes (`source_account_id`) |

The Realtime plugin provides WebSocket-based real-time communication using Socket.io backed by a Redis adapter for horizontal scaling. It supports room-based messaging, presence tracking, typing indicators, and event persistence to PostgreSQL.

---

## Quick Start

```bash
# Install and initialize
nself plugin install realtime
nself plugin realtime init

# Start the server
nself plugin realtime server

# Check stats
nself plugin realtime stats
```

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |
| `REALTIME_REDIS_URL` | Redis connection URL for Socket.io adapter |
| `REALTIME_CORS_ORIGIN` | Allowed CORS origins |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `REALTIME_PORT` | `3101` | Server port |
| `REALTIME_MAX_CONNECTIONS` | `10000` | Maximum concurrent connections |
| `REALTIME_PING_TIMEOUT` | `20000` | Socket.io ping timeout (ms) |
| `REALTIME_PING_INTERVAL` | `25000` | Socket.io ping interval (ms) |
| `REALTIME_JWT_SECRET` | - | JWT secret for authentication |
| `REALTIME_ENABLE_PRESENCE` | `true` | Enable presence tracking |
| `REALTIME_ENABLE_TYPING` | `true` | Enable typing indicators |
| `REALTIME_ENABLE_COMPRESSION` | `false` | Enable per-message compression |
| `REALTIME_BATCH_SIZE` | `100` | Event batch processing size |
| `REALTIME_RATE_LIMIT` | `50` | Messages per second per connection |
| `REALTIME_LOG_LEVEL` | `info` | Logging level |
| `REALTIME_ENABLE_METRICS` | `false` | Enable metrics endpoint |
| `REALTIME_ENABLE_HEALTH_CHECK` | `true` | Enable health check endpoint |

---

## CLI Commands

| Command | Description | Options |
|---------|-------------|---------|
| `init` | Initialize database schema | - |
| `server` | Start the Socket.io + HTTP server | `-p, --port`, `-h, --host` |
| `stats` | Show server statistics | - |
| `rooms` | List all active rooms | - |
| `create-room` | Create a new room | `-t, --type <type>`, `-v, --visibility <vis>` |
| `connections` | Show active connections | - |
| `events` | Show recent events | `-n, --number <count>` |

---

## Socket.io Events

### Client-to-Server Events

| Event | Payload | Description |
|-------|---------|-------------|
| `room:join` | `{ room_id }` | Join a room |
| `room:leave` | `{ room_id }` | Leave a room |
| `message:send` | `{ room_id, content, type?, metadata? }` | Send a message to a room |
| `typing:start` | `{ room_id }` | Broadcast typing started |
| `typing:stop` | `{ room_id }` | Broadcast typing stopped |
| `presence:update` | `{ status, custom_data? }` | Update user presence |
| `ping` | - | Client heartbeat |

### Server-to-Client Events

| Event | Payload | Description |
|-------|---------|-------------|
| `room:joined` | `{ room, members }` | Confirmation of room join |
| `room:left` | `{ room_id }` | Confirmation of room leave |
| `room:member_joined` | `{ user_id, room_id }` | Another user joined |
| `room:member_left` | `{ user_id, room_id }` | Another user left |
| `message:received` | `{ id, room_id, sender_id, content, ... }` | New message in room |
| `typing:started` | `{ user_id, room_id }` | User started typing |
| `typing:stopped` | `{ user_id, room_id }` | User stopped typing |
| `presence:updated` | `{ user_id, status, ... }` | Presence update |
| `pong` | - | Server heartbeat response |

---

## REST API

The Realtime plugin provides HTTP endpoints via Fastify alongside Socket.io.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/metrics` | Server metrics (connections, rooms, memory) |

Most interaction is through Socket.io events rather than REST endpoints.

---

## Database Schema

### `realtime_connections`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `user_id` | VARCHAR(255) | Connected user ID |
| `socket_id` | VARCHAR(255) | Socket.io socket ID |
| `device_info` | JSONB | Device metadata |
| `connected_at` | TIMESTAMPTZ | Connection start time |
| `disconnected_at` | TIMESTAMPTZ | Disconnection time |
| `last_activity_at` | TIMESTAMPTZ | Last activity timestamp |

### `realtime_rooms`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `name` | VARCHAR(255) | Room name |
| `type` | VARCHAR(32) | Room type (direct, group, channel) |
| `visibility` | VARCHAR(32) | public / private |
| `max_members` | INTEGER | Maximum allowed members |
| `metadata` | JSONB | Custom metadata |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Last update |

### `realtime_room_members`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `room_id` | UUID | FK to realtime_rooms |
| `user_id` | VARCHAR(255) | Member user ID |
| `role` | VARCHAR(32) | member / moderator / admin |
| `joined_at` | TIMESTAMPTZ | Join timestamp |
| `left_at` | TIMESTAMPTZ | Leave timestamp |

### `realtime_presence`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `user_id` | VARCHAR(255) | User ID (unique per account) |
| `status` | VARCHAR(32) | online / away / dnd / offline |
| `custom_status` | VARCHAR(255) | Custom status text |
| `custom_data` | JSONB | Additional presence data |
| `last_seen_at` | TIMESTAMPTZ | Last seen timestamp |
| `updated_at` | TIMESTAMPTZ | Last update |

### `realtime_typing`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `room_id` | UUID | FK to realtime_rooms |
| `user_id` | VARCHAR(255) | Typing user |
| `started_at` | TIMESTAMPTZ | Typing start |
| `expires_at` | TIMESTAMPTZ | Auto-expire timestamp |

### `realtime_events`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `event_type` | VARCHAR(128) | Event type |
| `room_id` | UUID | Associated room |
| `user_id` | VARCHAR(255) | Acting user |
| `data` | JSONB | Event payload |
| `created_at` | TIMESTAMPTZ | Event timestamp |

---

## Features

- **Socket.io with Redis adapter** for horizontal scaling across multiple server instances
- **Room-based messaging** with direct, group, and channel room types
- **Presence tracking** with online/away/dnd/offline states and custom data
- **Typing indicators** with auto-expiration
- **JWT authentication** middleware for socket connections
- **Event persistence** to PostgreSQL for audit and history
- **Rate limiting** per connection with configurable messages/second
- **Per-message compression** support for bandwidth optimization
- **Multi-app isolation** via `source_account_id` on all tables
- **Batch event processing** for high-throughput scenarios
- **Metrics endpoint** for monitoring connection counts and memory usage

---

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Connection refused | Verify `REALTIME_REDIS_URL` is correct and Redis is running |
| CORS errors | Ensure `REALTIME_CORS_ORIGIN` includes your client domain |
| JWT authentication fails | Check `REALTIME_JWT_SECRET` matches your auth service |
| High memory usage | Reduce `REALTIME_MAX_CONNECTIONS` or enable compression |
| Messages not delivered cross-instance | Verify Redis adapter is configured correctly |
| Typing indicators stuck | Check `REALTIME_ENABLE_TYPING` is `true` and client sends `typing:stop` |
