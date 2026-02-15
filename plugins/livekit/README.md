# livekit

LiveKit voice/video infrastructure - room management, participant tracking, recording/egress, quality monitoring

## Installation

```bash
nself plugin install livekit
```

## Configuration

See plugin.json for environment variables and configuration options.

## Features

### JWT Token Generation

Full LiveKit JWT token generation is implemented using `livekit-server-sdk`:

- **Access token generation** with room name and participant identity
- **Permission control** (canPublish, canSubscribe, canPublishData)
- **Configurable expiration** (default: 3600s, max: 86400s)
- **Token tracking** with SHA-256 hashing for security
- **Revocation support** with audit trail

### Token API Endpoints

**Create Token** - `POST /api/livekit/tokens`
```json
{
  "roomName": "my-room",
  "participantIdentity": "user-123",
  "participantName": "John Doe",
  "grants": {
    "canPublish": true,
    "canSubscribe": true,
    "canPublishData": true
  },
  "ttl": 3600
}
```

**Response**:
```json
{
  "success": true,
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "tokenId": "uuid",
  "livekitUrl": "wss://localhost:7880",
  "expiresAt": "2024-02-15T12:00:00Z"
}
```

**Revoke Token** - `POST /api/livekit/tokens/:tokenId/revoke`
```json
{
  "revokedBy": "admin-user-id",
  "reason": "User banned"
}
```

**List Tokens** - `GET /api/livekit/tokens?roomId=uuid&limit=50&offset=0`

### Additional Features

- Room management (create, list, close)
- Participant tracking and management
- Recording/egress (room composite, track, stream)
- Quality monitoring and metrics
- Webhook event processing

## Usage

See plugin.json for complete CLI commands and API endpoints.

## License

See LICENSE file in repository root.
