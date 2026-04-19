# Chat Plugin

Complete chat and messaging system with conversations (direct, group, channel), threaded messages, reactions, read receipts, pinned messages, attachments, mentions, typing indicators, full-text search, and comprehensive moderation tools.

| Property | Value |
|----------|-------|
| **Port** | `3401` |
| **Category** | `communication` |
| **Multi-App** | `source_account_id` (UUID) |
| **Min nself** | `0.4.8` |

---

## Quick Start

```bash
nself plugin run chat init
nself plugin run chat server
```

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CHAT_PLUGIN_PORT` | `3401` | Server port |
| `CHAT_PLUGIN_HOST` | `0.0.0.0` | Server host |
| `CHAT_MAX_MESSAGE_LENGTH` | `10000` | Maximum message content length (characters) |
| `CHAT_MAX_ATTACHMENTS` | `10` | Maximum attachments per message |
| `CHAT_EDIT_WINDOW_MINUTES` | `15` | Time window for editing messages (0 = unlimited) |
| `CHAT_MAX_PARTICIPANTS` | `100` | Maximum participants per conversation |
| `CHAT_MAX_PINNED` | `50` | Maximum pinned messages per conversation |
| `CHAT_API_KEY` | - | API key for plugin authentication |
| `CHAT_RATE_LIMIT_MAX` | `200` | Max requests per window |
| `CHAT_RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window (ms) |

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (6 tables) |
| `server` | Start the HTTP API server (`-p`/`--port`, `-h`/`--host`) |
| `status` | Show plugin status and statistics |
| `conversations` | List conversations (`-u`/`--user`, `-l`/`--limit`) |
| `messages` | List messages in a conversation (`-c`/`--conversation`, `-l`/`--limit`) |
| `stats` | Show detailed statistics |

---

## REST API

All endpoints support multi-app isolation via `X-Source-Account-Id` header.

### Health & Status

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (DB) |
| `GET` | `/live` | Liveness with memory/uptime/stats |
| `GET` | `/v1/status` | Plugin status with conversation/message/user counts |

### Conversations

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/conversations` | Create conversation (body: `type`, `name?`, `description?`, `avatar_url?`, `created_by?`, `participant_ids?`, `metadata?`) |
| `GET` | `/v1/conversations` | List conversations (query: `user_id?`, `limit?`, `offset?`) |
| `GET` | `/v1/conversations/:id` | Get conversation with participants (query: `user_id?` for unread count) |
| `PUT` | `/v1/conversations/:id` | Update conversation (body: `name?`, `description?`, `avatar_url?`, `is_archived?`, `is_muted?`, `metadata?`) |
| `DELETE` | `/v1/conversations/:id` | Archive conversation (soft delete -- sets `is_archived: true`) |

### Participants

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/conversations/:id/participants` | Add participant (body: `user_id`, `role?`, `nickname?`, `notification_level?`) |
| `GET` | `/v1/conversations/:id/participants` | List active participants |
| `PUT` | `/v1/conversations/:id/participants/:userId` | Update participant (body: `role?`, `nickname?`, `muted_until?`, `notification_level?`) |
| `DELETE` | `/v1/conversations/:id/participants/:userId` | Remove participant (sets `left_at`) |

### Messages

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/conversations/:id/messages` | Send message (body: `sender_id`, `content?`, `content_type?`, `reply_to_id?`, `thread_root_id?`, `attachments?`, `mentions?`, `metadata?`) |
| `GET` | `/v1/conversations/:id/messages` | List messages (query: `limit?`, `before?` cursor) |
| `GET` | `/v1/conversations/:id/messages/:msgId` | Get single message |
| `PUT` | `/v1/conversations/:id/messages/:msgId` | Edit message within edit window (body: `content?`, `attachments?`, `mentions?`, `metadata?`; query: `sender_id`) |
| `DELETE` | `/v1/conversations/:id/messages/:msgId` | Soft delete message (query: `sender_id`) |

### Reactions

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/conversations/:id/messages/:msgId/reactions` | Add reaction (body: `user_id`, `emoji`) |
| `DELETE` | `/v1/conversations/:id/messages/:msgId/reactions/:emoji` | Remove reaction (query: `user_id`) |

### Pinned Messages

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/conversations/:id/messages/:msgId/pin` | Pin message (body: `pinned_by`) -- enforces `CHAT_MAX_PINNED` limit |
| `DELETE` | `/v1/conversations/:id/messages/:msgId/pin` | Unpin message |
| `GET` | `/v1/conversations/:id/pinned` | List pinned messages |

### Threads

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/conversations/:id/threads/:msgId` | List thread replies (query: `limit?`) |

### Read Receipts

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/conversations/:id/read` | Update read receipt (body: `user_id`, `last_read_message_id`) -- recalculates unread count |
| `GET` | `/v1/conversations/:id/unread` | Get unread count (query: `user_id`) |

### Search

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/conversations/:id/search` | Search messages (query: `content?`, `sender_id?`, `content_type?`, `has_attachments?`, `is_pinned?`, `from_date?`, `to_date?`, `limit?`, `offset?`) |

### Typing Indicators

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/conversations/:id/typing` | Send typing indicator (body: `user_id`) -- fire-and-forget |

### Moderation

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/moderate` | Take moderation action (body: `conversation_id?`, `message_id?`, `target_user_id?`, `moderator_id`, `action`, `reason?`, `duration_minutes?`) |

### Sync

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/sync` | Get updated data (query: `conversation_id?`, `user_id?`) -- returns conversations and messages |

---

## Webhook Events

| Event | Description |
|-------|-------------|
| `conversation.created` | New conversation created |
| `conversation.updated` | Conversation metadata updated |
| `conversation.archived` | Conversation archived |
| `participant.joined` | Participant joined conversation |
| `participant.left` | Participant left conversation |
| `participant.role_changed` | Participant role updated |
| `message.created` | New message sent |
| `message.updated` | Message edited |
| `message.deleted` | Message deleted |
| `message.pinned` | Message pinned |
| `message.unpinned` | Message unpinned |
| `message.reaction_added` | Reaction added to message |
| `message.reaction_removed` | Reaction removed from message |
| `moderation.action_taken` | Moderation action executed |

---

## Conversation Types

| Type | Description |
|------|-------------|
| `direct` | One-to-one private messaging |
| `group` | Multi-user group chat |
| `channel` | Broadcast channel (many readers, few writers) |

---

## Message Content Types

| Type | Description |
|------|-------------|
| `text` | Plain text message |
| `image` | Image message |
| `file` | File attachment |
| `audio` | Audio message |
| `video` | Video message |
| `system` | System-generated message (join, leave, etc.) |
| `embed` | Rich embed content |

---

## Participant Roles

| Role | Description |
|------|-------------|
| `owner` | Conversation creator with full control |
| `admin` | Administrative privileges |
| `moderator` | Can moderate messages and users |
| `member` | Standard participant |
| `guest` | Limited-access participant |

---

## Notification Levels

| Level | Description |
|-------|-------------|
| `all` | Receive all notifications |
| `mentions` | Only receive notifications when mentioned |
| `none` | No notifications |

---

## Moderation Actions

| Action | Description | Side Effect |
|--------|-------------|-------------|
| `delete_message` | Remove a message | Content replaced with `[deleted by moderator]` |
| `warn` | Issue a warning to a user | Recorded in moderation log only |
| `mute` | Temporarily mute a user | Sets `muted_until` on participant (requires `duration_minutes`) |
| `kick` | Remove a user from conversation | Sets `left_at` on participant |
| `ban` | Ban a user from conversation | Removes participant (same as kick currently) |

---

## Message Attachments

Each attachment in the `attachments` JSONB array has the following structure:

| Field | Type | Description |
|-------|------|-------------|
| `url` | `string` | Attachment URL |
| `type` | `string` | Attachment type (e.g., `image`, `file`, `audio`) |
| `size` | `number?` | File size in bytes |
| `name` | `string?` | Original filename |
| `thumbnail_url` | `string?` | Thumbnail URL for previews |

---

## Reactions

Reactions are stored as a JSONB map on each message where keys are emoji strings and values are arrays of user IDs. Example:

```json
{
  "thumbsup": ["user123", "user456"],
  "heart": ["user789"]
}
```

Adding a reaction that already exists for a user is idempotent. Removing the last user from an emoji key deletes the key entirely.

---

## Threading

Messages support two threading fields:

- `reply_to_id` -- References the specific message being replied to (inline reply)
- `thread_root_id` -- References the root message of a thread (groups all replies under one parent)

Use `GET /v1/conversations/:id/threads/:msgId` to fetch all messages in a thread by passing the root message ID.

---

## Message Editing

Messages can be edited within the configured edit window (`CHAT_EDIT_WINDOW_MINUTES`). When a message is edited:

1. The original content is preserved in `metadata.original_content`
2. The `edited_at` timestamp is set
3. The `content` field is updated to the new text

The edit window is enforced server-side. If `CHAT_EDIT_WINDOW_MINUTES` is `0`, editing is unlimited. The `sender_id` query parameter must match the original sender.

---

## Message Deletion

Deleting a message is a soft delete:

- The `deleted_at` timestamp is set
- The `content` is replaced with `[deleted]`
- The message record remains in the database for audit purposes

Moderators can delete any message via `POST /v1/moderate` with `action: "delete_message"`, which replaces content with `[deleted by moderator]`.

---

## Read Receipts & Unread Counts

When a message is sent, unread counts are incremented for all participants in the conversation except the sender. When a user marks a conversation as read via `POST /v1/conversations/:id/read`:

1. The `last_read_message_id` and `last_read_at` are updated
2. The `unread_count` is recalculated by counting messages after the read position

---

## Search

The search endpoint supports multiple filters that can be combined:

| Filter | Type | Description |
|--------|------|-------------|
| `content` | `string` | Case-insensitive substring match (`ILIKE`) |
| `sender_id` | `string` | Filter by sender |
| `content_type` | `string` | Filter by content type |
| `has_attachments` | `boolean` | Filter messages with/without attachments |
| `is_pinned` | `boolean` | Filter pinned messages |
| `from_date` | `ISO date` | Messages created on or after this date |
| `to_date` | `ISO date` | Messages created on or before this date |

---

## Database Schema

### `np_chat_conversations`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Conversation ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `type` | `VARCHAR(16)` | `direct`, `group`, `channel` |
| `name` | `VARCHAR(255)` | Conversation display name |
| `description` | `TEXT` | Conversation description |
| `avatar_url` | `TEXT` | Conversation avatar URL |
| `created_by` | `VARCHAR(255)` | User who created the conversation |
| `is_archived` | `BOOLEAN` | Whether conversation is archived |
| `is_muted` | `BOOLEAN` | Whether conversation is muted |
| `last_message_at` | `TIMESTAMPTZ` | Timestamp of last message |
| `last_message_preview` | `TEXT` | Preview of last message (first 100 chars) |
| `message_count` | `INTEGER` | Total message count |
| `member_count` | `INTEGER` | Active participant count |
| `metadata` | `JSONB` | Arbitrary metadata |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `np_chat_participants`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Participant record ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `conversation_id` | `UUID` (FK) | References `np_chat_conversations` |
| `user_id` | `VARCHAR(255)` | User ID |
| `role` | `VARCHAR(32)` | `owner`, `admin`, `moderator`, `member`, `guest` |
| `nickname` | `VARCHAR(128)` | Display nickname in this conversation |
| `muted_until` | `TIMESTAMPTZ` | Muted expiration (null = not muted) |
| `joined_at` | `TIMESTAMPTZ` | When user joined |
| `left_at` | `TIMESTAMPTZ` | When user left (null = active) |
| `last_read_message_id` | `UUID` | Last message read by this user |
| `last_read_at` | `TIMESTAMPTZ` | When user last read |
| `notification_level` | `VARCHAR(32)` | `all`, `mentions`, `none` |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

Unique constraint on `(conversation_id, user_id)`. Re-adding a removed participant clears `left_at` via upsert.

### `np_chat_messages`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Message ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `conversation_id` | `UUID` (FK) | References `np_chat_conversations` |
| `sender_id` | `VARCHAR(255)` | Message sender |
| `reply_to_id` | `UUID` (FK) | Inline reply reference |
| `thread_root_id` | `UUID` (FK) | Thread root message reference |
| `content` | `TEXT` | Message text content |
| `content_type` | `VARCHAR(32)` | `text`, `image`, `file`, `audio`, `video`, `system`, `embed` |
| `attachments` | `JSONB` | Array of attachment objects |
| `mentions` | `TEXT[]` | Array of mentioned user IDs |
| `reactions` | `JSONB` | Map of emoji to user ID arrays |
| `edited_at` | `TIMESTAMPTZ` | When message was edited |
| `deleted_at` | `TIMESTAMPTZ` | When message was soft-deleted |
| `is_pinned` | `BOOLEAN` | Whether message is pinned |
| `pinned_at` | `TIMESTAMPTZ` | When message was pinned |
| `pinned_by` | `VARCHAR(255)` | Who pinned the message |
| `metadata` | `JSONB` | Arbitrary metadata (includes `original_content` after edit) |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `np_chat_read_receipts`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Receipt ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `conversation_id` | `UUID` (FK) | References `np_chat_conversations` |
| `user_id` | `VARCHAR(255)` | User ID |
| `last_read_message_id` | `UUID` (FK) | Last read message reference |
| `last_read_at` | `TIMESTAMPTZ` | When user last read |
| `unread_count` | `INTEGER` | Number of unread messages |

Unique constraint on `(conversation_id, user_id)`. Upserted on each read receipt update.

### `np_chat_moderation_actions`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Action ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `conversation_id` | `UUID` (FK) | Target conversation |
| `message_id` | `UUID` (FK) | Target message (for `delete_message`) |
| `target_user_id` | `VARCHAR(255)` | User receiving the action |
| `moderator_id` | `VARCHAR(255)` | Moderator who took the action |
| `action` | `VARCHAR(32)` | `delete_message`, `warn`, `mute`, `kick`, `ban` |
| `reason` | `TEXT` | Reason for the action |
| `duration_minutes` | `INTEGER` | Duration for timed actions (mute) |
| `created_at` | `TIMESTAMPTZ` | Action timestamp |

### `np_chat_webhook_events`

Standard webhook event tracking table with `id`, `source_account_id`, `event_type`, `payload` (JSONB), `processed`, `processed_at`, `error`, `created_at`.

---

## Database Indexes

The schema creates the following indexes for query performance:

| Index | Columns | Notes |
|-------|---------|-------|
| `idx_chat_conversations_source_account` | `source_account_id` | Multi-app filtering |
| `idx_chat_conversations_type` | `type` | Filter by conversation type |
| `idx_chat_conversations_created_by` | `created_by` | Find conversations by creator |
| `idx_chat_conversations_created_at` | `created_at DESC` | Chronological listing |
| `idx_chat_conversations_last_message_at` | `last_message_at DESC NULLS LAST` | Sort by activity |
| `idx_chat_conversations_archived` | `is_archived` (partial) | Active conversations only |
| `idx_chat_participants_conversation` | `conversation_id` | Participant lookups |
| `idx_chat_participants_user` | `user_id` | User's conversations |
| `idx_chat_participants_active` | `(conversation_id, user_id)` (partial) | Active participants only |
| `idx_chat_messages_conversation_created` | `(conversation_id, created_at DESC)` | Message listing per conversation |
| `idx_chat_messages_sender` | `sender_id` | Messages by sender |
| `idx_chat_messages_thread_root` | `thread_root_id` (partial) | Thread lookups |
| `idx_chat_messages_reply_to` | `reply_to_id` (partial) | Reply chain lookups |
| `idx_chat_messages_pinned` | `(conversation_id, is_pinned)` (partial) | Pinned message listing |
| `idx_chat_messages_deleted` | `deleted_at` (partial) | Deleted message tracking |

---

## Troubleshooting

**Messages not appearing** -- Verify the conversation ID is correct and the sender is a participant. Check message content length against `CHAT_MAX_MESSAGE_LENGTH`.

**Edit window expired** -- The server enforces `CHAT_EDIT_WINDOW_MINUTES` from the message's `created_at`. Increase the value or set to `0` for unlimited editing.

**Participant not found** -- Participants with `left_at` set are not returned by default. Re-adding a removed participant clears `left_at` via upsert.

**Pinned message limit reached** -- The server enforces `CHAT_MAX_PINNED` per conversation. Unpin existing messages before pinning new ones.

**Unread count incorrect** -- Run `POST /v1/conversations/:id/read` with the latest `last_read_message_id` to recalculate. Unread counts are incremented on each new message and recalculated on each read receipt update.

**Search returns no results** -- Search uses case-insensitive `ILIKE` matching. Deleted messages (with `deleted_at` set) are still returned in search results. Use the `from_date` and `to_date` filters to narrow the time range.

**Reactions not updating** -- Reactions are stored as a JSONB map on the message record. Adding the same reaction twice for the same user is idempotent. The emoji in the DELETE URL must be URL-encoded.
