# Chat Plugin for nself

Production-ready chat and messaging plugin for nself CLI. Manage conversations, messages, participants, read receipts, and moderation actions.

## Features

- **Conversations**: Direct messages, group chats, and channels
- **Messages**: Text, images, files, audio, video, system messages, and embeds
- **Participants**: Role-based access (owner, admin, moderator, member, guest)
- **Read Receipts**: Track unread messages per user
- **Reactions**: Emoji reactions on messages
- **Threads**: Reply chains and threaded conversations
- **Pinned Messages**: Pin important messages with configurable limits
- **Message Editing**: Edit messages within a configurable time window
- **Search**: Full-text search across messages
- **Moderation**: Delete messages, warn, mute, kick, and ban users
- **Multi-App Support**: Isolate data by `source_account_id`

## Quick Start

### Installation

```bash
cd plugins/chat/ts
npm install
npm run build
```

### Configuration

```bash
cp .env.example .env
# Edit .env with your database credentials
```

### Initialize Database

```bash
npm run build
node dist/cli.js init
```

### Start Server

```bash
node dist/cli.js server
# Or in development mode:
npm run dev
```

## CLI Commands

### `init`
Initialize the database schema.

```bash
nself-chat init
```

### `server`
Start the HTTP server.

```bash
nself-chat server [--port 3401] [--host 0.0.0.0]
```

### `status`
Show plugin status and statistics.

```bash
nself-chat status
```

### `conversations`
List conversations.

```bash
nself-chat conversations [--user <userId>] [--limit 20]
```

### `messages`
List messages in a conversation.

```bash
nself-chat messages --conversation <id> [--limit 20]
```

### `stats`
Show detailed statistics.

```bash
nself-chat stats
```

## REST API Endpoints

### Health Checks

- `GET /health` - Basic health check
- `GET /ready` - Readiness check (verifies database connectivity)
- `GET /live` - Liveness check with stats
- `GET /v1/status` - Full status with statistics

### Conversations

- `POST /v1/conversations` - Create conversation
- `GET /v1/conversations` - List conversations
- `GET /v1/conversations/:id` - Get conversation details
- `PUT /v1/conversations/:id` - Update conversation
- `DELETE /v1/conversations/:id` - Archive conversation

### Participants

- `POST /v1/conversations/:id/participants` - Add participant
- `GET /v1/conversations/:id/participants` - List participants
- `PUT /v1/conversations/:id/participants/:userId` - Update participant role
- `DELETE /v1/conversations/:id/participants/:userId` - Remove participant

### Messages

- `POST /v1/conversations/:id/messages` - Send message
- `GET /v1/conversations/:id/messages` - List messages (cursor-based pagination)
- `GET /v1/conversations/:id/messages/:msgId` - Get message
- `PUT /v1/conversations/:id/messages/:msgId` - Edit message
- `DELETE /v1/conversations/:id/messages/:msgId` - Delete message

### Reactions

- `POST /v1/conversations/:id/messages/:msgId/reactions` - Add reaction
- `DELETE /v1/conversations/:id/messages/:msgId/reactions/:emoji` - Remove reaction

### Pinned Messages

- `POST /v1/conversations/:id/messages/:msgId/pin` - Pin message
- `DELETE /v1/conversations/:id/messages/:msgId/pin` - Unpin message
- `GET /v1/conversations/:id/pinned` - List pinned messages

### Threads

- `GET /v1/conversations/:id/threads/:msgId` - Get thread messages

### Read Receipts

- `POST /v1/conversations/:id/read` - Mark as read
- `GET /v1/conversations/:id/unread` - Get unread count

### Search

- `GET /v1/conversations/:id/search` - Search messages

### Typing Indicators

- `POST /v1/conversations/:id/typing` - Send typing indicator (fire-and-forget)

### Moderation

- `POST /v1/moderate` - Execute moderation action

### Sync

- `GET /v1/sync` - Get updates since timestamp

## Database Schema

### Tables

1. **chat_conversations** - Conversation metadata
2. **chat_participants** - Conversation participants with roles
3. **chat_messages** - All messages
4. **chat_read_receipts** - Read receipts and unread counts
5. **chat_moderation_actions** - Moderation action log
6. **chat_webhook_events** - Webhook event log

See [DEVELOPMENT.md](../../.wiki/DEVELOPMENT.md) for complete schema details.

## Configuration

### Environment Variables

**Required:**
- `DATABASE_URL` - PostgreSQL connection string

**Optional:**
- `CHAT_PLUGIN_PORT` (default: 3401)
- `CHAT_MAX_MESSAGE_LENGTH` (default: 10000)
- `CHAT_MAX_ATTACHMENTS` (default: 10)
- `CHAT_EDIT_WINDOW_MINUTES` (default: 15)
- `CHAT_MAX_PARTICIPANTS` (default: 100)
- `CHAT_MAX_PINNED` (default: 50)
- `CHAT_API_KEY` - API key for authentication
- `CHAT_RATE_LIMIT_MAX` (default: 200)
- `CHAT_RATE_LIMIT_WINDOW_MS` (default: 60000)

### Security

Enable API key authentication by setting `CHAT_API_KEY`:

```bash
CHAT_API_KEY=your-secret-key
```

All requests must include the header:
```
X-API-Key: your-secret-key
```

## Development

### Build

```bash
npm run build
```

### Watch Mode

```bash
npm run watch
```

### Type Checking

```bash
npm run typecheck
```

### Development Server

```bash
npm run dev
```

## Example Usage

### Create a Conversation

```bash
curl -X POST http://localhost:3401/v1/conversations \
  -H "Content-Type: application/json" \
  -d '{
    "type": "group",
    "name": "Team Chat",
    "created_by": "user1",
    "participant_ids": ["user1", "user2", "user3"]
  }'
```

### Send a Message

```bash
curl -X POST http://localhost:3401/v1/conversations/{id}/messages \
  -H "Content-Type: application/json" \
  -d '{
    "sender_id": "user1",
    "content": "Hello team!",
    "content_type": "text"
  }'
```

### List Messages

```bash
curl http://localhost:3401/v1/conversations/{id}/messages?limit=50
```

### Add Reaction

```bash
curl -X POST http://localhost:3401/v1/conversations/{id}/messages/{msgId}/reactions \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user2",
    "emoji": "👍"
  }'
```

## License

MIT
