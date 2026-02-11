# Devices Plugin

IoT device enrollment, trust management, command dispatch, and ingest session tracking service for nself applications.

## Overview

The Devices plugin provides comprehensive device management for IoT deployments, streaming infrastructure, and device-based services. It handles secure device enrollment, certificate-based trust, command dispatch, telemetry tracking, and ingest session management.

### Key Features

- **Device Enrollment**: Secure challenge-response enrollment flow
- **Trust Management**: Certificate-based device authentication
- **Command Dispatch**: Send commands to devices with acknowledgment tracking
- **Telemetry Tracking**: Monitor device health and metrics
- **Ingest Sessions**: Track active streaming/upload sessions
- **Device Lifecycle**: Manage device states (active, suspended, revoked)
- **Health Monitoring**: Device health scores and alerts
- **Audit Logging**: Complete audit trail of device operations
- **Multi-Device Types**: Support for various device types (antbox, encoder, etc.)
- **Fleet Management**: Manage large device fleets
- **Multi-App Support**: Isolated device management per app

### Use Cases

- **Live Streaming**: Manage streaming encoders and ingest endpoints
- **IoT Deployments**: Device fleet management
- **Content Delivery**: CDN edge device management
- **Smart Home**: Connected device management
- **Industrial IoT**: Manufacturing equipment monitoring
- **Security Systems**: Camera and sensor management
- **Digital Signage**: Display device management
- **Edge Computing**: Edge node management

---

## Quick Start

### Installation

```bash
# Install the plugin
nself plugin install devices

# Initialize database schema
nself devices init

# Start the server
nself devices server
```

### Basic Usage

```bash
# Start device enrollment
curl -X POST http://localhost:3603/v1/devices/enroll \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "antbox-001",
    "device_type": "antbox",
    "model": "AB-PRO-2024"
  }'

# Complete enrollment challenge
curl -X POST http://localhost:3603/v1/devices/antbox-001/complete-enrollment \
  -H "Content-Type: application/json" \
  -d '{
    "challenge_response": "signed-challenge-data",
    "public_key": "device-public-key"
  }'

# Send command to device
curl -X POST http://localhost:3603/v1/devices/antbox-001/commands \
  -H "Content-Type: application/json" \
  -d '{
    "command": "start_ingest",
    "parameters": {"stream_key": "abc123"}
  }'

# Check device status
curl http://localhost:3603/v1/devices/antbox-001

# Check plugin status
nself devices status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `REDIS_URL` | No | - | Redis connection string (for real-time) |
| `PORT` | No | `3603` | HTTP server port |
| `DEV_ENROLLMENT_TOKEN_TTL` | No | `300` | Enrollment token TTL (seconds) |
| `DEV_CHALLENGE_TTL` | No | `60` | Challenge TTL (seconds) |
| `DEV_HEARTBEAT_INTERVAL` | No | `30` | Expected heartbeat interval (seconds) |
| `DEV_HEARTBEAT_TIMEOUT` | No | `90` | Heartbeat timeout (seconds) |
| `DEV_COMMAND_DEFAULT_TIMEOUT` | No | `30` | Command timeout (seconds) |
| `DEV_COMMAND_MAX_RETRIES` | No | `3` | Max command retry attempts |
| `DEV_TELEMETRY_RETENTION_DAYS` | No | `90` | Days to retain telemetry |
| `DEV_INGEST_HEARTBEAT_INTERVAL` | No | `5` | Ingest heartbeat interval (seconds) |
| `DEV_INGEST_HEARTBEAT_TIMEOUT` | No | `15` | Ingest heartbeat timeout (seconds) |
| `DEV_INGEST_RETRY_MAX` | No | `3` | Max ingest retry attempts |
| `DEV_INGEST_RETRY_BACKOFF_BASE` | No | `1000` | Retry backoff base (ms) |
| `DEV_REALTIME_URL` | No | - | Realtime service URL |
| `DEV_RECORDING_URL` | No | - | Recording service URL |
| `DEV_STREAM_GATEWAY_URL` | No | - | Stream gateway URL |

### Example Configuration

```bash
# .env file
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
REDIS_URL=redis://localhost:6379
PORT=3603
DEV_ENROLLMENT_TOKEN_TTL=600
DEV_HEARTBEAT_INTERVAL=30
DEV_COMMAND_DEFAULT_TIMEOUT=60
DEV_TELEMETRY_RETENTION_DAYS=180
```

---

## CLI Commands

### `init`
Initialize database schema.

```bash
nself devices init
```

### `server`
Start API server.

```bash
nself devices server
```

### `status`
Show fleet statistics.

```bash
nself devices status
```

### `devices list`
List all devices.

```bash
nself devices devices list [options]

Options:
  --status <status>    Filter by status
  --type <type>        Filter by device type
  --limit <limit>      Results limit
```

### `devices enroll`
Start enrollment for device.

```bash
nself devices devices enroll <deviceId> [options]

Options:
  --type <type>        Device type
  --model <model>      Device model
```

### `devices revoke`
Revoke device trust.

```bash
nself devices devices revoke <deviceId> [options]

Options:
  --reason <reason>    Revocation reason
```

### `devices suspend`
Suspend device.

```bash
nself devices devices suspend <deviceId>
```

### `commands send`
Send command to device.

```bash
nself devices commands send <deviceId> <command> [options]

Options:
  --params <json>      Command parameters (JSON)
  --timeout <seconds>  Command timeout
```

### `ingest sessions`
List all ingest sessions.

```bash
nself devices ingest sessions
```

### `ingest active`
List active ingest sessions.

```bash
nself devices ingest active
```

### `health`
Show device health summary.

```bash
nself devices health
```

### `stats`
Show fleet-wide statistics.

```bash
nself devices stats
```

---

## REST API

### Health & Status

#### `GET /health`
Basic health check.

#### `GET /v1/status`
Fleet statistics.

**Response:**
```json
{
  "plugin": "devices",
  "version": "1.0.0",
  "status": "running",
  "stats": {
    "totalDevices": 234,
    "activeDevices": 198,
    "suspendedDevices": 12,
    "revokedDevices": 24,
    "totalCommands": 15234,
    "activeIngestSessions": 45,
    "devicesByType": {
      "antbox": 189,
      "encoder": 45
    }
  }
}
```

### Device Management

#### `POST /v1/devices/enroll`
Start device enrollment.

**Request:**
```json
{
  "device_id": "antbox-001",
  "device_type": "antbox",
  "model": "AB-PRO-2024",
  "metadata": {
    "location": "datacenter-1",
    "rack": "R42"
  }
}
```

**Response:**
```json
{
  "device_id": "antbox-001",
  "enrollment_token": "enroll-token-abc123",
  "challenge": "random-challenge-string",
  "expires_at": "2026-02-11T10:35:00Z"
}
```

#### `POST /v1/devices/:deviceId/complete-enrollment`
Complete enrollment with challenge response.

**Request:**
```json
{
  "challenge_response": "signed-challenge-data",
  "public_key": "device-public-key-pem",
  "certificate": "device-certificate-pem"
}
```

**Response:**
```json
{
  "device_id": "antbox-001",
  "status": "active",
  "enrolled_at": "2026-02-11T10:30:00Z",
  "trust_level": "trusted"
}
```

#### `GET /v1/devices`
List devices.

**Query Parameters:**
- `status`: Filter by status (active, suspended, revoked)
- `type`: Filter by device type
- `limit`: Results per page (default: 50)

**Response:**
```json
{
  "data": [
    {
      "device_id": "antbox-001",
      "device_type": "antbox",
      "model": "AB-PRO-2024",
      "status": "active",
      "last_heartbeat": "2026-02-11T10:29:00Z",
      "health_score": 95,
      "enrolled_at": "2026-02-10T08:00:00Z"
    }
  ],
  "total": 234
}
```

#### `GET /v1/devices/:deviceId`
Get device details.

**Response:**
```json
{
  "device_id": "antbox-001",
  "device_type": "antbox",
  "model": "AB-PRO-2024",
  "status": "active",
  "trust_level": "trusted",
  "public_key": "...",
  "last_heartbeat": "2026-02-11T10:29:00Z",
  "health_score": 95,
  "metadata": {...},
  "statistics": {
    "uptime_seconds": 345678,
    "commands_received": 523,
    "ingest_sessions": 89,
    "bytes_transmitted": 123456789012
  },
  "enrolled_at": "2026-02-10T08:00:00Z",
  "last_seen": "2026-02-11T10:29:00Z"
}
```

#### `POST /v1/devices/:deviceId/heartbeat`
Device heartbeat.

**Request:**
```json
{
  "health_metrics": {
    "cpu_percent": 45,
    "memory_percent": 67,
    "disk_percent": 34,
    "temperature": 52
  },
  "active_sessions": ["session-1", "session-2"]
}
```

**Response:**
```json
{
  "acknowledged": true,
  "pending_commands": [
    {
      "command_id": "cmd-uuid",
      "command": "update_config",
      "parameters": {...}
    }
  ]
}
```

#### `POST /v1/devices/:deviceId/revoke`
Revoke device trust.

**Request:**
```json
{
  "reason": "Security compromise"
}
```

**Response:**
```json
{
  "device_id": "antbox-001",
  "status": "revoked",
  "revoked_at": "2026-02-11T10:30:00Z"
}
```

#### `POST /v1/devices/:deviceId/suspend`
Suspend device.

**Response:**
```json
{
  "device_id": "antbox-001",
  "status": "suspended",
  "suspended_at": "2026-02-11T10:30:00Z"
}
```

#### `POST /v1/devices/:deviceId/activate`
Reactivate suspended device.

**Response:**
```json
{
  "device_id": "antbox-001",
  "status": "active",
  "activated_at": "2026-02-11T10:30:00Z"
}
```

### Command Dispatch

#### `POST /v1/devices/:deviceId/commands`
Send command to device.

**Request:**
```json
{
  "command": "start_ingest",
  "parameters": {
    "stream_key": "abc123",
    "rtmp_url": "rtmp://ingest.example.com/live",
    "bitrate": 5000
  },
  "timeout_seconds": 60,
  "priority": "high"
}
```

**Response:**
```json
{
  "command_id": "cmd-uuid",
  "device_id": "antbox-001",
  "command": "start_ingest",
  "status": "pending",
  "created_at": "2026-02-11T10:30:00Z",
  "expires_at": "2026-02-11T10:31:00Z"
}
```

#### `GET /v1/devices/:deviceId/commands`
List device commands.

**Query Parameters:**
- `status`: Filter by status (pending, acknowledged, completed, failed, timeout)
- `limit`: Results per page

**Response:**
```json
{
  "data": [
    {
      "command_id": "cmd-uuid",
      "command": "start_ingest",
      "status": "completed",
      "created_at": "2026-02-11T10:30:00Z",
      "acknowledged_at": "2026-02-11T10:30:05Z",
      "completed_at": "2026-02-11T10:30:15Z"
    }
  ],
  "total": 523
}
```

#### `GET /v1/commands/:commandId`
Get command status.

**Response:**
```json
{
  "command_id": "cmd-uuid",
  "device_id": "antbox-001",
  "command": "start_ingest",
  "parameters": {...},
  "status": "completed",
  "result": {
    "success": true,
    "session_id": "session-uuid"
  },
  "retry_count": 0,
  "created_at": "2026-02-11T10:30:00Z",
  "completed_at": "2026-02-11T10:30:15Z"
}
```

#### `POST /v1/commands/:commandId/acknowledge`
Device acknowledges command (called by device).

**Request:**
```json
{
  "device_id": "antbox-001"
}
```

**Response:**
```json
{
  "acknowledged": true
}
```

#### `POST /v1/commands/:commandId/complete`
Device reports command completion (called by device).

**Request:**
```json
{
  "device_id": "antbox-001",
  "success": true,
  "result": {
    "session_id": "session-uuid",
    "message": "Ingest started successfully"
  }
}
```

**Response:**
```json
{
  "completed": true
}
```

### Ingest Sessions

#### `POST /v1/ingest/sessions`
Start ingest session.

**Request:**
```json
{
  "device_id": "antbox-001",
  "stream_key": "abc123",
  "protocol": "rtmp",
  "source_url": "rtmp://source.example.com/live/stream",
  "metadata": {
    "title": "Live Event",
    "bitrate": 5000
  }
}
```

**Response:**
```json
{
  "session_id": "session-uuid",
  "device_id": "antbox-001",
  "status": "active",
  "started_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/ingest/sessions`
List ingest sessions.

**Query Parameters:**
- `device_id`: Filter by device
- `status`: Filter by status (active, completed, failed)
- `limit`: Results per page

**Response:**
```json
{
  "data": [
    {
      "session_id": "session-uuid",
      "device_id": "antbox-001",
      "stream_key": "abc123",
      "status": "active",
      "duration_seconds": 1234,
      "bytes_received": 123456789,
      "started_at": "2026-02-11T10:30:00Z"
    }
  ],
  "total": 89
}
```

#### `GET /v1/ingest/sessions/active`
List active sessions.

**Response:**
```json
{
  "data": [
    {
      "session_id": "session-uuid",
      "device_id": "antbox-001",
      "stream_key": "abc123",
      "duration_seconds": 1234,
      "health_score": 98,
      "bitrate_kbps": 4850,
      "started_at": "2026-02-11T10:30:00Z"
    }
  ],
  "total": 45
}
```

#### `POST /v1/ingest/sessions/:sessionId/heartbeat`
Ingest session heartbeat (called by device).

**Request:**
```json
{
  "device_id": "antbox-001",
  "metrics": {
    "bytes_received": 123456789,
    "bitrate_kbps": 4850,
    "dropped_frames": 12,
    "fps": 29.97
  }
}
```

**Response:**
```json
{
  "acknowledged": true,
  "continue": true
}
```

#### `POST /v1/ingest/sessions/:sessionId/end`
End ingest session.

**Request:**
```json
{
  "device_id": "antbox-001",
  "reason": "stream_ended",
  "final_metrics": {...}
}
```

**Response:**
```json
{
  "session_id": "session-uuid",
  "status": "completed",
  "duration_seconds": 3456,
  "ended_at": "2026-02-11T11:00:00Z"
}
```

### Telemetry

#### `POST /v1/devices/:deviceId/telemetry`
Report device telemetry (called by device).

**Request:**
```json
{
  "metrics": {
    "cpu_percent": 45,
    "memory_percent": 67,
    "disk_percent": 34,
    "temperature": 52,
    "network_rx_mbps": 850,
    "network_tx_mbps": 920
  },
  "timestamp": "2026-02-11T10:30:00Z"
}
```

**Response:**
```json
{
  "recorded": true
}
```

#### `GET /v1/devices/:deviceId/telemetry`
Get device telemetry history.

**Query Parameters:**
- `from`: Start time (ISO 8601)
- `to`: End time (ISO 8601)
- `metric`: Specific metric name
- `limit`: Results per page

**Response:**
```json
{
  "data": [
    {
      "timestamp": "2026-02-11T10:30:00Z",
      "metrics": {
        "cpu_percent": 45,
        "memory_percent": 67,
        "temperature": 52
      }
    }
  ],
  "total": 2880
}
```

#### `GET /v1/devices/:deviceId/health`
Get device health status.

**Response:**
```json
{
  "device_id": "antbox-001",
  "health_score": 95,
  "status": "healthy",
  "checks": {
    "heartbeat": {"status": "ok", "last": "2026-02-11T10:29:00Z"},
    "cpu": {"status": "ok", "value": 45},
    "memory": {"status": "ok", "value": 67},
    "temperature": {"status": "ok", "value": 52},
    "network": {"status": "ok"}
  },
  "last_check": "2026-02-11T10:30:00Z"
}
```

### Analytics

#### `GET /v1/analytics/fleet`
Fleet-wide analytics.

**Response:**
```json
{
  "total_devices": 234,
  "active_devices": 198,
  "health_distribution": {
    "excellent": 156,
    "good": 42,
    "fair": 12,
    "poor": 5
  },
  "device_types": {
    "antbox": 189,
    "encoder": 45
  },
  "total_ingest_sessions": 2341,
  "active_ingest_sessions": 45,
  "total_bytes_transmitted": 9876543210987,
  "average_uptime_hours": 234.5
}
```

---

## Database Schema

### `dev_devices`
```sql
CREATE TABLE dev_devices (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id VARCHAR(128) DEFAULT 'default',
  device_id VARCHAR(255) NOT NULL UNIQUE,
  device_type VARCHAR(64) NOT NULL,
  model VARCHAR(128),
  status VARCHAR(32) DEFAULT 'enrolling',
  trust_level VARCHAR(32) DEFAULT 'untrusted',
  public_key TEXT,
  certificate TEXT,
  last_heartbeat TIMESTAMP WITH TIME ZONE,
  health_score INTEGER DEFAULT 0,
  metadata JSONB DEFAULT '{}',
  enrolled_at TIMESTAMP WITH TIME ZONE,
  last_seen TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  CHECK (status IN ('enrolling', 'active', 'suspended', 'revoked')),
  CHECK (trust_level IN ('untrusted', 'pending', 'trusted'))
);
```

### `dev_commands`
```sql
CREATE TABLE dev_commands (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id VARCHAR(128) DEFAULT 'default',
  device_id VARCHAR(255) NOT NULL,
  command VARCHAR(128) NOT NULL,
  parameters JSONB DEFAULT '{}',
  status VARCHAR(32) DEFAULT 'pending',
  priority VARCHAR(16) DEFAULT 'normal',
  timeout_seconds INTEGER DEFAULT 30,
  retry_count INTEGER DEFAULT 0,
  max_retries INTEGER DEFAULT 3,
  result JSONB,
  error TEXT,
  acknowledged_at TIMESTAMP WITH TIME ZONE,
  completed_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  expires_at TIMESTAMP WITH TIME ZONE,
  CHECK (status IN ('pending', 'acknowledged', 'completed', 'failed', 'timeout', 'cancelled'))
);
```

### `dev_telemetry`
```sql
CREATE TABLE dev_telemetry (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id VARCHAR(128) DEFAULT 'default',
  device_id VARCHAR(255) NOT NULL,
  metrics JSONB NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### `dev_ingest_sessions`
```sql
CREATE TABLE dev_ingest_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id VARCHAR(128) DEFAULT 'default',
  device_id VARCHAR(255) NOT NULL,
  stream_key VARCHAR(255),
  protocol VARCHAR(32),
  status VARCHAR(32) DEFAULT 'active',
  source_url TEXT,
  bytes_received BIGINT DEFAULT 0,
  duration_seconds INTEGER DEFAULT 0,
  health_score INTEGER DEFAULT 100,
  metadata JSONB DEFAULT '{}',
  started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  last_heartbeat TIMESTAMP WITH TIME ZONE,
  ended_at TIMESTAMP WITH TIME ZONE,
  CHECK (status IN ('active', 'completed', 'failed', 'timeout'))
);
```

### `dev_audit_log`
```sql
CREATE TABLE dev_audit_log (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id VARCHAR(128) DEFAULT 'default',
  device_id VARCHAR(255),
  action VARCHAR(128) NOT NULL,
  actor VARCHAR(255),
  details JSONB DEFAULT '{}',
  ip_address INET,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

---

## Examples

### Example 1: Device Enrollment Flow

```bash
# 1. Device requests enrollment
curl -X POST http://localhost:3603/v1/devices/enroll \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "antbox-001",
    "device_type": "antbox",
    "model": "AB-PRO-2024"
  }'

# Response includes challenge
# 2. Device signs challenge with private key and submits
curl -X POST http://localhost:3603/v1/devices/antbox-001/complete-enrollment \
  -H "Content-Type: application/json" \
  -d '{
    "challenge_response": "signed-data",
    "public_key": "public-key-pem"
  }'

# 3. Device is now trusted and active
```

### Example 2: Send Command to Device

```bash
# Send start ingest command
curl -X POST http://localhost:3603/v1/devices/antbox-001/commands \
  -H "Content-Type: application/json" \
  -d '{
    "command": "start_ingest",
    "parameters": {
      "stream_key": "live-abc123",
      "rtmp_url": "rtmp://ingest.example.com/live"
    },
    "timeout_seconds": 60
  }'

# Device receives command via next heartbeat
# Device acknowledges command
# Device completes command and reports result
```

### Example 3: Monitor Device Health

```bash
# Get device health
curl http://localhost:3603/v1/devices/antbox-001/health

# Get telemetry history
curl "http://localhost:3603/v1/devices/antbox-001/telemetry?from=2026-02-11T00:00:00Z&to=2026-02-11T23:59:59Z"

# Get fleet analytics
curl http://localhost:3603/v1/analytics/fleet
```

### Example 4: Ingest Session Management

```bash
# Start ingest
curl -X POST http://localhost:3603/v1/ingest/sessions \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "antbox-001",
    "stream_key": "abc123",
    "protocol": "rtmp"
  }'

# Device sends heartbeats every 5 seconds
curl -X POST http://localhost:3603/v1/ingest/sessions/session-uuid/heartbeat \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "antbox-001",
    "metrics": {"bitrate_kbps": 4850}
  }'

# End session
curl -X POST http://localhost:3603/v1/ingest/sessions/session-uuid/end \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "antbox-001",
    "reason": "stream_ended"
  }'
```

### Example 5: Device Revocation

```bash
# Revoke compromised device
curl -X POST http://localhost:3603/v1/devices/antbox-001/revoke \
  -H "Content-Type: application/json" \
  -d '{
    "reason": "Security compromise detected"
  }'

# Device will be rejected on next heartbeat
```

---

## Troubleshooting

### Device Not Enrolling

**Solution:**
- Check enrollment token hasn't expired
- Verify challenge response is correctly signed
- Ensure public key format is correct
- Check device_id is unique

### Commands Not Reaching Device

**Solution:**
- Verify device is sending heartbeats
- Check device status is "active"
- Review command timeout settings
- Check network connectivity

### Ingest Session Failures

**Solution:**
- Verify device health score
- Check network bandwidth
- Review ingest heartbeat frequency
- Check stream gateway connectivity

### High Device Count Performance

**Solution:**
- Increase heartbeat intervals
- Implement device grouping
- Use Redis for real-time state
- Scale horizontally

---

## License

Source-Available License

## Support

- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- Plugin Homepage: https://github.com/acamarata/nself-plugins/tree/main/plugins/devices
