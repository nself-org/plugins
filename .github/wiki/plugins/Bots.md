# Bots Plugin

Bot framework for nself-chat with command registration, event subscriptions, marketplace with ratings and reviews, API key management, and workspace installations. Build custom bots, integrations, and official platform bots.

| Property | Value |
|----------|-------|
| **Port** | `3708` |
| **Category** | `automation` |
| **Multi-App** | `source_account_id` (UUID) |
| **Min nself** | `0.4.8` |

---

## Quick Start

```bash
nself plugin run bots init
nself plugin run bots server
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
| `BOTS_PLUGIN_PORT` | `3708` | Server port |
| `BOTS_PLUGIN_HOST` | `0.0.0.0` | Server host |
| `BOTS_API_KEY` | - | API key for plugin authentication |
| `BOTS_RATE_LIMIT_MAX` | `200` | Max requests per window |
| `BOTS_RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window (ms) |
| `BOTS_WEBHOOK_SECRET` | - | Secret for verifying incoming webhooks |

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (9 tables) |
| `server` | Start the HTTP API server (`-p`/`--port`) |
| `status` | Show bot/command/subscription/installation counts |
| `bots:create` | Create a bot (`<name>` `<username>`, `-d`/`--description`, `-o`/`--owner`) |
| `bots:list` | List bots (`-o`/`--owner`, `--public`, `-l`/`--limit`) |
| `bots:info` | Show bot details (`<botId>`) |
| `bots:delete` | Delete a bot (`<botId>`) |
| `bots:commands:register` | Register a command (`<botId>` `<command>` `<description>`, `-u`/`--usage`) |
| `bots:commands:list` | List bot commands (`<botId>`) |
| `marketplace:search` | Search marketplace (`-c`/`--category`, `--verified`, `-s`/`--sort`, `-l`/`--limit`) |
| `bots:token:generate` | Generate API key for a bot (`<botId>`, `-n`/`--name`, `-s`/`--scopes`) |
| `health` | Check server health |

---

## REST API

### Health & Status

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (DB) |
| `GET` | `/live` | Liveness with memory/uptime |

### Bots

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/bots` | Create bot (body: `name`, `username`, `description?`, `avatar_url?`, `bot_type?`, `owner_id?`, `is_public?`, `category?`, `tags?`, `metadata?`) |
| `GET` | `/api/bots` | List bots (query: `owner_id?`, `is_public?`, `limit?`, `offset?`) |
| `GET` | `/api/bots/:botId` | Get bot details |
| `PUT` | `/api/bots/:botId` | Update bot |
| `DELETE` | `/api/bots/:botId` | Delete bot |

### Commands

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/bots/:botId/commands` | Register command (body: `command`, `description`, `usage?`, `command_type?`, `scope?`, `parameters?`, `enabled?`) |
| `GET` | `/api/bots/:botId/commands` | List bot commands |
| `DELETE` | `/api/bots/:botId/commands/:commandId` | Delete command |

### Subscriptions

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/bots/:botId/subscriptions` | Create subscription (body: `event_type`, `webhook_url?`, `method?`, `filters?`, `enabled?`) |
| `GET` | `/api/bots/:botId/subscriptions` | List bot subscriptions |
| `DELETE` | `/api/bots/:botId/subscriptions/:subId` | Delete subscription |

### Workspace Installations

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/workspaces/:workspaceId/bots/:botId/install` | Install bot in workspace (body: `installed_by`, `permissions?`, `config?`) |
| `DELETE` | `/api/workspaces/:workspaceId/bots/:botId/uninstall` | Uninstall bot from workspace |
| `GET` | `/api/workspaces/:workspaceId/bots` | List installed bots in workspace |

### Marketplace

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/marketplace/bots` | Search marketplace (query: `query?`, `category?`, `verified?`, `sort?`, `limit?`, `offset?`) |
| `GET` | `/api/marketplace/bots/:botId` | Get marketplace listing |
| `GET` | `/api/marketplace/bots/:botId/reviews` | Get bot reviews (query: `limit?`, `offset?`) |
| `POST` | `/api/marketplace/bots/:botId/reviews` | Submit review (body: `user_id`, `rating`, `title?`, `body?`) |

### Messages & Interactions

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/bot/messages` | Send message as bot (body: `bot_id`, `conversation_id`, `content`, `content_type?`, `metadata?`) |
| `POST` | `/api/bot/interactions/:interactionId/respond` | Respond to interaction (body: `response_data`, `ephemeral?`) |

### API Keys

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/bots/:botId/api-keys` | Create API key (body: `name`, `scopes?`, `expires_at?`) -- returns key only once |
| `DELETE` | `/api/bots/:botId/api-keys/:keyId` | Revoke API key |

### Webhooks

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/webhook` | Receive incoming webhook events |

---

## Webhook Events

| Event | Description |
|-------|-------------|
| `bot.created` | New bot registered |
| `bot.updated` | Bot settings updated |
| `bot.deleted` | Bot removed |
| `bot.installed` | Bot installed in workspace |
| `bot.uninstalled` | Bot uninstalled from workspace |
| `bot.command.registered` | New command registered |
| `bot.message.received` | Bot received a message |
| `bot.interaction.received` | Bot received an interaction (button click, form submit) |
| `bot.review.submitted` | New marketplace review submitted |

---

## Bot Types

| Type | Description |
|------|-------------|
| `custom` | User-built bot for specific use cases |
| `integration` | Third-party service integration (e.g., GitHub, Jira) |
| `official` | Platform-provided official bot |

---

## Command System

### Command Types

| Type | Description |
|------|-------------|
| `message` | Triggered by text messages matching the command |
| `slash` | Triggered by `/command` syntax |
| `context_menu` | Triggered from right-click or context menus |

### Command Scope

| Scope | Description |
|-------|-------------|
| `all` | Available in all contexts |
| `dm` | Direct messages only |
| `channel` | Channel conversations only |

### Command Parameters

Commands can define typed parameters:

```json
{
  "command": "remind",
  "description": "Set a reminder",
  "parameters": [
    { "name": "message", "type": "string", "required": true, "description": "Reminder text" },
    { "name": "time", "type": "string", "required": true, "description": "When to remind (e.g., '5m', '1h', 'tomorrow')" }
  ]
}
```

---

## Subscription System

Bots subscribe to events and receive notifications via webhook or polling.

| Method | Description |
|--------|-------------|
| `webhook` | Events are POSTed to the bot's `webhook_url` |
| `polling` | Bot polls for new events via API |

Subscriptions support `filters` (JSONB) to narrow which events are delivered (e.g., only messages in specific channels).

---

## Marketplace

The marketplace enables bot discovery with search, categories, ratings, and reviews.

### Search Sort Options

| Sort | Description |
|------|-------------|
| `installs` | Most installed bots first |
| `rating` | Highest rated bots first |
| `recent` | Most recently created first |

### Reviews

Reviews include a 1-5 star rating. When a review is submitted, the bot's `average_rating` and `review_count` are automatically recalculated using an aggregate query.

---

## API Keys

Bot API keys are generated with the `nbot_` prefix. The raw key is returned only once at creation time. Subsequent lookups use SHA-256 hashed values. Keys support scoped permissions and optional expiration.

---

## Message Types

| Type | Description |
|------|-------------|
| `text` | Plain text message |
| `card` | Rich card with title, description, image |
| `form` | Interactive form with input fields |
| `button_group` | Set of action buttons |
| `embed` | Rich embed (like Discord embeds) |

---

## Interaction Types

| Type | Description |
|------|-------------|
| `button_click` | User clicked a button |
| `form_submit` | User submitted a form |
| `menu_select` | User selected from a dropdown menu |
| `modal_submit` | User submitted a modal dialog |

---

## Database Schema

### `nchat_bots`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Bot ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `name` | `VARCHAR(255)` | Bot display name |
| `username` | `VARCHAR(100)` | Unique bot username |
| `description` | `TEXT` | Bot description |
| `avatar_url` | `TEXT` | Bot avatar URL |
| `bot_type` | `VARCHAR(50)` | `custom`, `integration`, `official` |
| `owner_id` | `VARCHAR(255)` | Bot owner user ID |
| `bot_token` | `VARCHAR(255)` | Bot authentication token (`nbot_` prefix) |
| `is_public` | `BOOLEAN` | Whether listed in marketplace |
| `is_verified` | `BOOLEAN` | Verified by platform |
| `category` | `VARCHAR(100)` | Marketplace category |
| `tags` | `TEXT[]` | Searchable tags |
| `install_count` | `INTEGER` | Total installations |
| `average_rating` | `DECIMAL(3,2)` | Average review rating |
| `review_count` | `INTEGER` | Total reviews |
| `rate_limit_rpm` | `INTEGER` | Rate limit: requests per minute |
| `rate_limit_rpd` | `INTEGER` | Rate limit: requests per day |
| `status` | `VARCHAR(50)` | `active`, `suspended`, `disabled` |
| `metadata` | `JSONB` | Arbitrary metadata |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `nchat_bot_commands`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Command ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `bot_id` | `UUID` (FK) | References `nchat_bots` |
| `command` | `VARCHAR(100)` | Command name/trigger |
| `description` | `TEXT` | Command description |
| `usage` | `TEXT` | Usage help text |
| `command_type` | `VARCHAR(50)` | `message`, `slash`, `context_menu` |
| `scope` | `VARCHAR(50)` | `all`, `dm`, `channel` |
| `parameters` | `JSONB` | Command parameters definition |
| `enabled` | `BOOLEAN` | Whether command is active |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `nchat_bot_subscriptions`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Subscription ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `bot_id` | `UUID` (FK) | References `nchat_bots` |
| `event_type` | `VARCHAR(128)` | Event to subscribe to |
| `webhook_url` | `TEXT` | Delivery URL |
| `method` | `VARCHAR(20)` | `webhook` or `polling` |
| `filters` | `JSONB` | Event filters |
| `enabled` | `BOOLEAN` | Whether subscription is active |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `nchat_bot_installations`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Installation ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `bot_id` | `UUID` (FK) | References `nchat_bots` |
| `workspace_id` | `VARCHAR(255)` | Workspace where installed |
| `installed_by` | `VARCHAR(255)` | User who installed |
| `permissions` | `JSONB` | Granted permissions |
| `config` | `JSONB` | Installation config |
| `access_token_encrypted` | `TEXT` | Encrypted workspace access token |
| `refresh_token_encrypted` | `TEXT` | Encrypted refresh token |
| `is_active` | `BOOLEAN` | Whether installation is active |
| `installed_at` | `TIMESTAMPTZ` | Installation time |
| `uninstalled_at` | `TIMESTAMPTZ` | Uninstallation time |

### `nchat_bot_messages`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Message ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `bot_id` | `UUID` (FK) | References `nchat_bots` |
| `conversation_id` | `VARCHAR(255)` | Target conversation |
| `content` | `TEXT` | Message content |
| `content_type` | `VARCHAR(50)` | `text`, `card`, `form`, `button_group`, `embed` |
| `metadata` | `JSONB` | Message metadata |
| `created_at` | `TIMESTAMPTZ` | Message timestamp |

### `nchat_bot_interactions`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Interaction ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `bot_id` | `UUID` (FK) | References `nchat_bots` |
| `user_id` | `VARCHAR(255)` | User who triggered interaction |
| `interaction_type` | `VARCHAR(50)` | `button_click`, `form_submit`, `menu_select`, `modal_submit` |
| `interaction_data` | `JSONB` | Interaction payload |
| `response_data` | `JSONB` | Bot's response |
| `responded_at` | `TIMESTAMPTZ` | When bot responded |
| `created_at` | `TIMESTAMPTZ` | Interaction timestamp |

### `nchat_bot_reviews`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Review ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `bot_id` | `UUID` (FK) | References `nchat_bots` |
| `user_id` | `VARCHAR(255)` | Reviewer user ID |
| `rating` | `INTEGER` | 1-5 star rating |
| `title` | `VARCHAR(255)` | Review title |
| `body` | `TEXT` | Review body |
| `created_at` | `TIMESTAMPTZ` | Review timestamp |

### `nchat_bot_api_keys`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | API key record ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `bot_id` | `UUID` (FK) | References `nchat_bots` |
| `name` | `VARCHAR(255)` | Key name/label |
| `key_hash` | `VARCHAR(128)` | SHA-256 hash of the key |
| `key_prefix` | `VARCHAR(20)` | First characters of key (for identification) |
| `scopes` | `TEXT[]` | Permitted scopes |
| `last_used_at` | `TIMESTAMPTZ` | Last usage timestamp |
| `expires_at` | `TIMESTAMPTZ` | Optional expiration |
| `revoked_at` | `TIMESTAMPTZ` | Revocation timestamp |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `nchat_bots_webhook_events`

Standard webhook event tracking table with `id`, `source_account_id`, `event_type`, `payload` (JSONB), `processed`, `processed_at`, `error`, `created_at`.

---

## Troubleshooting

**Bot token not working** -- Bot tokens use the `nbot_` prefix and are returned only at creation time. If lost, generate a new API key via `POST /api/bots/:botId/api-keys`.

**Commands not triggering** -- Verify the command is `enabled: true` and the `scope` matches the conversation type. Check that the bot is installed in the workspace.

**Webhook events not delivered** -- Confirm the subscription's `webhook_url` is reachable and the subscription is `enabled: true`. Check `nchat_bots_webhook_events` for delivery errors.

**Marketplace search returns empty** -- Only bots with `is_public: true` appear in marketplace search. Verify the bot exists and is public.

**Review rating not updating** -- The `average_rating` on `nchat_bots` is recalculated via aggregate query on each new review. Check that the review was successfully inserted.
