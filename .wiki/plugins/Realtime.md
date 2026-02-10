# Realtime Plugin

Production-ready Socket.io real-time server with presence tracking, typing indicators, and room management for nself.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Analytics Views](#analytics-views)
- [Performance Considerations](#performance-considerations)
- [Security Notes](#security-notes)
- [Advanced Code Examples](#advanced-code-examples)
- [Monitoring & Alerting](#monitoring--alerting)
- [Use Cases](#use-cases)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Realtime plugin provides a Socket.io-based WebSocket server with full presence tracking, typing indicators, and room management. It supports horizontal scaling via Redis pub/sub and stores all state in PostgreSQL.

- **6 Database Tables** - Connections, rooms, members, presence, typing, events
- **4 Analytics Views** - Active connections, room stats, current typing, presence summary
- **JWT Authentication** - Secure token-based authentication
- **Redis Adapter** - Horizontal scaling across multiple server instances
- **10,000+ Connections** - High-concurrency connection pooling
- **Event Logging** - Comprehensive audit trail for all events

### Key Capabilities

| Capability | Description |
|------------|-------------|
| Presence Tracking | Online/away/busy/offline status with custom statuses |
| Typing Indicators | Real-time typing notifications with auto-expiration |
| Room Management | Channels, DMs, groups, broadcast rooms |
| Metrics | Real-time statistics and performance monitoring |

---

## Quick Start

```bash
# Install the plugin
nself plugin install realtime

# Configure environment
echo "REALTIME_REDIS_URL=redis://localhost:6379" >> .env
echo "REALTIME_CORS_ORIGIN=http://localhost:3000" >> .env
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env

# Initialize database schema
nself plugin realtime init

# Start the server
nself plugin realtime server start
```

### Prerequisites

- PostgreSQL 12+
- Redis 6+
- Node.js 18+
- nself CLI 0.4.8+

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `REALTIME_REDIS_URL` | Yes | - | Redis connection string |
| `REALTIME_CORS_ORIGIN` | Yes | - | Comma-separated allowed CORS origins |
| `REALTIME_PORT` | No | `3101` | Socket.io server port |
| `REALTIME_HOST` | No | `0.0.0.0` | Server bind host |
| `REALTIME_MAX_CONNECTIONS` | No | `10000` | Maximum concurrent connections |
| `REALTIME_JWT_SECRET` | No | - | JWT secret for authentication |
| `REALTIME_ALLOW_ANONYMOUS` | No | `false` | Allow unauthenticated connections |
| `REALTIME_ENABLE_PRESENCE` | No | `true` | Enable presence tracking |
| `REALTIME_ENABLE_TYPING` | No | `true` | Enable typing indicators |
| `REALTIME_TYPING_TIMEOUT` | No | `3000` | Typing indicator timeout (ms) |
| `REALTIME_PRESENCE_HEARTBEAT` | No | `30000` | Presence heartbeat interval (ms) |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |
| `REALTIME_LOG_EVENTS` | No | `true` | Log events to database |
| `REALTIME_LOG_EVENT_TYPES` | No | `connect,disconnect,error` | Event types to log |

### Example .env File

```bash
# Required
REALTIME_REDIS_URL=redis://localhost:6379
REALTIME_CORS_ORIGIN=http://localhost:3000,http://localhost:3001
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Authentication
REALTIME_JWT_SECRET=your-secret-key
REALTIME_ALLOW_ANONYMOUS=false

# Features
REALTIME_ENABLE_PRESENCE=true
REALTIME_ENABLE_TYPING=true
REALTIME_TYPING_TIMEOUT=3000

# Server
REALTIME_PORT=3101
LOG_LEVEL=info
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema and verify configuration
nself plugin realtime init

# Check plugin status
nself plugin realtime status

# View statistics
nself plugin realtime stats
```

### Server Management

```bash
# Start the Socket.io server
nself plugin realtime server start

# Stop the server
nself plugin realtime server stop

# View server logs
nself plugin realtime server logs 100
```

### Room Management

```bash
# List all rooms
nself plugin realtime rooms

# Create a room
nself plugin realtime create-room "my-channel" --type channel --visibility public
```

### Connection Management

```bash
# List active connections
nself plugin realtime connections

# Show recent events
nself plugin realtime events -n 50
```

---

## REST API

The plugin exposes HTTP endpoints alongside the Socket.io server.

### Base URL

```
http://localhost:3101
```

### Endpoints

#### Health & Metrics

```http
GET /health
```
Returns server health status including connection count and uptime.

```http
GET /metrics
```
Returns detailed metrics: connection counts (total, authenticated, anonymous), room counts, presence summary, event totals, and memory/CPU usage.

### Socket.io Events

#### Client-to-Server Events

| Event | Payload | Description |
|-------|---------|-------------|
| `room:join` | `{ roomName: string }` | Join a room |
| `room:leave` | `{ roomName: string }` | Leave a room |
| `message:send` | `{ roomName, content, threadId?, metadata? }` | Send a message to a room |
| `typing:start` | `{ roomName, threadId? }` | Start typing indicator |
| `typing:stop` | `{ roomName, threadId? }` | Stop typing indicator |
| `presence:update` | `{ status, customStatus? }` | Update presence status |
| `ping` | - | Latency check |

#### Server-to-Client Events

| Event | Payload | Description |
|-------|---------|-------------|
| `connected` | `{ socketId, serverTime, protocolVersion }` | Connection established |
| `authenticated` | `{ userId, sessionId, rooms }` | Authentication successful |
| `user:joined` | `{ roomName, userId }` | User joined a room |
| `user:left` | `{ roomName, userId }` | User left a room |
| `message:new` | `{ roomName, userId, content, timestamp }` | New message received |
| `typing:event` | `{ roomName, threadId?, users }` | Typing status changed |
| `presence:changed` | `{ userId, status, customStatus? }` | Presence updated |
| `pong` | `{ timestamp }` | Pong response |
| `error` | `{ code, message, details? }` | Error occurred |

---

## Webhook Events

N/A - internal service. The Realtime plugin does not receive external webhooks. It is an event-driven system using Socket.io for real-time communication between clients and the server.

---

## Database Schema

### realtime_connections

Tracks active WebSocket connections.

```sql
CREATE TABLE realtime_connections (
    id UUID PRIMARY KEY,
    socket_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(255),
    device JSONB,                          -- {type, os, browser}
    connected_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_activity TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    disconnected_at TIMESTAMP WITH TIME ZONE,
    metadata JSONB DEFAULT '{}'
);
```

### realtime_rooms

Chat rooms and channels.

```sql
CREATE TABLE realtime_rooms (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    type VARCHAR(50),                      -- channel, dm, group, broadcast
    visibility VARCHAR(50),                -- public, private
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    metadata JSONB DEFAULT '{}'
);
```

### realtime_room_members

Room membership tracking.

```sql
CREATE TABLE realtime_room_members (
    id UUID PRIMARY KEY,
    room_id UUID REFERENCES realtime_rooms(id),
    user_id VARCHAR(255) NOT NULL,
    joined_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    left_at TIMESTAMP WITH TIME ZONE
);
```

### realtime_presence

User presence status.

```sql
CREATE TABLE realtime_presence (
    user_id VARCHAR(255) PRIMARY KEY,
    status VARCHAR(20) NOT NULL,           -- online, away, busy, offline
    custom_status JSONB,                   -- {text, emoji}
    last_seen TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### realtime_typing

Typing indicator state.

```sql
CREATE TABLE realtime_typing (
    id UUID PRIMARY KEY,
    room_name VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    thread_id VARCHAR(255),
    started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL
);
```

### realtime_events

Event audit log.

```sql
CREATE TABLE realtime_events (
    id UUID PRIMARY KEY,
    type VARCHAR(100) NOT NULL,            -- connect, disconnect, message, error, etc.
    user_id VARCHAR(255),
    room_name VARCHAR(255),
    data JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_realtime_events_type ON realtime_events(type);
CREATE INDEX idx_realtime_events_created ON realtime_events(created_at DESC);
```

---

## Analytics Views

### realtime_active_connections

Active connections with presence information.

```sql
CREATE VIEW realtime_active_connections AS
SELECT
    c.socket_id,
    c.user_id,
    c.device,
    c.connected_at,
    c.last_activity,
    p.status AS presence_status,
    p.custom_status
FROM realtime_connections c
LEFT JOIN realtime_presence p ON c.user_id = p.user_id
WHERE c.disconnected_at IS NULL
ORDER BY c.connected_at DESC;
```

### realtime_room_stats

Room statistics and member counts.

```sql
CREATE VIEW realtime_room_stats AS
SELECT
    r.name,
    r.type,
    r.visibility,
    COUNT(rm.id) AS member_count,
    r.created_at
FROM realtime_rooms r
LEFT JOIN realtime_room_members rm ON r.id = rm.room_id AND rm.left_at IS NULL
GROUP BY r.id, r.name, r.type, r.visibility, r.created_at
ORDER BY member_count DESC;
```

### realtime_current_typing

Non-expired typing indicators.

```sql
CREATE VIEW realtime_current_typing AS
SELECT
    room_name,
    user_id,
    thread_id,
    started_at
FROM realtime_typing
WHERE expires_at > NOW()
ORDER BY room_name, started_at;
```

### realtime_presence_summary

Summary of presence status counts.

```sql
CREATE VIEW realtime_presence_summary AS
SELECT
    status,
    COUNT(*) AS user_count
FROM realtime_presence
WHERE last_seen > NOW() - INTERVAL '1 hour'
GROUP BY status
ORDER BY user_count DESC;
```

---

## Performance Considerations

### Redis Scaling

The Realtime plugin uses Redis for horizontal scaling across multiple server instances. Here's how to optimize Redis for high-traffic scenarios:

#### Connection Pooling

```bash
# Recommended Redis configuration for production
# /etc/redis/redis.conf

# Maximum number of clients
maxclients 10000

# Memory management
maxmemory 2gb
maxmemory-policy allkeys-lru

# Persistence (disable for pure cache)
save ""
appendonly no

# Network optimization
tcp-backlog 511
timeout 300
tcp-keepalive 60
```

#### Redis Cluster Setup

For very high throughput (100,000+ connections), use Redis Cluster:

```bash
# Start Redis Cluster (3 master, 3 replica)
redis-cli --cluster create \
  127.0.0.1:7000 127.0.0.1:7001 127.0.0.1:7002 \
  127.0.0.1:7003 127.0.0.1:7004 127.0.0.1:7005 \
  --cluster-replicas 1

# Configure plugin for cluster
REALTIME_REDIS_URL=redis://127.0.0.1:7000,127.0.0.1:7001,127.0.0.1:7002
```

#### Memory Optimization

Monitor Redis memory usage and optimize:

```bash
# Check Redis memory stats
redis-cli info memory

# Monitor key eviction
redis-cli info stats | grep evicted

# Set memory limits per connection
REALTIME_REDIS_MAX_MEMORY_PER_CONNECTION=1048576  # 1MB
```

### Namespace Optimization

Organize Redis keys by namespace for better performance:

```javascript
// Recommended namespace structure
const namespaces = {
  connections: 'rt:conn:',      // rt:conn:{socketId}
  rooms: 'rt:room:',            // rt:room:{roomName}
  presence: 'rt:pres:',         // rt:pres:{userId}
  typing: 'rt:typ:',            // rt:typ:{roomName}:{threadId}
  events: 'rt:evt:',            // rt:evt:{eventId}
  metrics: 'rt:met:'            // rt:met:{metric}
};

// Example: Set TTL on ephemeral data
// Typing indicators expire automatically
await redis.setex(`rt:typ:${roomName}:${threadId}:${userId}`, 3, 'typing');
```

### Database Connection Pooling

Configure PostgreSQL connection pool for optimal performance:

```bash
# Environment variables for connection pooling
DATABASE_POOL_MIN=10                    # Minimum pool size
DATABASE_POOL_MAX=50                    # Maximum pool size
DATABASE_POOL_IDLE_TIMEOUT=30000        # 30 seconds
DATABASE_POOL_CONNECTION_TIMEOUT=5000   # 5 seconds
DATABASE_STATEMENT_TIMEOUT=10000        # 10 seconds query timeout
```

PostgreSQL configuration for high concurrency:

```sql
-- postgresql.conf optimizations
max_connections = 200
shared_buffers = 256MB
effective_cache_size = 1GB
work_mem = 4MB
maintenance_work_mem = 64MB

-- Connection pooling with PgBouncer (recommended)
# pgbouncer.ini
[databases]
nself = host=localhost port=5432 dbname=nself

[pgbouncer]
pool_mode = transaction
max_client_conn = 1000
default_pool_size = 25
reserve_pool_size = 5
reserve_pool_timeout = 3
```

### Socket.io Performance Tuning

Optimize Socket.io for maximum throughput:

```javascript
// Socket.io server configuration
const io = new Server(server, {
  // Connection settings
  pingTimeout: 20000,           // 20 seconds
  pingInterval: 25000,          // 25 seconds
  maxHttpBufferSize: 1e6,       // 1MB max message size

  // Performance settings
  transports: ['websocket'],     // WebSocket only (faster)
  allowUpgrades: false,          // Disable polling fallback

  // Adapter settings
  adapter: createAdapter(pubClient, subClient, {
    key: 'realtime:',
    requestsTimeout: 5000
  }),

  // Connection limits
  connectTimeout: 10000,         // 10 seconds
  maxConnections: 10000          // Per server instance
});

// Enable compression for large payloads
io.use((socket, next) => {
  socket.compress(true);
  next();
});
```

### Load Balancing

Deploy multiple server instances behind a load balancer:

```nginx
# nginx.conf for WebSocket load balancing
upstream realtime_servers {
    ip_hash;  # Sticky sessions (important for Socket.io)

    server 10.0.1.10:3101 max_fails=3 fail_timeout=30s;
    server 10.0.1.11:3101 max_fails=3 fail_timeout=30s;
    server 10.0.1.12:3101 max_fails=3 fail_timeout=30s;
}

server {
    listen 80;
    server_name realtime.example.com;

    location /socket.io/ {
        proxy_pass http://realtime_servers;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        # Timeouts
        proxy_connect_timeout 7d;
        proxy_send_timeout 7d;
        proxy_read_timeout 7d;
    }
}
```

### Performance Benchmarks

Expected performance under optimal conditions:

| Metric | Single Instance | 3-Instance Cluster |
|--------|----------------|-------------------|
| Concurrent Connections | 10,000 | 30,000 |
| Messages/sec | 50,000 | 150,000 |
| Latency (p50) | 5ms | 8ms |
| Latency (p99) | 50ms | 100ms |
| Memory per Connection | ~10KB | ~12KB |
| CPU Usage (idle) | 5% | 5% per instance |
| CPU Usage (peak) | 80% | 70% per instance |

### Optimization Checklist

- [ ] Redis configured with connection pooling
- [ ] PostgreSQL connection pool tuned
- [ ] Socket.io using WebSocket-only transport
- [ ] Load balancer configured with sticky sessions
- [ ] Compression enabled for large messages
- [ ] Event logging selective (not all events)
- [ ] Presence heartbeat interval optimized
- [ ] Typing indicators have TTL set
- [ ] Database indexes on frequently queried columns
- [ ] Stale connection cleanup scheduled

---

## Security Notes

### JWT Authentication

The Realtime plugin supports JWT-based authentication for secure connections.

#### Generating JWT Tokens

```javascript
// Server-side: Generate JWT token for client
import jwt from 'jsonwebtoken';

function generateRealtimeToken(userId: string, metadata?: object) {
  return jwt.sign(
    {
      userId,
      type: 'realtime',
      ...metadata
    },
    process.env.REALTIME_JWT_SECRET,
    {
      expiresIn: '24h',
      issuer: 'your-app',
      audience: 'realtime-server'
    }
  );
}

// Example usage
const token = generateRealtimeToken('user_123', {
  rooms: ['general', 'announcements'],
  permissions: ['read', 'write']
});
```

#### Client Authentication

```javascript
// Client-side: Connect with JWT token
import { io } from 'socket.io-client';

const socket = io('http://localhost:3101', {
  auth: {
    token: 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...'
  }
});

socket.on('authenticated', (data) => {
  console.log('Authenticated as:', data.userId);
  console.log('Session ID:', data.sessionId);
  console.log('Available rooms:', data.rooms);
});

socket.on('error', (error) => {
  if (error.code === 'AUTHENTICATION_FAILED') {
    console.error('Invalid token:', error.message);
  }
});
```

#### Token Refresh Strategy

```javascript
// Refresh token before expiration
let socket;
let tokenRefreshTimer;

function connectWithAutoRefresh(getToken) {
  // Initial connection
  socket = io('http://localhost:3101', {
    auth: { token: getToken() }
  });

  socket.on('authenticated', (data) => {
    // Schedule token refresh (refresh 5 minutes before expiry)
    const expiresIn = 24 * 60 * 60 * 1000; // 24 hours
    const refreshIn = expiresIn - (5 * 60 * 1000);

    clearTimeout(tokenRefreshTimer);
    tokenRefreshTimer = setTimeout(() => {
      socket.auth.token = getToken();
      socket.disconnect().connect();
    }, refreshIn);
  });

  socket.on('disconnect', () => {
    clearTimeout(tokenRefreshTimer);
  });
}

// Usage
connectWithAutoRefresh(() => fetchTokenFromAPI());
```

### Room Authorization

Implement server-side authorization checks for room access:

```javascript
// Server-side middleware: Check room permissions
io.use(async (socket, next) => {
  const { token } = socket.handshake.auth;

  try {
    const decoded = jwt.verify(token, process.env.REALTIME_JWT_SECRET);
    socket.userId = decoded.userId;
    socket.permissions = decoded.permissions || [];
    socket.allowedRooms = decoded.rooms || [];
    next();
  } catch (error) {
    next(new Error('AUTHENTICATION_FAILED'));
  }
});

// Room join authorization
socket.on('room:join', async ({ roomName }) => {
  // Check if user has access to this room
  const hasAccess = await checkRoomAccess(socket.userId, roomName);

  if (!hasAccess) {
    socket.emit('error', {
      code: 'ROOM_ACCESS_DENIED',
      message: `Access denied to room: ${roomName}`
    });
    return;
  }

  socket.join(roomName);
  socket.emit('room:joined', { roomName });
});

async function checkRoomAccess(userId: string, roomName: string): Promise<boolean> {
  // Query database for room membership
  const result = await db.query(
    `SELECT 1 FROM realtime_room_members rm
     JOIN realtime_rooms r ON r.id = rm.room_id
     WHERE r.name = $1 AND rm.user_id = $2 AND rm.left_at IS NULL`,
    [roomName, userId]
  );

  return result.rows.length > 0;
}
```

### Rate Limiting

Protect against abuse with rate limiting:

```javascript
import { RateLimiterRedis } from 'rate-limiter-flexible';

// Create rate limiter
const rateLimiter = new RateLimiterRedis({
  storeClient: redisClient,
  keyPrefix: 'rl:realtime',
  points: 100,        // Number of points
  duration: 60,       // Per 60 seconds
  blockDuration: 300  // Block for 5 minutes if exceeded
});

// Apply rate limiting to message sending
socket.on('message:send', async (data) => {
  try {
    await rateLimiter.consume(socket.userId || socket.id);

    // Process message
    await handleMessage(socket, data);
  } catch (error) {
    if (error instanceof Error && error.name === 'RateLimiterRes') {
      socket.emit('error', {
        code: 'RATE_LIMIT_EXCEEDED',
        message: 'Too many messages. Please slow down.',
        retryAfter: Math.round(error.msBeforeNext / 1000)
      });
    }
  }
});

// Per-room rate limiting
const roomRateLimiter = new RateLimiterRedis({
  storeClient: redisClient,
  keyPrefix: 'rl:room',
  points: 10,         // 10 messages
  duration: 1,        // Per second per room
});

socket.on('message:send', async ({ roomName, content }) => {
  try {
    // User-level limit
    await rateLimiter.consume(socket.userId);

    // Room-level limit
    await roomRateLimiter.consume(`${roomName}:${socket.userId}`);

    // Broadcast message
    io.to(roomName).emit('message:new', {
      roomName,
      userId: socket.userId,
      content,
      timestamp: new Date()
    });
  } catch (error) {
    socket.emit('error', {
      code: 'RATE_LIMIT_EXCEEDED',
      message: 'Slow down!'
    });
  }
});
```

### Input Validation & Sanitization

```javascript
import { z } from 'zod';
import DOMPurify from 'isomorphic-dompurify';

// Define schemas for event payloads
const MessageSchema = z.object({
  roomName: z.string().min(1).max(255).regex(/^[a-z0-9-_]+$/),
  content: z.string().min(1).max(5000),
  threadId: z.string().optional(),
  metadata: z.record(z.any()).optional()
});

const PresenceSchema = z.object({
  status: z.enum(['online', 'away', 'busy', 'offline']),
  customStatus: z.object({
    text: z.string().max(100).optional(),
    emoji: z.string().max(10).optional()
  }).optional()
});

// Validate and sanitize messages
socket.on('message:send', async (data) => {
  try {
    // Validate schema
    const validated = MessageSchema.parse(data);

    // Sanitize content (remove XSS)
    const sanitized = {
      ...validated,
      content: DOMPurify.sanitize(validated.content, {
        ALLOWED_TAGS: ['b', 'i', 'em', 'strong', 'a'],
        ALLOWED_ATTR: ['href']
      })
    };

    // Process sanitized message
    await handleMessage(socket, sanitized);
  } catch (error) {
    socket.emit('error', {
      code: 'VALIDATION_ERROR',
      message: 'Invalid message format',
      details: error.errors
    });
  }
});
```

### CORS Configuration

```javascript
// Strict CORS configuration
const io = new Server(server, {
  cors: {
    origin: process.env.REALTIME_CORS_ORIGIN.split(','),
    methods: ['GET', 'POST'],
    credentials: true,
    allowedHeaders: ['Authorization', 'Content-Type']
  }
});

// Dynamic CORS validation
io.engine.on('initial_headers', (headers, req) => {
  const origin = req.headers.origin;

  // Validate origin against whitelist
  if (allowedOrigins.includes(origin)) {
    headers['Access-Control-Allow-Origin'] = origin;
  }
});
```

### Security Checklist

- [ ] JWT_SECRET set to strong random value (min 32 chars)
- [ ] CORS origins restricted to known domains
- [ ] Rate limiting enabled on all events
- [ ] Input validation on all client payloads
- [ ] XSS sanitization on user-generated content
- [ ] Room access authorization implemented
- [ ] Anonymous connections disabled in production
- [ ] TLS/SSL enabled for all connections
- [ ] Redis AUTH password set
- [ ] PostgreSQL connections use SSL
- [ ] Event logging excludes sensitive data
- [ ] Regular security audits scheduled

---

## Advanced Code Examples

### Socket.io Client Examples

#### React Hook

```typescript
// useRealtime.ts - React Hook for Socket.io
import { useEffect, useRef, useState } from 'react';
import { io, Socket } from 'socket.io-client';

interface UseRealtimeOptions {
  url: string;
  token: string;
  autoConnect?: boolean;
}

interface RealtimeState {
  connected: boolean;
  authenticated: boolean;
  error: string | null;
}

export function useRealtime({ url, token, autoConnect = true }: UseRealtimeOptions) {
  const socketRef = useRef<Socket | null>(null);
  const [state, setState] = useState<RealtimeState>({
    connected: false,
    authenticated: false,
    error: null
  });

  useEffect(() => {
    if (!autoConnect) return;

    // Initialize socket
    const socket = io(url, {
      auth: { token },
      autoConnect: true
    });

    socketRef.current = socket;

    // Connection events
    socket.on('connect', () => {
      setState(prev => ({ ...prev, connected: true, error: null }));
    });

    socket.on('disconnect', () => {
      setState(prev => ({ ...prev, connected: false, authenticated: false }));
    });

    socket.on('authenticated', (data) => {
      setState(prev => ({ ...prev, authenticated: true }));
      console.log('Authenticated as:', data.userId);
    });

    socket.on('error', (error) => {
      setState(prev => ({ ...prev, error: error.message }));
    });

    // Cleanup
    return () => {
      socket.disconnect();
    };
  }, [url, token, autoConnect]);

  // Helper functions
  const joinRoom = (roomName: string) => {
    socketRef.current?.emit('room:join', { roomName });
  };

  const leaveRoom = (roomName: string) => {
    socketRef.current?.emit('room:leave', { roomName });
  };

  const sendMessage = (roomName: string, content: string, metadata?: object) => {
    socketRef.current?.emit('message:send', { roomName, content, metadata });
  };

  const updatePresence = (status: string, customStatus?: object) => {
    socketRef.current?.emit('presence:update', { status, customStatus });
  };

  const startTyping = (roomName: string, threadId?: string) => {
    socketRef.current?.emit('typing:start', { roomName, threadId });
  };

  const stopTyping = (roomName: string, threadId?: string) => {
    socketRef.current?.emit('typing:stop', { roomName, threadId });
  };

  const on = (event: string, handler: (...args: any[]) => void) => {
    socketRef.current?.on(event, handler);
  };

  const off = (event: string, handler: (...args: any[]) => void) => {
    socketRef.current?.off(event, handler);
  };

  return {
    socket: socketRef.current,
    ...state,
    joinRoom,
    leaveRoom,
    sendMessage,
    updatePresence,
    startTyping,
    stopTyping,
    on,
    off
  };
}
```

#### React Chat Component

```tsx
// ChatRoom.tsx - Complete chat room implementation
import React, { useEffect, useState } from 'react';
import { useRealtime } from './useRealtime';

interface Message {
  userId: string;
  content: string;
  timestamp: Date;
}

interface ChatRoomProps {
  roomName: string;
  token: string;
}

export function ChatRoom({ roomName, token }: ChatRoomProps) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [inputValue, setInputValue] = useState('');
  const [typingUsers, setTypingUsers] = useState<string[]>([]);

  const realtime = useRealtime({
    url: 'http://localhost:3101',
    token
  });

  useEffect(() => {
    if (!realtime.authenticated) return;

    // Join room
    realtime.joinRoom(roomName);

    // Listen for messages
    const handleMessage = (msg: Message) => {
      setMessages(prev => [...prev, msg]);
    };

    const handleTyping = ({ users }: { users: string[] }) => {
      setTypingUsers(users);
    };

    realtime.on('message:new', handleMessage);
    realtime.on('typing:event', handleTyping);

    return () => {
      realtime.off('message:new', handleMessage);
      realtime.off('typing:event', handleTyping);
      realtime.leaveRoom(roomName);
    };
  }, [realtime.authenticated, roomName]);

  const handleSend = () => {
    if (!inputValue.trim()) return;

    realtime.sendMessage(roomName, inputValue);
    setInputValue('');
    realtime.stopTyping(roomName);
  };

  const handleTyping = () => {
    realtime.startTyping(roomName);

    // Auto-stop after 3 seconds
    setTimeout(() => {
      realtime.stopTyping(roomName);
    }, 3000);
  };

  return (
    <div className="chat-room">
      <div className="messages">
        {messages.map((msg, i) => (
          <div key={i} className="message">
            <strong>{msg.userId}:</strong> {msg.content}
            <span className="timestamp">
              {new Date(msg.timestamp).toLocaleTimeString()}
            </span>
          </div>
        ))}
      </div>

      {typingUsers.length > 0 && (
        <div className="typing-indicator">
          {typingUsers.join(', ')} {typingUsers.length === 1 ? 'is' : 'are'} typing...
        </div>
      )}

      <div className="input-area">
        <input
          type="text"
          value={inputValue}
          onChange={(e) => {
            setInputValue(e.target.value);
            handleTyping();
          }}
          onKeyPress={(e) => e.key === 'Enter' && handleSend()}
          placeholder="Type a message..."
        />
        <button onClick={handleSend}>Send</button>
      </div>

      <div className="status">
        {realtime.connected ? '🟢 Connected' : '🔴 Disconnected'}
      </div>
    </div>
  );
}
```

#### Vue 3 Composable

```typescript
// useRealtime.ts - Vue 3 Composable
import { ref, onMounted, onUnmounted } from 'vue';
import { io, Socket } from 'socket.io-client';

export function useRealtime(url: string, token: string) {
  const socket = ref<Socket | null>(null);
  const connected = ref(false);
  const authenticated = ref(false);
  const error = ref<string | null>(null);

  onMounted(() => {
    socket.value = io(url, {
      auth: { token }
    });

    socket.value.on('connect', () => {
      connected.value = true;
      error.value = null;
    });

    socket.value.on('disconnect', () => {
      connected.value = false;
      authenticated.value = false;
    });

    socket.value.on('authenticated', () => {
      authenticated.value = true;
    });

    socket.value.on('error', (err) => {
      error.value = err.message;
    });
  });

  onUnmounted(() => {
    socket.value?.disconnect();
  });

  const emit = (event: string, data: any) => {
    socket.value?.emit(event, data);
  };

  const on = (event: string, handler: (...args: any[]) => void) => {
    socket.value?.on(event, handler);
  };

  const off = (event: string, handler: (...args: any[]) => void) => {
    socket.value?.off(event, handler);
  };

  return {
    socket,
    connected,
    authenticated,
    error,
    emit,
    on,
    off
  };
}
```

#### Vanilla JavaScript

```javascript
// vanilla-realtime.js - No framework required
class RealtimeClient {
  constructor(url, token) {
    this.url = url;
    this.token = token;
    this.socket = null;
    this.listeners = new Map();
  }

  connect() {
    this.socket = io(this.url, {
      auth: { token: this.token }
    });

    this.socket.on('connect', () => {
      console.log('Connected to realtime server');
      this.emit('connection-change', { connected: true });
    });

    this.socket.on('disconnect', () => {
      console.log('Disconnected from realtime server');
      this.emit('connection-change', { connected: false });
    });

    this.socket.on('authenticated', (data) => {
      console.log('Authenticated as:', data.userId);
      this.emit('authenticated', data);
    });

    this.socket.on('error', (error) => {
      console.error('Socket error:', error);
      this.emit('error', error);
    });
  }

  disconnect() {
    this.socket?.disconnect();
  }

  joinRoom(roomName) {
    this.socket?.emit('room:join', { roomName });
  }

  leaveRoom(roomName) {
    this.socket?.emit('room:leave', { roomName });
  }

  sendMessage(roomName, content, metadata = {}) {
    this.socket?.emit('message:send', { roomName, content, metadata });
  }

  on(event, handler) {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, []);
      this.socket?.on(event, (...args) => {
        this.listeners.get(event)?.forEach(h => h(...args));
      });
    }
    this.listeners.get(event).push(handler);
  }

  off(event, handler) {
    const handlers = this.listeners.get(event);
    if (handlers) {
      const index = handlers.indexOf(handler);
      if (index > -1) {
        handlers.splice(index, 1);
      }
    }
  }

  emit(event, data) {
    const handlers = this.listeners.get(event) || [];
    handlers.forEach(handler => handler(data));
  }
}

// Usage
const client = new RealtimeClient('http://localhost:3101', 'your-jwt-token');

client.on('authenticated', (data) => {
  console.log('Ready!', data);
  client.joinRoom('general');
});

client.on('message:new', (message) => {
  console.log('New message:', message);
  displayMessage(message);
});

client.connect();
```

### Presence Tracking

```typescript
// PresenceTracker.ts - Track online users
import { Socket } from 'socket.io-client';

interface PresenceUpdate {
  userId: string;
  status: 'online' | 'away' | 'busy' | 'offline';
  customStatus?: {
    text: string;
    emoji: string;
  };
}

class PresenceTracker {
  private presenceMap = new Map<string, PresenceUpdate>();
  private socket: Socket;

  constructor(socket: Socket) {
    this.socket = socket;
    this.setupListeners();
  }

  private setupListeners() {
    this.socket.on('presence:changed', (update: PresenceUpdate) => {
      this.presenceMap.set(update.userId, update);
      this.notifySubscribers(update.userId);
    });

    this.socket.on('user:joined', ({ userId }) => {
      // Request presence for new user
      this.socket.emit('presence:request', { userId });
    });

    this.socket.on('user:left', ({ userId }) => {
      const presence = this.presenceMap.get(userId);
      if (presence) {
        presence.status = 'offline';
        this.notifySubscribers(userId);
      }
    });
  }

  setStatus(status: PresenceUpdate['status'], customStatus?: PresenceUpdate['customStatus']) {
    this.socket.emit('presence:update', { status, customStatus });
  }

  getPresence(userId: string): PresenceUpdate | undefined {
    return this.presenceMap.get(userId);
  }

  getOnlineUsers(): string[] {
    return Array.from(this.presenceMap.entries())
      .filter(([_, presence]) => presence.status === 'online')
      .map(([userId]) => userId);
  }

  subscribe(userId: string, callback: (presence: PresenceUpdate) => void) {
    // Implementation for subscriber pattern
  }

  private notifySubscribers(userId: string) {
    // Notify all subscribers of presence change
  }
}

// Usage in React
function UserPresence({ userId }: { userId: string }) {
  const [presence, setPresence] = useState<PresenceUpdate | null>(null);
  const { socket } = useRealtime({ url, token });

  useEffect(() => {
    if (!socket) return;

    const tracker = new PresenceTracker(socket);

    const handlePresenceChange = (update: PresenceUpdate) => {
      if (update.userId === userId) {
        setPresence(update);
      }
    };

    socket.on('presence:changed', handlePresenceChange);

    return () => {
      socket.off('presence:changed', handlePresenceChange);
    };
  }, [socket, userId]);

  const statusColor = {
    online: 'green',
    away: 'yellow',
    busy: 'red',
    offline: 'gray'
  }[presence?.status || 'offline'];

  return (
    <div className="user-presence">
      <div className={`status-indicator ${statusColor}`} />
      {presence?.customStatus?.emoji} {presence?.customStatus?.text}
    </div>
  );
}
```

### Typing Indicators

```typescript
// TypingIndicator.ts - Debounced typing indicators
import { Socket } from 'socket.io-client';

class TypingIndicator {
  private socket: Socket;
  private roomName: string;
  private threadId?: string;
  private timeout: NodeJS.Timeout | null = null;
  private isTyping = false;

  constructor(socket: Socket, roomName: string, threadId?: string) {
    this.socket = socket;
    this.roomName = roomName;
    this.threadId = threadId;
  }

  start() {
    if (!this.isTyping) {
      this.socket.emit('typing:start', {
        roomName: this.roomName,
        threadId: this.threadId
      });
      this.isTyping = true;
    }

    // Reset timeout
    if (this.timeout) {
      clearTimeout(this.timeout);
    }

    // Auto-stop after 3 seconds of inactivity
    this.timeout = setTimeout(() => {
      this.stop();
    }, 3000);
  }

  stop() {
    if (this.isTyping) {
      this.socket.emit('typing:stop', {
        roomName: this.roomName,
        threadId: this.threadId
      });
      this.isTyping = false;
    }

    if (this.timeout) {
      clearTimeout(this.timeout);
      this.timeout = null;
    }
  }

  cleanup() {
    this.stop();
  }
}

// Usage in React
function MessageInput({ roomName }: { roomName: string }) {
  const [message, setMessage] = useState('');
  const { socket } = useRealtime({ url, token });
  const typingIndicator = useRef<TypingIndicator | null>(null);

  useEffect(() => {
    if (socket) {
      typingIndicator.current = new TypingIndicator(socket, roomName);
    }

    return () => {
      typingIndicator.current?.cleanup();
    };
  }, [socket, roomName]);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setMessage(e.target.value);

    if (e.target.value.length > 0) {
      typingIndicator.current?.start();
    } else {
      typingIndicator.current?.stop();
    }
  };

  const handleSend = () => {
    if (message.trim()) {
      socket?.emit('message:send', { roomName, content: message });
      setMessage('');
      typingIndicator.current?.stop();
    }
  };

  return (
    <input
      type="text"
      value={message}
      onChange={handleChange}
      onKeyPress={(e) => e.key === 'Enter' && handleSend()}
    />
  );
}

// Display typing users
function TypingUsers({ roomName }: { roomName: string }) {
  const [typingUsers, setTypingUsers] = useState<string[]>([]);
  const { socket } = useRealtime({ url, token });

  useEffect(() => {
    if (!socket) return;

    const handleTyping = ({ roomName: room, users }: { roomName: string; users: string[] }) => {
      if (room === roomName) {
        setTypingUsers(users);
      }
    };

    socket.on('typing:event', handleTyping);

    return () => {
      socket.off('typing:event', handleTyping);
    };
  }, [socket, roomName]);

  if (typingUsers.length === 0) return null;

  const text = typingUsers.length === 1
    ? `${typingUsers[0]} is typing...`
    : typingUsers.length === 2
    ? `${typingUsers[0]} and ${typingUsers[1]} are typing...`
    : `${typingUsers[0]} and ${typingUsers.length - 1} others are typing...`;

  return <div className="typing-indicator">{text}</div>;
}
```

### Chat Room Manager

```typescript
// ChatRoomManager.ts - Multi-room management
import { Socket } from 'socket.io-client';

interface Room {
  name: string;
  type: 'channel' | 'dm' | 'group' | 'broadcast';
  unreadCount: number;
  lastMessage?: {
    content: string;
    timestamp: Date;
  };
}

class ChatRoomManager {
  private socket: Socket;
  private rooms = new Map<string, Room>();
  private currentRoom: string | null = null;

  constructor(socket: Socket) {
    this.socket = socket;
    this.setupListeners();
  }

  private setupListeners() {
    this.socket.on('room:joined', ({ roomName }) => {
      if (!this.rooms.has(roomName)) {
        this.rooms.set(roomName, {
          name: roomName,
          type: 'channel',
          unreadCount: 0
        });
      }
    });

    this.socket.on('message:new', ({ roomName, content, timestamp }) => {
      const room = this.rooms.get(roomName);
      if (room) {
        room.lastMessage = { content, timestamp };

        // Increment unread if not current room
        if (roomName !== this.currentRoom) {
          room.unreadCount++;
        }
      }
    });

    this.socket.on('user:joined', ({ roomName, userId }) => {
      console.log(`${userId} joined ${roomName}`);
    });

    this.socket.on('user:left', ({ roomName, userId }) => {
      console.log(`${userId} left ${roomName}`);
    });
  }

  joinRoom(roomName: string) {
    this.socket.emit('room:join', { roomName });
    this.currentRoom = roomName;

    // Reset unread count
    const room = this.rooms.get(roomName);
    if (room) {
      room.unreadCount = 0;
    }
  }

  leaveRoom(roomName: string) {
    this.socket.emit('room:leave', { roomName });

    if (this.currentRoom === roomName) {
      this.currentRoom = null;
    }
  }

  getRooms(): Room[] {
    return Array.from(this.rooms.values());
  }

  getUnreadTotal(): number {
    return Array.from(this.rooms.values())
      .reduce((sum, room) => sum + room.unreadCount, 0);
  }

  markAsRead(roomName: string) {
    const room = this.rooms.get(roomName);
    if (room) {
      room.unreadCount = 0;
    }
  }
}

// Usage in React
function RoomList() {
  const [rooms, setRooms] = useState<Room[]>([]);
  const { socket } = useRealtime({ url, token });
  const manager = useRef<ChatRoomManager | null>(null);

  useEffect(() => {
    if (socket) {
      manager.current = new ChatRoomManager(socket);

      const interval = setInterval(() => {
        setRooms(manager.current?.getRooms() || []);
      }, 1000);

      return () => clearInterval(interval);
    }
  }, [socket]);

  return (
    <div className="room-list">
      {rooms.map(room => (
        <div
          key={room.name}
          className="room-item"
          onClick={() => manager.current?.joinRoom(room.name)}
        >
          <span className="room-name">{room.name}</span>
          {room.unreadCount > 0 && (
            <span className="unread-badge">{room.unreadCount}</span>
          )}
          {room.lastMessage && (
            <div className="last-message">
              {room.lastMessage.content.substring(0, 50)}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
```

---

## Monitoring & Alerting

### Connection Metrics

Track real-time connection statistics:

```sql
-- Active connections by status
SELECT
    p.status,
    COUNT(DISTINCT c.socket_id) AS connection_count,
    AVG(EXTRACT(EPOCH FROM (NOW() - c.last_activity))) AS avg_idle_seconds
FROM realtime_connections c
LEFT JOIN realtime_presence p ON c.user_id = p.user_id
WHERE c.disconnected_at IS NULL
GROUP BY p.status;

-- Connection duration distribution
SELECT
    CASE
        WHEN duration < INTERVAL '1 minute' THEN '< 1 min'
        WHEN duration < INTERVAL '5 minutes' THEN '1-5 min'
        WHEN duration < INTERVAL '15 minutes' THEN '5-15 min'
        WHEN duration < INTERVAL '1 hour' THEN '15-60 min'
        ELSE '> 1 hour'
    END AS duration_bucket,
    COUNT(*) AS connection_count
FROM (
    SELECT NOW() - connected_at AS duration
    FROM realtime_connections
    WHERE disconnected_at IS NULL
) AS durations
GROUP BY duration_bucket
ORDER BY duration_bucket;

-- Peak concurrent connections (hourly)
SELECT
    DATE_TRUNC('hour', connected_at) AS hour,
    COUNT(*) AS connections
FROM realtime_connections
WHERE connected_at > NOW() - INTERVAL '24 hours'
GROUP BY hour
ORDER BY hour DESC;
```

### Message Throughput

Monitor message volume and patterns:

```sql
-- Messages per minute (last hour)
SELECT
    DATE_TRUNC('minute', created_at) AS minute,
    COUNT(*) AS message_count
FROM realtime_events
WHERE type = 'message'
  AND created_at > NOW() - INTERVAL '1 hour'
GROUP BY minute
ORDER BY minute DESC
LIMIT 60;

-- Top active rooms by message count
SELECT
    room_name,
    COUNT(*) AS message_count,
    COUNT(DISTINCT user_id) AS unique_users
FROM realtime_events
WHERE type = 'message'
  AND created_at > NOW() - INTERVAL '1 hour'
GROUP BY room_name
ORDER BY message_count DESC
LIMIT 20;

-- Message latency (if timestamps stored)
SELECT
    room_name,
    AVG(EXTRACT(EPOCH FROM (created_at - (data->>'client_timestamp')::TIMESTAMP))) AS avg_latency_seconds,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (created_at - (data->>'client_timestamp')::TIMESTAMP))) AS p95_latency
FROM realtime_events
WHERE type = 'message'
  AND data->>'client_timestamp' IS NOT NULL
  AND created_at > NOW() - INTERVAL '1 hour'
GROUP BY room_name;
```

### Room Statistics

```sql
-- Room activity summary
SELECT
    r.name,
    r.type,
    COUNT(DISTINCT rm.user_id) AS member_count,
    COUNT(DISTINCT CASE WHEN p.status = 'online' THEN rm.user_id END) AS online_count,
    (
        SELECT COUNT(*)
        FROM realtime_events e
        WHERE e.room_name = r.name
          AND e.type = 'message'
          AND e.created_at > NOW() - INTERVAL '24 hours'
    ) AS messages_24h
FROM realtime_rooms r
LEFT JOIN realtime_room_members rm ON r.id = rm.room_id AND rm.left_at IS NULL
LEFT JOIN realtime_presence p ON rm.user_id = p.user_id
GROUP BY r.id, r.name, r.type
ORDER BY messages_24h DESC;

-- Room growth (new members per day)
SELECT
    DATE(joined_at) AS date,
    room_id,
    COUNT(*) AS new_members
FROM realtime_room_members
WHERE joined_at > NOW() - INTERVAL '30 days'
GROUP BY DATE(joined_at), room_id
ORDER BY date DESC, new_members DESC;
```

### Error Tracking

```sql
-- Error frequency by type
SELECT
    data->>'error_code' AS error_code,
    COUNT(*) AS error_count,
    COUNT(DISTINCT user_id) AS affected_users,
    MAX(created_at) AS last_occurrence
FROM realtime_events
WHERE type = 'error'
  AND created_at > NOW() - INTERVAL '24 hours'
GROUP BY error_code
ORDER BY error_count DESC;

-- Connection failures
SELECT
    DATE_TRUNC('hour', created_at) AS hour,
    COUNT(*) AS failure_count,
    data->>'reason' AS failure_reason
FROM realtime_events
WHERE type IN ('disconnect', 'connection_error')
  AND created_at > NOW() - INTERVAL '24 hours'
GROUP BY hour, failure_reason
ORDER BY hour DESC, failure_count DESC;
```

### Prometheus Metrics

Export metrics in Prometheus format:

```typescript
// metrics.ts - Prometheus exporter
import { Registry, Counter, Gauge, Histogram } from 'prom-client';

const register = new Registry();

// Connection metrics
const connectionsTotal = new Counter({
  name: 'realtime_connections_total',
  help: 'Total number of connections',
  labelNames: ['type'], // authenticated, anonymous
  registers: [register]
});

const connectionsActive = new Gauge({
  name: 'realtime_connections_active',
  help: 'Currently active connections',
  labelNames: ['status'], // online, away, busy
  registers: [register]
});

// Message metrics
const messagesTotal = new Counter({
  name: 'realtime_messages_total',
  help: 'Total messages sent',
  labelNames: ['room_type'],
  registers: [register]
});

const messageLatency = new Histogram({
  name: 'realtime_message_latency_seconds',
  help: 'Message delivery latency',
  buckets: [0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1],
  registers: [register]
});

// Room metrics
const roomsActive = new Gauge({
  name: 'realtime_rooms_active',
  help: 'Number of active rooms',
  labelNames: ['type'],
  registers: [register]
});

// Error metrics
const errorsTotal = new Counter({
  name: 'realtime_errors_total',
  help: 'Total errors',
  labelNames: ['error_code'],
  registers: [register]
});

// Export metrics endpoint
app.get('/metrics', async (req, res) => {
  res.set('Content-Type', register.contentType);
  res.end(await register.metrics());
});

// Update metrics from database
async function updateMetrics() {
  // Active connections
  const { rows } = await db.query(`
    SELECT p.status, COUNT(*) AS count
    FROM realtime_connections c
    LEFT JOIN realtime_presence p ON c.user_id = p.user_id
    WHERE c.disconnected_at IS NULL
    GROUP BY p.status
  `);

  rows.forEach(row => {
    connectionsActive.set({ status: row.status || 'unknown' }, parseInt(row.count));
  });

  // Active rooms
  const roomStats = await db.query(`
    SELECT r.type, COUNT(*) AS count
    FROM realtime_rooms r
    WHERE EXISTS (
      SELECT 1 FROM realtime_room_members rm
      WHERE rm.room_id = r.id AND rm.left_at IS NULL
    )
    GROUP BY r.type
  `);

  roomStats.rows.forEach(row => {
    roomsActive.set({ type: row.type }, parseInt(row.count));
  });
}

// Update every 30 seconds
setInterval(updateMetrics, 30000);
```

### Grafana Dashboard

Example Grafana queries for visualization:

```promql
# Active connections
realtime_connections_active{status="online"}

# Message rate (per second)
rate(realtime_messages_total[1m])

# P95 message latency
histogram_quantile(0.95, rate(realtime_message_latency_seconds_bucket[5m]))

# Error rate
rate(realtime_errors_total[5m])

# Connection churn (connects - disconnects)
rate(realtime_connections_total{type="authenticated"}[5m]) -
rate(realtime_disconnections_total[5m])
```

### Alerting Rules

Prometheus alerting rules:

```yaml
# alerts.yml
groups:
  - name: realtime
    interval: 30s
    rules:
      # High error rate
      - alert: HighErrorRate
        expr: rate(realtime_errors_total[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High error rate detected"
          description: "Error rate is {{ $value }} errors/sec"

      # Connection drop
      - alert: ConnectionDrop
        expr: rate(realtime_connections_active[5m]) < -100
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Massive connection drop"
          description: "Losing {{ $value }} connections/sec"

      # High latency
      - alert: HighMessageLatency
        expr: histogram_quantile(0.95, rate(realtime_message_latency_seconds_bucket[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High message latency"
          description: "P95 latency is {{ $value }}s"

      # Database connection pool exhausted
      - alert: DatabasePoolExhausted
        expr: realtime_db_pool_idle_connections < 2
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Database connection pool nearly exhausted"

      # Memory usage high
      - alert: HighMemoryUsage
        expr: process_resident_memory_bytes > 2e9
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High memory usage: {{ $value | humanize }}B"
```

### Health Check Script

```bash
#!/bin/bash
# healthcheck.sh - Comprehensive health monitoring

REALTIME_URL="http://localhost:3101"
THRESHOLD_CONNECTIONS=5000
THRESHOLD_LATENCY_MS=100
THRESHOLD_ERROR_RATE=0.01

# Check server health
health=$(curl -s "$REALTIME_URL/health")
if [ $? -ne 0 ]; then
    echo "ERROR: Server not responding"
    exit 1
fi

# Check metrics
metrics=$(curl -s "$REALTIME_URL/metrics")

# Parse metrics
connections=$(echo "$metrics" | jq -r '.connections.total')
latency=$(echo "$metrics" | jq -r '.performance.avgLatencyMs')
error_rate=$(echo "$metrics" | jq -r '.errors.rate')

# Validate thresholds
if [ "$connections" -gt "$THRESHOLD_CONNECTIONS" ]; then
    echo "WARNING: High connection count: $connections"
fi

if [ "$latency" -gt "$THRESHOLD_LATENCY_MS" ]; then
    echo "WARNING: High latency: ${latency}ms"
fi

if (( $(echo "$error_rate > $THRESHOLD_ERROR_RATE" | bc -l) )); then
    echo "WARNING: High error rate: $error_rate"
fi

# Check database
psql $DATABASE_URL -c "SELECT COUNT(*) FROM realtime_connections WHERE disconnected_at IS NULL" > /dev/null
if [ $? -ne 0 ]; then
    echo "ERROR: Database connection failed"
    exit 1
fi

# Check Redis
redis-cli -u $REALTIME_REDIS_URL PING > /dev/null
if [ $? -ne 0 ]; then
    echo "ERROR: Redis connection failed"
    exit 1
fi

echo "OK: All health checks passed"
exit 0
```

---

## Use Cases

### 1. Live Chat Application

Real-time messaging for customer support or team collaboration.

**Implementation:**
```typescript
// Create chat rooms for support tickets
socket.emit('room:join', { roomName: 'support-ticket-12345' });

// Send messages
socket.emit('message:send', {
  roomName: 'support-ticket-12345',
  content: 'How can I help you?',
  metadata: { agentId: 'agent_001', priority: 'high' }
});

// Track agent typing
socket.on('typing:event', ({ users }) => {
  displayTypingIndicator(users);
});
```

### 2. Collaborative Document Editing

Multiple users editing the same document with cursor positions.

**Implementation:**
```typescript
// Join document room
socket.emit('room:join', { roomName: 'doc-abc123' });

// Broadcast cursor position
socket.emit('cursor:move', {
  roomName: 'doc-abc123',
  position: { line: 42, column: 15 },
  userId: 'user_123'
});

// Listen for other cursors
socket.on('cursor:update', ({ userId, position }) => {
  updateCursorDisplay(userId, position);
});

// Send text changes
socket.emit('text:change', {
  roomName: 'doc-abc123',
  changes: [{ start: 100, end: 105, text: 'hello' }]
});
```

### 3. Live Notifications

Push notifications for user actions, mentions, and alerts.

**Implementation:**
```typescript
// Subscribe to user-specific notifications
socket.emit('room:join', { roomName: `user-${userId}-notifications` });

// Receive notifications
socket.on('notification:new', (notification) => {
  showToast(notification.title, notification.message);
  updateNotificationBadge();
});

// Mark as read
socket.emit('notification:read', { notificationId: '123' });
```

### 4. Real-time Dashboard

Live metrics and analytics updates for admin dashboards.

**Implementation:**
```typescript
// Subscribe to metrics channel
socket.emit('room:join', { roomName: 'admin-metrics' });

// Receive metric updates
socket.on('metrics:update', ({ metric, value, timestamp }) => {
  updateChart(metric, value, timestamp);
});

// Request snapshot
socket.emit('metrics:snapshot', { metrics: ['users', 'revenue', 'orders'] });
```

### 5. Multiplayer Game

Real-time game state synchronization for players.

**Implementation:**
```typescript
// Join game room
socket.emit('room:join', { roomName: 'game-session-xyz' });

// Send player action
socket.emit('game:action', {
  roomName: 'game-session-xyz',
  action: 'move',
  data: { x: 100, y: 200 }
});

// Receive game state updates
socket.on('game:state', (state) => {
  updateGameState(state);
  renderScene();
});
```

### 6. Video Call Signaling

WebRTC signaling for peer-to-peer video calls.

**Implementation:**
```typescript
// Join call room
socket.emit('room:join', { roomName: 'call-room-123' });

// Send WebRTC offer
socket.emit('webrtc:offer', {
  roomName: 'call-room-123',
  targetUserId: 'user_456',
  offer: sdpOffer
});

// Receive WebRTC answer
socket.on('webrtc:answer', ({ fromUserId, answer }) => {
  peerConnection.setRemoteDescription(answer);
});

// Send ICE candidates
socket.emit('webrtc:ice-candidate', {
  roomName: 'call-room-123',
  candidate: iceCandidate
});
```

### 7. Live Polls & Voting

Real-time poll results during webinars or events.

**Implementation:**
```typescript
// Join poll room
socket.emit('room:join', { roomName: 'poll-event-123' });

// Submit vote
socket.emit('poll:vote', {
  roomName: 'poll-event-123',
  pollId: 'poll_1',
  optionId: 'option_a'
});

// Watch live results
socket.on('poll:results', ({ pollId, results }) => {
  updatePollChart(pollId, results);
});
```

### 8. Location Tracking

Real-time GPS tracking for delivery or ride-sharing apps.

**Implementation:**
```typescript
// Driver joins tracking room
socket.emit('room:join', { roomName: 'delivery-456' });

// Driver sends location updates
setInterval(() => {
  socket.emit('location:update', {
    roomName: 'delivery-456',
    coordinates: { lat: 37.7749, lng: -122.4194 },
    speed: 35,
    heading: 180
  });
}, 5000);

// Customer watches driver location
socket.on('location:update', ({ coordinates }) => {
  updateMapMarker(coordinates);
});
```

### 9. Auction Bidding

Real-time bid updates for online auctions.

**Implementation:**
```typescript
// Join auction room
socket.emit('room:join', { roomName: 'auction-item-789' });

// Place bid
socket.emit('bid:place', {
  roomName: 'auction-item-789',
  amount: 1500,
  bidderId: 'user_123'
});

// Watch bid updates
socket.on('bid:new', ({ amount, bidderId, timestamp }) => {
  updateHighestBid(amount);
  updateBidHistory({ amount, bidderId, timestamp });
});

// Auction ending soon
socket.on('auction:ending', ({ secondsRemaining }) => {
  startCountdown(secondsRemaining);
});
```

### 10. Social Media Feed

Live updates for social feeds (likes, comments, new posts).

**Implementation:**
```typescript
// Subscribe to feed
socket.emit('room:join', { roomName: 'user-feed-123' });

// New post appears
socket.on('post:new', (post) => {
  prependPostToFeed(post);
  showNotification('New post from ' + post.author);
});

// Real-time like updates
socket.on('post:liked', ({ postId, likeCount }) => {
  updateLikeCount(postId, likeCount);
});

// New comment
socket.on('comment:new', ({ postId, comment }) => {
  appendComment(postId, comment);
});
```

### 11. Stock Market Ticker

Live stock price updates and trading signals.

**Implementation:**
```typescript
// Subscribe to ticker symbols
socket.emit('room:join', { roomName: 'ticker-AAPL' });
socket.emit('room:join', { roomName: 'ticker-GOOGL' });

// Receive price updates
socket.on('price:update', ({ symbol, price, change, volume }) => {
  updateTickerDisplay(symbol, price, change);
  updateChart(symbol, price);
});

// Trading alerts
socket.on('alert:trigger', ({ symbol, condition, price }) => {
  showAlert(`${symbol} ${condition} at $${price}`);
});
```

### 12. Smart Home Control

Real-time device status and control for IoT devices.

**Implementation:**
```typescript
// Join home network room
socket.emit('room:join', { roomName: 'home-network-001' });

// Control device
socket.emit('device:control', {
  roomName: 'home-network-001',
  deviceId: 'thermostat_1',
  command: 'setTemperature',
  value: 72
});

// Receive device status
socket.on('device:status', ({ deviceId, status }) => {
  updateDeviceUI(deviceId, status);
});

// Sensor alerts
socket.on('sensor:alert', ({ sensorId, type, value }) => {
  if (type === 'motion') {
    triggerSecurityAlert();
  }
});
```

---

## Troubleshooting

### Common Issues

#### "Server won't start"

```
Error: EADDRINUSE: address already in use
```

**Solution:** Check if the port is already in use.

```bash
lsof -i :3101
```

#### "Redis Connection Failed"

```
Error: Redis connection to localhost:6379 failed
```

**Solutions:**
1. Verify Redis is running: `redis-cli -u $REALTIME_REDIS_URL ping`
2. Check `REALTIME_REDIS_URL` is set correctly in `.env`

#### "Database Connection Failed"

```
Error: Connection refused
```

**Solutions:**
1. Verify PostgreSQL is running
2. Check `DATABASE_URL` format
3. Test connection: `psql $DATABASE_URL -c "SELECT 1"`

#### "High Memory Usage"

**Solutions:**
1. Check connection count: `nself plugin realtime connections`
2. Clean up stale connections: `psql $DATABASE_URL -c "SELECT disconnect_stale_connections()"`
3. Lower `REALTIME_MAX_CONNECTIONS` if needed

#### "Messages Not Delivering"

**Solutions:**
1. Check room membership: `nself plugin realtime rooms`
2. Check event logs: `nself plugin realtime events -n 100`
3. Enable debug logging: `LOG_LEVEL=debug npm start`

### Debug Mode

Enable debug logging for detailed troubleshooting:

```bash
LOG_LEVEL=debug nself plugin realtime server start
```

### Health Checks

```bash
# Check server health
curl http://localhost:3101/health

# Check detailed metrics
curl http://localhost:3101/metrics
```

---

## Support

- **GitHub Issues:** [nself-plugins/issues](https://github.com/acamarata/nself-plugins/issues)
- **Socket.io Documentation:** [socket.io/docs](https://socket.io/docs/v4/)

---

*Last Updated: January 2026*
*Plugin Version: 1.0.0*
