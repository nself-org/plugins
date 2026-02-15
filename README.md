# nPlugins — The nself Plugin Ecosystem

Official plugin repository for [nself](https://github.com/acamarata/nself), the production-ready self-hosted backend infrastructure manager.

**64 plugins** across **16 categories** — admin, authentication, automation, commerce, communication, compliance, content, data, development, infrastructure, integrations, media, monitoring, networking, sports, and streaming.

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

### Admin (1 plugin)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [admin-api](plugins/admin-api/) | 3212 | 0 | Admin API service providing aggregated metrics, system health, session counts, storage breakdown, and real-time dashboard endpoints |

### Authentication (4 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [access-controls](plugins/access-controls/) | 3027 | 0 | Role-based and attribute-based access control (RBAC + ABAC) with policy engine |
| [auth](plugins/auth/) | 3014 | 0 | Advanced authentication: OAuth, WebAuthn/passkeys, TOTP 2FA, magic links, device-code flow |
| [idme](plugins/idme/) | 3010 | 0 | ID.me OAuth authentication with government-grade identity verification for 7 groups |
| [vpn](plugins/vpn/) | 3200 | 0 | Multi-provider VPN management (3 VPN providers) and torrent downloads with P2P optimization, server carousel, kill switch, and leak protection |

### Automation (2 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [bots](plugins/bots/) | 3103 | 0 | Bot framework for nself-chat - commands, subscriptions, marketplace, API keys, reviews |
| [workflows](plugins/workflows/) | 3712 | 0 | Automation engine providing trigger-action workflow chains, conditional logic, scheduled tasks, webhook integrations, and cross-plugin orchestration |

### Commerce (5 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [donorbox](plugins/donorbox/) | 3005 | 0 | Donorbox donation data sync with webhook handling |
| [entitlements](plugins/entitlements/) | 3714 | 0 | Feature gating, subscription plan management, usage quota tracking, and metered billing |
| [paypal](plugins/paypal/) | 3004 | 0 | PayPal payment data sync with webhook handling |
| [shopify](plugins/shopify/) | 3003 | 0 | Shopify store, orders, and product synchronization |
| [stripe](plugins/stripe/) | 3001 | 0 | Stripe billing data sync with webhook handling |

### Communication (5 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [chat](plugins/chat/) | 3401 | 0 | Chat and messaging data management with conversation, messages, participants, and moderation |
| [invitations](plugins/invitations/) | 3402 | 0 | Invitation management system with email/SMS delivery and tracking |
| [livekit](plugins/livekit/) | 3107 | 0 | LiveKit voice/video infrastructure - room management, participant tracking, recording/egress, quality monitoring |
| [streaming](plugins/streaming/) | 3711 | 0 | Live streaming and broadcasting with RTMP/HLS, viewer analytics, chat integration, multi-quality streams, DVR playback, and moderation |
| [webhooks](plugins/webhooks/) | 3403 | 0 | Outbound webhook delivery service with retry logic, HMAC signing, and dead-letter queue |

### Compliance (1 plugin)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [compliance](plugins/compliance/) | 3211 | 0 | Comprehensive compliance and audit platform with GDPR/CCPA/HIPAA/SOC2/PCI management, DSARs, consent tracking, data retention, breach notification, immutable audit logging, SIEM integration, and compliance reporting |

### Content (8 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [activity-feed](plugins/activity-feed/) | 3209 | 0 | Universal activity feed system with fan-out-on-read/write, aggregation, and subscriptions |
| [calendar](plugins/calendar/) | 3105 | 0 | Calendar and event management with recurring events, iCal export, and RSVP tracking |
| [cms](plugins/cms/) | 3501 | 0 | Headless CMS plugin with content types, posts, categories, tags, and versioning |
| [knowledge-base](plugins/knowledge-base/) | 3713 | 0 | Knowledge base with documentation, FAQ, semantic search, versioning, translations, and analytics |
| [link-preview](plugins/link-preview/) | 3718 | 0 | URL metadata extraction with Open Graph, Twitter Cards, oEmbed support, custom previews, and caching |
| [moderation](plugins/moderation/) | 3208 | 0 | Unified content moderation platform with profanity filtering, toxicity detection, AI-powered review, rule-based policies, automated actions, manual review workflows, user strikes, and appeals management |
| [social](plugins/social/) | 3502 | 0 | Universal social features plugin with posts, comments, reactions, follows, and bookmarks |
| [support](plugins/support/) | 3111 | 0 | Helpdesk and customer support for nself-chat - ticketing, SLA, canned responses, knowledge base, analytics |

### Data (2 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [documents](plugins/documents/) | 3106 | 0 | Document management and generation service with templates, versioning, and sharing |
| [geolocation](plugins/geolocation/) | 3026 | 0 | Real-time location sharing, history tracking, geofencing, and proximity queries |

### Development (2 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [github](plugins/github/) | 3002 | 0 | GitHub repository, issue, and workflow integration |
| [meetings](plugins/meetings/) | 3710 | 0 | Calendar integration and meeting management with room booking, Google/Outlook sync, recurring meetings, and availability tracking |

### Infrastructure (12 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [analytics](plugins/analytics/) | 3206 | 0 | Event tracking, counters, funnels, and quota management analytics engine |
| [backup](plugins/backup/) | 3210 | 0 | PostgreSQL backup and restore automation with scheduling |
| [cdn](plugins/cdn/) | 3036 | 0 | CDN management and integration plugin - cache purging, signed URLs, analytics |
| [cloudflare](plugins/cloudflare/) | 3024 | 0 | Cloudflare zone, DNS, R2, cache, and analytics management |
| [feature-flags](plugins/feature-flags/) | 3207 | 0 | Feature flags service with targeting rules, segments, and evaluation engine |
| [file-processing](plugins/file-processing/) | 3104 | 0 | File processing with thumbnails, optimization, and virus scanning for MinIO/S3/GCS/R2/B2/Azure |
| [geocoding](plugins/geocoding/) | 3203 | 0 | Geocoding and location services plugin - forward/reverse geocoding, place search, geofences |
| [jobs](plugins/jobs/) | 3105 | 0 | BullMQ background job queue with priorities, scheduling, retries, and BullBoard dashboard |
| [notifications](plugins/notifications/) | 3102 | 0 | Multi-channel notifications (email, push, SMS) with templates, preferences, and delivery tracking |
| [object-storage](plugins/object-storage/) | 3301 | 0 | Multi-provider object storage with S3-compatible API, local storage, presigned URLs, and multipart uploads |
| [realtime](plugins/realtime/) | 3109 | 0 | Socket.io real-time server with presence tracking, typing indicators, and room management |
| [search](plugins/search/) | 3110 | 0 | Full-text search engine with PostgreSQL FTS and MeiliSearch support |

### Integrations (2 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [ai](plugins/ai/) | 3101 | 0 | Unified AI gateway with multi-provider LLM support, embeddings, semantic search, prompt templates, and usage tracking |
| [web3](plugins/web3/) | 3112 | 0 | Blockchain integration, NFT support, token-gated access, DAO governance, and decentralized identity |

### Media (13 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [content-acquisition](plugins/content-acquisition/) | 3202 | 0 | Automated content acquisition with RSS monitoring, release calendar, and download rules engine |
| [content-progress](plugins/content-progress/) | 3022 | 0 | Track video, audio, and content playback progress with continue watching, watchlists, and favorites |
| [epg](plugins/epg/) | 3031 | 0 | Electronic program guide with XMLTV import, channel management, and schedule queries |
| [game-metadata](plugins/game-metadata/) | 3211 | 0 | Game metadata service with IGDB integration, ROM hash matching, tier requirements, and artwork management |
| [media-processing](plugins/media-processing/) | 3019 | 0 | FFmpeg-based media encoding and processing with HLS streaming support |
| [photos](plugins/photos/) | 3108 | 0 | Photo album management with EXIF extraction, tagging, face grouping, and thumbnails |
| [podcast](plugins/podcast/) | 3210 | 0 | Podcast service with RSS feed parsing, episode management, playback position sync, and subscription management |
| [retro-gaming](plugins/retro-gaming/) | 3033 | 0 | Retro gaming ROM library management, emulator core serving, save state synchronization, play sessions, and controller configuration for nself-tv |
| [rom-discovery](plugins/rom-discovery/) | 3034 | 0 | ROM metadata database, search, discovery, automated download orchestration, and multi-source scraping for nself-tv |
| [subtitle-manager](plugins/subtitle-manager/) | 3204 | 0 | Subtitle search, download, and sync verification via OpenSubtitles |
| [tmdb](plugins/tmdb/) | 3032 | 0 | Comprehensive media metadata enrichment from TMDB/IMDb/TVDB/MusicBrainz with auto-matching, manual review queue, and multi-provider support |
| [tokens](plugins/tokens/) | 3107 | 0 | Secure content delivery tokens, HLS encryption key management, and entitlement checks |
| [torrent-manager](plugins/torrent-manager/) | 3201 | 0 | Torrent downloading with Transmission/qBittorrent integration, multi-source search, seeding policies, and VPN enforcement |

### Monitoring (1 plugin)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [observability](plugins/observability/) | 3215 | 0 | Unified observability service with health probes, watchdog timers, service auto-discovery, and systemd integration |

### Networking (2 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [ddns](plugins/ddns/) | 3217 | 0 | Dynamic DNS updater with multi-provider support and external IP monitoring |
| [mdns](plugins/mdns/) | 3216 | 0 | mDNS/Bonjour service discovery for zero-config LAN advertising |

### Sports (1 plugin)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [sports](plugins/sports/) | 3035 | 0 | Comprehensive sports data plugin with live scores, schedules, standings, team rosters, player stats, and real-time game updates |

### Streaming (3 plugins)

| Plugin | Port | Tables | Description |
|--------|------|--------|-------------|
| [devices](plugins/devices/) | 3603 | 0 | IoT device enrollment, trust management, and command dispatch service. |
| [recording](plugins/recording/) | 3602 | 0 | Recording orchestration and archive management service. |
| [stream-gateway](plugins/stream-gateway/) | 3601 | 0 | Stream admission and governance service. |

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
