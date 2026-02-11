# Chat Plugin

Comprehensive chat and messaging system with conversations, participants, messages, read receipts, reactions, and moderation for nself applications.

## Overview

The Chat plugin provides a complete messaging infrastructure for building real-time chat features. It supports direct messages, group chats, channels, threaded conversations, message reactions, read receipts, and comprehensive moderation tools.

### Key Features

- **Conversation Types**: Direct messages, group chats, and broadcast channels
- **Rich Messaging**: Text, images, files, audio, video, and embeds
- **Message Threading**: Reply chains and threaded discussions
- **Reactions & Mentions**: Emoji reactions and @mentions
- **Read Receipts**: Track who read which messages
- **Message Editing**: Edit messages within a configurable time window
- **Pinned Messages**: Pin important messages to conversations
- **Participant Management**: Roles, permissions, and notifications
- **Moderation Tools**: Delete messages, warn, mute, kick, and ban users
- **Search & Filtering**: Full-text message search
- **Multi-App Support**: Isolated chat instances per source account
- **Attachments**: Support for multiple file attachments per message

### Use Cases

- **Customer Support**: Live chat support systems
- **Team Collaboration**: Internal team communication
- **Social Networks**: Private messaging between users
- **Gaming**: In-game chat and guild communication
- **E-commerce**: Buyer-seller messaging
- **Education**: Student-teacher communication
- **Healthcare**: Secure patient-provider messaging
- **Community Platforms**: Forums and discussion boards

---

## Quick Start

### Installation

```bash
# Install the plugin
nself plugin install chat

# Initialize database schema
nself chat init

# Start the server
nself chat server
```

### Basic Usage

```bash
# Create a conversation
curl -X POST http://localhost:3401/v1/conversations \
  -H "Content-Type: application/json" \
  -d '{
    "type": "group",
    "name": "Team Chat",
    "created_by": "user123"
  }'

# Send a message
curl -X POST http://localhost:3401/v1/conversations/conv-id/messages \
  -H "Content-Type: application/json" \
  -d '{
    "sender_id": "user123",
    "content": "Hello team!"
  }'

# View messages
curl http://localhost:3401/v1/conversations/conv-id/messages

# Check status
nself chat status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `CHAT_PLUGIN_PORT` | No | `3401` | HTTP server port |
| `CHAT_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server host |
| `CHAT_MAX_MESSAGE_LENGTH` | No | `10000` | Maximum message content length |
| `CHAT_MAX_ATTACHMENTS` | No | `10` | Maximum attachments per message |
| `CHAT_EDIT_WINDOW_MINUTES` | No | `15` | Time window for editing messages |
| `CHAT_MAX_PARTICIPANTS` | No | `100` | Maximum participants per conversation |
| `CHAT_MAX_PINNED` | No | `50` | Maximum pinned messages per conversation |
| `CHAT_API_KEY` | No | - | API key for authentication |
| `CHAT_RATE_LIMIT_MAX` | No | `200` | Max requests per window |
| `CHAT_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (ms) |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | - | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `LOG_LEVEL` | No | `info` | Logging level |

### Example Configuration

```bash
# .env file
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
CHAT_PLUGIN_PORT=3401
CHAT_MAX_MESSAGE_LENGTH=5000
CHAT_MAX_ATTACHMENTS=5
CHAT_EDIT_WINDOW_MINUTES=30
CHAT_MAX_PARTICIPANTS=200
CHAT_API_KEY=your-secret-key
```

---

## CLI Commands

### `init`
Initialize the database schema.

```bash
nself chat init
```

### `server`
Start the HTTP API server.

```bash
nself chat server [options]

Options:
  -p, --port <port>    Server port (default: 3401)
  -h, --host <host>    Server host (default: 0.0.0.0)
```

**Example:**
```bash
nself chat server --port 3401 --host 0.0.0.0
```

### `status`
Show plugin status and statistics.

```bash
nself chat status
```

**Output:**
```
Chat Plugin Status
==================
Version:              1.0.0
Port:                 3401
Max Message Length:   10000
Max Attachments:      10
Edit Window:          15 minutes
Max Participants:     100
Max Pinned:           50

Database Statistics
===================
Total Conversations:  1523
Active Conversations: 987
Total Participants:   4521
Total Users:          2341
Total Messages:       125678
Messages (24h):       3421
```

### `conversations`
List conversations.

```bash
nself chat conversations [options]

Options:
  -u, --user <userId>    Filter by user ID
  -l, --limit <limit>    Number to show (default: 20)
```

**Examples:**
```bash
# List all conversations
nself chat conversations

# Filter by user
nself chat conversations --user user123

# Limit results
nself chat conversations --limit 50
```

### `messages`
List messages in a conversation.

```bash
nself chat messages <conversationId> [options]

Options:
  -l, --limit <limit>    Number to show (default: 50)
```

**Example:**
```bash
nself chat messages conv-uuid-123 --limit 100
```

### `stats`
Show detailed statistics (alias for `status`).

```bash
nself chat stats
```

---

## REST API

All endpoints support multi-app isolation via `X-Source-Account-Id` header.

### Health & Status

#### `GET /health`
Basic health check.

**Response:**
```json
{
  "status": "ok",
  "plugin": "chat",
  "timestamp": "2026-02-11T10:30:00Z"
}
```

#### `GET /ready`
Readiness check with database connectivity.

**Response:**
```json
{
  "ready": true,
  "plugin": "chat",
  "timestamp": "2026-02-11T10:30:00Z"
}
```

#### `GET /live`
Liveness check with stats.

**Response:**
```json
{
  "alive": true,
  "plugin": "chat",
  "version": "1.0.0",
  "uptime": 3600.5,
  "memory": {
    "rss": 52428800,
    "heapTotal": 20971520,
    "heapUsed": 15728640
  },
  "stats": {
    "conversations": 1523,
    "messages": 125678,
    "activeConversations": 987
  },
  "timestamp": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/status`
Plugin status and statistics.

**Response:**
```json
{
  "plugin": "chat",
  "version": "1.0.0",
  "status": "running",
  "stats": {
    "conversations": 1523,
    "activeConversations": 987,
    "participants": 4521,
    "totalUsers": 2341,
    "messages": 125678,
    "messagesLast24h": 3421,
    "messagesByType": {
      "text": 110234,
      "image": 8932,
      "file": 4521,
      "system": 1991
    },
    "conversationsByType": {
      "direct": 892,
      "group": 445,
      "channel": 186
    }
  },
  "timestamp": "2026-02-11T10:30:00Z"
}
```

### Conversations

#### `POST /v1/conversations`
Create a new conversation.

**Request:**
```json
{
  "type": "group",
  "name": "Project Team",
  "description": "Team chat for Project Alpha",
  "avatar_url": "https://example.com/avatar.jpg",
  "created_by": "user123",
  "metadata": {
    "project_id": "proj-456",
    "department": "engineering"
  }
}
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "source_account_id": "primary",
  "type": "group",
  "name": "Project Team",
  "description": "Team chat for Project Alpha",
  "avatar_url": "https://example.com/avatar.jpg",
  "created_by": "user123",
  "is_archived": false,
  "is_muted": false,
  "last_message_at": null,
  "last_message_preview": null,
  "message_count": 0,
  "member_count": 0,
  "metadata": {
    "project_id": "proj-456",
    "department": "engineering"
  },
  "created_at": "2026-02-11T10:30:00Z",
  "updated_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/conversations`
List conversations.

**Query Parameters:**
- `user_id`: Filter by user ID (only conversations they're in)
- `limit`: Results per page (default: 50)
- `offset`: Pagination offset (default: 0)

**Response:**
```json
{
  "data": [
    {
      "id": "conv-uuid",
      "type": "group",
      "name": "Project Team",
      "member_count": 5,
      "message_count": 234,
      "last_message_at": "2026-02-11T10:25:00Z",
      "last_message_preview": "Hey team, meeting at 3pm",
      "created_at": "2026-02-10T08:00:00Z"
    }
  ],
  "total": 15,
  "limit": 50,
  "offset": 0
}
```

#### `GET /v1/conversations/:id`
Get conversation details with participant information.

**Query Parameters:**
- `user_id`: Get user-specific details (unread count, read receipts)

**Response:**
```json
{
  "id": "conv-uuid",
  "type": "group",
  "name": "Project Team",
  "description": "Team chat for Project Alpha",
  "member_count": 5,
  "message_count": 234,
  "last_message_at": "2026-02-11T10:25:00Z",
  "participants": [
    {
      "id": "participant-uuid",
      "user_id": "user123",
      "role": "admin",
      "nickname": "John D",
      "joined_at": "2026-02-10T08:00:00Z",
      "notification_level": "all"
    }
  ],
  "unread_count": 5,
  "last_read_at": "2026-02-11T09:00:00Z",
  "created_at": "2026-02-10T08:00:00Z"
}
```

#### `PUT /v1/conversations/:id`
Update conversation details.

**Request:**
```json
{
  "name": "Updated Project Team",
  "description": "New description",
  "is_archived": false,
  "is_muted": true
}
```

**Response:**
```json
{
  "id": "conv-uuid",
  "name": "Updated Project Team",
  "description": "New description",
  "is_archived": false,
  "is_muted": true,
  "updated_at": "2026-02-11T10:30:00Z"
}
```

#### `DELETE /v1/conversations/:id`
Archive a conversation (soft delete).

**Response:**
```json
{
  "success": true,
  "conversation": {
    "id": "conv-uuid",
    "is_archived": true
  }
}
```

### Participants

#### `POST /v1/conversations/:id/participants`
Add a participant to a conversation.

**Request:**
```json
{
  "user_id": "user456",
  "role": "member",
  "notification_level": "all"
}
```

**Response:**
```json
{
  "id": "participant-uuid",
  "conversation_id": "conv-uuid",
  "user_id": "user456",
  "role": "member",
  "notification_level": "all",
  "joined_at": "2026-02-11T10:30:00Z",
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/conversations/:id/participants`
List conversation participants.

**Response:**
```json
{
  "data": [
    {
      "id": "participant-uuid",
      "user_id": "user123",
      "role": "admin",
      "nickname": "John D",
      "joined_at": "2026-02-10T08:00:00Z",
      "last_read_at": "2026-02-11T10:20:00Z",
      "notification_level": "all"
    }
  ],
  "total": 5
}
```

#### `PUT /v1/conversations/:id/participants/:userId`
Update participant settings.

**Request:**
```json
{
  "role": "moderator",
  "nickname": "Johnny",
  "notification_level": "mentions"
}
```

**Response:**
```json
{
  "id": "participant-uuid",
  "role": "moderator",
  "nickname": "Johnny",
  "notification_level": "mentions",
  "updated_at": "2026-02-11T10:30:00Z"
}
```

#### `DELETE /v1/conversations/:id/participants/:userId`
Remove a participant from conversation.

**Response:**
```json
{
  "success": true
}
```

### Messages

#### `POST /v1/conversations/:id/messages`
Send a new message.

**Request:**
```json
{
  "sender_id": "user123",
  "content": "Hello team! Here's the update.",
  "content_type": "text",
  "reply_to_id": "msg-uuid",
  "thread_root_id": "thread-uuid",
  "attachments": [
    {
      "type": "image",
      "url": "https://example.com/image.jpg",
      "filename": "screenshot.jpg",
      "size": 245678,
      "mime_type": "image/jpeg"
    }
  ],
  "mentions": ["user456", "user789"],
  "metadata": {
    "client": "web",
    "version": "2.0"
  }
}
```

**Response:**
```json
{
  "id": "msg-uuid",
  "conversation_id": "conv-uuid",
  "sender_id": "user123",
  "content": "Hello team! Here's the update.",
  "content_type": "text",
  "reply_to_id": "msg-uuid",
  "thread_root_id": "thread-uuid",
  "attachments": [...],
  "mentions": ["user456", "user789"],
  "reactions": {},
  "edited_at": null,
  "deleted_at": null,
  "is_pinned": false,
  "metadata": {...},
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/conversations/:id/messages`
List messages in a conversation.

**Query Parameters:**
- `limit`: Results per page (default: 50)
- `offset`: Pagination offset (default: 0)
- `before`: Get messages before this timestamp
- `after`: Get messages after this timestamp
- `thread_root_id`: Filter by thread
- `search`: Search message content

**Response:**
```json
{
  "data": [
    {
      "id": "msg-uuid",
      "sender_id": "user123",
      "content": "Hello team!",
      "created_at": "2026-02-11T10:30:00Z",
      "reactions": {
        "👍": ["user456", "user789"],
        "❤️": ["user456"]
      },
      "is_pinned": false
    }
  ],
  "total": 234,
  "limit": 50,
  "offset": 0,
  "hasMore": true
}
```

#### `GET /v1/conversations/:id/messages/:messageId`
Get a single message by ID.

**Response:**
```json
{
  "id": "msg-uuid",
  "conversation_id": "conv-uuid",
  "sender_id": "user123",
  "content": "Hello team!",
  "content_type": "text",
  "attachments": [],
  "mentions": [],
  "reactions": {},
  "reply_count": 5,
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `PUT /v1/conversations/:id/messages/:messageId`
Edit a message (within edit window).

**Request:**
```json
{
  "content": "Updated message content"
}
```

**Response:**
```json
{
  "id": "msg-uuid",
  "content": "Updated message content",
  "edited_at": "2026-02-11T10:32:00Z"
}
```

#### `DELETE /v1/conversations/:id/messages/:messageId`
Delete a message (soft delete).

**Response:**
```json
{
  "success": true,
  "message": {
    "id": "msg-uuid",
    "deleted_at": "2026-02-11T10:32:00Z"
  }
}
```

### Reactions

#### `POST /v1/conversations/:id/messages/:messageId/reactions`
Add a reaction to a message.

**Request:**
```json
{
  "user_id": "user123",
  "emoji": "👍"
}
```

**Response:**
```json
{
  "success": true,
  "reactions": {
    "👍": ["user123", "user456"],
    "❤️": ["user789"]
  }
}
```

#### `DELETE /v1/conversations/:id/messages/:messageId/reactions/:emoji`
Remove a reaction.

**Query Parameters:**
- `user_id`: User removing the reaction

**Response:**
```json
{
  "success": true,
  "reactions": {
    "👍": ["user456"],
    "❤️": ["user789"]
  }
}
```

### Pinned Messages

#### `POST /v1/conversations/:id/messages/:messageId/pin`
Pin a message to the conversation.

**Request:**
```json
{
  "pinned_by": "user123"
}
```

**Response:**
```json
{
  "success": true,
  "message": {
    "id": "msg-uuid",
    "is_pinned": true,
    "pinned_at": "2026-02-11T10:30:00Z",
    "pinned_by": "user123"
  }
}
```

#### `GET /v1/conversations/:id/pinned`
List pinned messages in a conversation.

**Response:**
```json
{
  "data": [
    {
      "id": "msg-uuid",
      "content": "Important announcement",
      "is_pinned": true,
      "pinned_at": "2026-02-11T09:00:00Z",
      "pinned_by": "user123",
      "created_at": "2026-02-11T08:00:00Z"
    }
  ],
  "total": 3
}
```

#### `DELETE /v1/conversations/:id/messages/:messageId/pin`
Unpin a message.

**Response:**
```json
{
  "success": true
}
```

### Read Receipts

#### `POST /v1/conversations/:id/read`
Update read receipt for a user.

**Request:**
```json
{
  "user_id": "user123",
  "last_read_message_id": "msg-uuid"
}
```

**Response:**
```json
{
  "success": true,
  "unread_count": 0,
  "last_read_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/conversations/:id/unread`
Get unread count for a user.

**Query Parameters:**
- `user_id`: User ID

**Response:**
```json
{
  "conversation_id": "conv-uuid",
  "user_id": "user123",
  "unread_count": 5,
  "last_read_at": "2026-02-11T09:00:00Z"
}
```

### Moderation

#### `POST /v1/conversations/:id/moderate`
Take a moderation action.

**Request:**
```json
{
  "action": "mute",
  "target_user_id": "user789",
  "moderator_id": "user123",
  "reason": "Spamming",
  "duration_minutes": 60
}
```

**Response:**
```json
{
  "id": "action-uuid",
  "action": "mute",
  "target_user_id": "user789",
  "moderator_id": "user123",
  "reason": "Spamming",
  "duration_minutes": 60,
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/conversations/:id/moderation`
List moderation actions for a conversation.

**Response:**
```json
{
  "data": [
    {
      "id": "action-uuid",
      "action": "delete_message",
      "message_id": "msg-uuid",
      "moderator_id": "user123",
      "reason": "Inappropriate content",
      "created_at": "2026-02-11T10:00:00Z"
    }
  ],
  "total": 12
}
```

---

## Webhook Events

### `conversation.created`
Triggered when a new conversation is created.

### `conversation.updated`
Triggered when conversation metadata is updated.

### `conversation.archived`
Triggered when a conversation is archived.

### `participant.joined`
Triggered when a participant joins a conversation.

### `participant.left`
Triggered when a participant leaves.

### `participant.role_changed`
Triggered when a participant's role is updated.

### `message.created`
Triggered when a new message is sent.

### `message.updated`
Triggered when a message is edited.

### `message.deleted`
Triggered when a message is deleted.

### `message.pinned`
Triggered when a message is pinned.

### `message.unpinned`
Triggered when a message is unpinned.

### `message.reaction_added`
Triggered when a reaction is added.

### `message.reaction_removed`
Triggered when a reaction is removed.

### `moderation.action_taken`
Triggered when a moderation action is executed.

---

## Database Schema

### `chat_conversations`
Stores all conversations.

```sql
CREATE TABLE chat_conversations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  type VARCHAR(16) NOT NULL DEFAULT 'direct',
  name VARCHAR(255),
  description TEXT,
  avatar_url TEXT,
  created_by VARCHAR(255),
  is_archived BOOLEAN DEFAULT false,
  is_muted BOOLEAN DEFAULT false,
  last_message_at TIMESTAMP WITH TIME ZONE,
  last_message_preview TEXT,
  message_count INTEGER DEFAULT 0,
  member_count INTEGER DEFAULT 0,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  CHECK (type IN ('direct', 'group', 'channel'))
);
```

### `chat_participants`
Tracks conversation participants.

```sql
CREATE TABLE chat_participants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  conversation_id UUID NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
  user_id VARCHAR(255) NOT NULL,
  role VARCHAR(32) DEFAULT 'member',
  nickname VARCHAR(128),
  muted_until TIMESTAMP WITH TIME ZONE,
  joined_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  left_at TIMESTAMP WITH TIME ZONE,
  last_read_message_id UUID,
  last_read_at TIMESTAMP WITH TIME ZONE,
  notification_level VARCHAR(32) DEFAULT 'all',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(conversation_id, user_id),
  CHECK (role IN ('owner', 'admin', 'moderator', 'member', 'guest')),
  CHECK (notification_level IN ('all', 'mentions', 'none'))
);
```

### `chat_messages`
Stores all messages.

```sql
CREATE TABLE chat_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  conversation_id UUID NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
  sender_id VARCHAR(255) NOT NULL,
  reply_to_id UUID REFERENCES chat_messages(id),
  thread_root_id UUID REFERENCES chat_messages(id),
  content TEXT,
  content_type VARCHAR(32) DEFAULT 'text',
  attachments JSONB DEFAULT '[]',
  mentions TEXT[] DEFAULT '{}',
  reactions JSONB DEFAULT '{}',
  edited_at TIMESTAMP WITH TIME ZONE,
  deleted_at TIMESTAMP WITH TIME ZONE,
  is_pinned BOOLEAN DEFAULT false,
  pinned_at TIMESTAMP WITH TIME ZONE,
  pinned_by VARCHAR(255),
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  CHECK (content_type IN ('text', 'image', 'file', 'audio', 'video', 'system', 'embed'))
);
```

### `chat_read_receipts`
Tracks read status per user.

```sql
CREATE TABLE chat_read_receipts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  conversation_id UUID NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
  user_id VARCHAR(255) NOT NULL,
  last_read_message_id UUID REFERENCES chat_messages(id),
  last_read_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  unread_count INTEGER DEFAULT 0,
  UNIQUE(conversation_id, user_id)
);
```

### `chat_moderation_actions`
Logs moderation actions.

```sql
CREATE TABLE chat_moderation_actions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  conversation_id UUID REFERENCES chat_conversations(id),
  message_id UUID REFERENCES chat_messages(id),
  target_user_id VARCHAR(255),
  moderator_id VARCHAR(255) NOT NULL,
  action VARCHAR(32) NOT NULL,
  reason TEXT,
  duration_minutes INTEGER,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  CHECK (action IN ('delete_message', 'warn', 'mute', 'kick', 'ban'))
);
```

### `chat_webhook_events`
Webhook event log.

```sql
CREATE TABLE chat_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  event_type VARCHAR(128) NOT NULL,
  payload JSONB NOT NULL,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMP WITH TIME ZONE,
  error TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

---

## Examples

### Example 1: Direct Message

```bash
# Create direct conversation
curl -X POST http://localhost:3401/v1/conversations \
  -H "Content-Type: application/json" \
  -d '{
    "type": "direct",
    "created_by": "user123"
  }'

# Add participants
curl -X POST http://localhost:3401/v1/conversations/conv-id/participants \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user456", "role": "member"}'

# Send message
curl -X POST http://localhost:3401/v1/conversations/conv-id/messages \
  -H "Content-Type: application/json" \
  -d '{
    "sender_id": "user123",
    "content": "Hey, how are you?"
  }'
```

### Example 2: Group Chat with Mentions

```bash
# Send message with mentions
curl -X POST http://localhost:3401/v1/conversations/conv-id/messages \
  -H "Content-Type: application/json" \
  -d '{
    "sender_id": "user123",
    "content": "Hey @user456 and @user789, check this out!",
    "mentions": ["user456", "user789"]
  }'
```

### Example 3: Message Threading

```bash
# Start a thread
curl -X POST http://localhost:3401/v1/conversations/conv-id/messages \
  -H "Content-Type: application/json" \
  -d '{
    "sender_id": "user123",
    "content": "This is the main message"
  }'

# Reply in thread
curl -X POST http://localhost:3401/v1/conversations/conv-id/messages \
  -H "Content-Type: application/json" \
  -d '{
    "sender_id": "user456",
    "content": "This is a reply",
    "reply_to_id": "main-message-uuid",
    "thread_root_id": "main-message-uuid"
  }'
```

### Example 4: Reactions

```bash
# Add reaction
curl -X POST http://localhost:3401/v1/conversations/conv-id/messages/msg-id/reactions \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user123",
    "emoji": "👍"
  }'

# Remove reaction
curl -X DELETE "http://localhost:3401/v1/conversations/conv-id/messages/msg-id/reactions/👍?user_id=user123"
```

### Example 5: Pin Important Messages

```bash
# Pin a message
curl -X POST http://localhost:3401/v1/conversations/conv-id/messages/msg-id/pin \
  -H "Content-Type: application/json" \
  -d '{"pinned_by": "user123"}'

# Get pinned messages
curl http://localhost:3401/v1/conversations/conv-id/pinned
```

---

## Troubleshooting

### Messages Not Appearing

**Solution:**
- Check conversation_id is correct
- Verify participant has joined conversation
- Check for database connection issues
- Review message content length limits

### Slow Message Queries

**Solution:**
- Ensure indexes are created via `nself chat init`
- Use pagination with reasonable limits
- Consider archiving old conversations
- Add database connection pooling

### Edit Window Expired

**Solution:**
- Check `CHAT_EDIT_WINDOW_MINUTES` setting
- Increase window if needed
- Inform users of the time limit

### Attachment Upload Failures

**Solution:**
- Check `CHAT_MAX_ATTACHMENTS` limit
- Verify file sizes are reasonable
- Ensure storage service is accessible
- Review body size limits

### Rate Limiting

**Solution:**
```bash
# Increase limits
export CHAT_RATE_LIMIT_MAX=500
export CHAT_RATE_LIMIT_WINDOW_MS=60000
```

---

## License

Source-Available License

## Support

- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- Plugin Homepage: https://github.com/acamarata/nself-plugins/tree/main/plugins/chat
