# nPlugins — The nself Plugin Ecosystem

Official plugin repository for [nself](https://github.com/acamarata/nself), the production-ready self-hosted backend infrastructure manager.

**59 plugins** across **13 categories** — authentication, automation, commerce, communication, content, compliance, data, development, infrastructure, integrations, media, sports, and streaming.

Every plugin provides: PostgreSQL schema with `np_` namespaced tables, REST API, CLI tools, webhook handling, and multi-app isolation via `source_account_id`.

## Quick Start

```bash
git clone https://github.com/acamarata/nself-plugins.git
cd nself-plugins

# Build shared utilities
cd shared && npm install && npm run build && cd ..

# Build any plugin
cd plugins/stripe/ts && npm install && npm run build

# Initialize database + start server
npx nself-stripe init
npx nself-stripe server --port 3001
```

### Prerequisites

- Node.js 18+
- PostgreSQL 14+
- nself v0.4.8+

### Configuration

Every plugin reads from environment variables. At minimum:

```bash
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
```

Plugin-specific variables are documented in each plugin's `plugin.json` and wiki page.

---

## Plugin Catalog

### Authentication (4 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [access-controls](plugins/access-controls/) | 3027 | 6 | Role-based and attribute-based access control (RBAC + ABAC) with policy engine |
| [auth](plugins/auth/) | 3014 | 7 | OAuth, WebAuthn/passkeys, TOTP 2FA, magic links, device-code flow |
| [idme](plugins/idme/) | 3010 | 5 | ID.me OAuth with government-grade identity verification for 7 groups |
| [vpn](plugins/vpn/) | 3200 | 8 | Multi-provider VPN management (NordVPN, PIA, Mullvad), kill switch, leak protection |

### Automation (2 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [bots](plugins/bots/) | — | 9 | Bot framework for nself-chat — commands, subscriptions, marketplace |
| [workflows](plugins/workflows/) | 3712 | 9 | Trigger-action workflow engine with conditional logic, scheduling, and cross-plugin orchestration |

### Commerce (5 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [donorbox](plugins/donorbox/) | 3005 | 7 | Donorbox donation data sync with webhook handling |
| [entitlements](plugins/entitlements/) | 3714 | 8 | Feature gating, subscription plans, usage quotas, metered billing |
| [paypal](plugins/paypal/) | 3004 | 14 | PayPal payment data sync with webhook handling |
| [shopify](plugins/shopify/) | — | 9 | Shopify store, orders, products, and inventory synchronization |
| [stripe](plugins/stripe/) | — | 23 | Stripe billing data sync — customers, subscriptions, invoices, payments |

### Communication (5 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [chat](plugins/chat/) | 3401 | 6 | Messaging with conversations, participants, and moderation |
| [invitations](plugins/invitations/) | 3402 | 4 | Invitation management with email/SMS delivery and tracking |
| [livekit](plugins/livekit/) | — | 6 | LiveKit voice/video — room management, recording, quality monitoring |
| [streaming](plugins/streaming/) | 3711 | 10 | Live broadcasting with RTMP/HLS, viewer analytics, chat integration, DVR |
| [webhooks](plugins/webhooks/) | 3403 | 4 | Outbound webhook delivery with retry logic, HMAC signing, dead-letter queue |

### Compliance (1 plugin)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [compliance](plugins/compliance/) | — | 17 | GDPR/CCPA/HIPAA/SOC2/PCI management, DSARs, consent tracking, breach notification, SIEM integration |

### Content (8 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [activity-feed](plugins/activity-feed/) | 3503 | 4 | Universal activity feed with fan-out and aggregation |
| [calendar](plugins/calendar/) | — | 6 | Calendar and event management with recurring events and iCal export |
| [cms](plugins/cms/) | 3501 | 8 | Headless CMS with content types, posts, categories, tags, versioning |
| [knowledge-base](plugins/knowledge-base/) | 3713 | 8 | Documentation, FAQ, semantic search, versioning, translations |
| [link-preview](plugins/link-preview/) | 3718 | 7 | URL metadata extraction — Open Graph, Twitter Cards, oEmbed, caching |
| [moderation](plugins/moderation/) | — | 18 | Content moderation with profanity filtering, toxicity detection, AI review, appeals |
| [social](plugins/social/) | 3502 | 7 | Posts, comments, reactions, follows, and bookmarks |
| [support](plugins/support/) | — | 9 | Helpdesk ticketing, SLA, canned responses, knowledge base |

### Data (3 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [data-operations](plugins/data-operations/) | 3306 | 11 | GDPR-compliant export/deletion, bulk import/export, cross-platform migration |
| [documents](plugins/documents/) | — | 5 | Document management with templates, versioning, and sharing |
| [geolocation](plugins/geolocation/) | 3026 | 5 | Real-time location sharing, history, geofencing, proximity queries |

### Development (2 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [github](plugins/github/) | — | 8 | GitHub repository, issue, PR, and workflow integration |
| [meetings](plugins/meetings/) | 3710 | 9 | Meeting management with room booking, Google/Outlook sync, availability |

### Infrastructure (12 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [analytics](plugins/analytics/) | 3304 | 6 | Event tracking, counters, funnels, and quota management |
| [backup](plugins/backup/) | — | 4 | PostgreSQL backup and restore automation with scheduling |
| [cdn](plugins/cdn/) | 3036 | 4 | CDN management — cache purging, signed URLs, analytics |
| [cloudflare](plugins/cloudflare/) | 3024 | 6 | Cloudflare zone, DNS, R2, cache, and analytics management |
| [feature-flags](plugins/feature-flags/) | 3305 | 5 | Feature flags with targeting rules, segments, and evaluation engine |
| [file-processing](plugins/file-processing/) | 3104 | 4 | File processing — thumbnails, optimization, virus scanning for S3/GCS/R2/B2 |
| [geocoding](plugins/geocoding/) | 3203 | 4 | Forward/reverse geocoding, place search, geofences |
| [jobs](plugins/jobs/) | — | 4 | BullMQ background job queue with priorities, scheduling, retries |
| [notifications](plugins/notifications/) | 3102 | 6 | Multi-channel notifications (email, push, SMS) with templates and preferences |
| [object-storage](plugins/object-storage/) | 3301 | 5 | Multi-provider object storage with S3-compatible API, presigned URLs, multipart |
| [realtime](plugins/realtime/) | — | 6 | Socket.io real-time server with presence, typing indicators, rooms |
| [search](plugins/search/) | — | 5 | Full-text search with PostgreSQL FTS and MeiliSearch support |

### Integrations (2 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [ai](plugins/ai/) | — | 10 | Unified AI gateway — multi-provider LLM, embeddings, semantic search, prompt templates |
| [web3](plugins/web3/) | 3715 | 12 | Blockchain integration, NFTs, token-gated access, DAO governance, decentralized identity |

### Media (12 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [content-acquisition](plugins/content-acquisition/) | 3202 | 8 | Automated content acquisition with RSS monitoring and download rules engine |
| [content-progress](plugins/content-progress/) | 3022 | 5 | Playback progress tracking — continue watching, watchlists, favorites |
| [epg](plugins/epg/) | — | 6 | Electronic program guide with XMLTV import and schedule queries |
| [media-processing](plugins/media-processing/) | 3019 | 7 | FFmpeg-based media encoding with HLS streaming support |
| [metadata-enrichment](plugins/metadata-enrichment/) | 3203 | 2 | TMDB metadata enrichment for movies and TV shows |
| [photos](plugins/photos/) | — | 5 | Photo albums with EXIF extraction, tagging, face grouping |
| [retro-gaming](plugins/retro-gaming/) | — | 6 | ROM library management, emulator cores, save states for nself-tv |
| [rom-discovery](plugins/rom-discovery/) | — | 4 | ROM metadata, multi-source scraping, download orchestration for nself-tv |
| [subtitle-manager](plugins/subtitle-manager/) | 3204 | 2 | Subtitle search and download via OpenSubtitles |
| [tmdb](plugins/tmdb/) | — | 7 | TMDB/IMDb metadata enrichment with auto-matching and review queue |
| [tokens](plugins/tokens/) | — | 5 | Secure content delivery tokens and HLS encryption key management |
| [torrent-manager](plugins/torrent-manager/) | 3201 | 8 | Torrent downloading with Transmission/qBittorrent, VPN enforcement, search |

### Sports (1 plugin)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [sports](plugins/sports/) | 3035 | 11 | Live scores, schedules, standings, rosters, player stats, real-time updates |

### Streaming (3 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [devices](plugins/devices/) | 3603 | 5 | IoT device enrollment, trust management, and command dispatch |
| [recording](plugins/recording/) | 3602 | 3 | Recording orchestration and archive management |
| [stream-gateway](plugins/stream-gateway/) | 3601 | 4 | Stream admission and governance |

---

## Architecture

### Plugin Structure

Every TypeScript plugin follows a consistent layout:

```
plugins/<name>/
├── plugin.json              # Manifest (metadata, tables, endpoints, env vars)
└── ts/
    ├── src/
    │   ├── types.ts         # TypeScript interfaces for API + database records
    │   ├── config.ts        # Environment variable loading and validation
    │   ├── client.ts        # External API client with rate limiting
    │   ├── database.ts      # PostgreSQL schema, CRUD, upsert operations
    │   ├── sync.ts          # Full and incremental data sync orchestration
    │   ├── webhooks.ts      # Inbound webhook signature verification + handlers
    │   ├── server.ts        # Fastify HTTP server with REST API routes
    │   ├── cli.ts           # Commander.js CLI with all user commands
    │   └── index.ts         # Module re-exports
    ├── package.json
    └── tsconfig.json
```

### Shared Utilities (`shared/`)

Common TypeScript library used by all plugins:

| Module | Purpose |
|--------|---------|
| `types.ts` | Core interfaces — `PluginConfig`, `SyncResult`, `WebhookEvent` |
| `logger.ts` | Colored logging with levels (debug, info, warn, error, success) |
| `database.ts` | PostgreSQL connection pool and query helpers |
| `http.ts` | HTTP client with rate limiting (`RateLimiter`) |
| `webhook.ts` | Webhook signature verification (HMAC-SHA256) |

### Standards

| Standard | Requirement |
|----------|-------------|
| **Table prefix** | All tables use `np_` prefix (e.g. `np_stripe_customers`) |
| **Multi-app isolation** | All tables include `source_account_id` column |
| **Plugin names** | lowercase-with-hyphens |
| **TypeScript** | Strict mode, `ES2022` target, `NodeNext` module resolution |
| **Imports** | `.js` extension for local imports |
| **Database** | PostgreSQL 14+, upsert pattern (`ON CONFLICT DO UPDATE`) |
| **API** | Fastify with CORS, `/health` endpoint on every server |

### Common CLI Commands

Every plugin provides at minimum:

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (create tables, indexes, views) |
| `server` | Start HTTP API server on configured port |
| `sync` | Sync data from external service (where applicable) |
| `status` | Show plugin status and statistics |

### Common API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check (always returns 200) |
| `POST` | `/webhook` | Inbound webhook receiver (signature verified) |
| `POST` | `/api/sync` | Trigger data sync |
| `GET` | `/api/status` | Plugin status and statistics |

Resource-specific endpoints are documented per-plugin in the [wiki](https://github.com/acamarata/nself-plugins/wiki).

---

## Development

### Building

```bash
# Shared utilities (build once)
cd shared && npm install && npm run build

# Any plugin
cd plugins/<name>/ts
npm install
npm run build      # Compile TypeScript
npm run typecheck  # Type check only (no output)
npm run watch      # Watch mode
npm run dev        # Development server (tsx)
npm start          # Production server
```

### Adding a New Plugin

1. Create `plugins/<name>/plugin.json` with full manifest
2. Create `plugins/<name>/ts/` with standard file structure
3. All tables MUST use `np_` prefix and include `source_account_id`
4. Add to `registry.json`
5. Add wiki documentation to `.wiki/plugins/`

See [CONTRIBUTING.md](.wiki/CONTRIBUTING.md) for full guidelines.

### Registry

The central `registry.json` describes all plugins and is served at:

- **Primary**: `https://plugins.nself.org/registry.json` (Cloudflare Worker)
- **Fallback**: `https://raw.githubusercontent.com/acamarata/nself-plugins/main/registry.json`

### Releasing

```bash
# Bump versions in plugin.json and registry.json
git add . && git commit -m "chore: Release v1.x.y"
git tag -a v1.x.y -m "Release v1.x.y"
git push origin main && git push origin v1.x.y
# GitHub Actions handles the rest
```

---

## Repository Structure

```
nself-plugins/
├── plugins/            # 59 plugin directories
├── shared/             # Common TypeScript utilities
├── .wiki/              # Public documentation (syncs to GitHub Wiki)
├── .github/            # CI/CD workflows
├── .workers/           # Cloudflare Worker (registry API)
├── registry.json       # Plugin registry
├── registry-schema.json
├── README.md
└── LICENSE
```

Root policy: planning artifacts stay in private dotfile directories (gitignored). Public docs go in `.wiki/`. Legacy `docs/` is retired.

## License

Source-Available License — See [LICENSE](LICENSE)

## Links

- [Documentation Wiki](https://github.com/acamarata/nself-plugins/wiki)
- [Contributing Guide](.wiki/CONTRIBUTING.md)
- [Development Guide](.wiki/DEVELOPMENT.md)
- [nself Main Repository](https://github.com/acamarata/nself)
- [Issues](https://github.com/acamarata/nself-plugins/issues)
