# nself Plugins Wiki Home

This wiki is the **SPORT** for this repository: **Single Point of Reference and Truth**.

All public docs are authored in `/.wiki` and synced to GitHub Wiki via `.github/workflows/wiki-sync.yml`.

## Start Here

- [[Installation]]
- [[Quick Start|getting-started/Quick-Start]]
- [[Commands|COMMANDS]]
- [[Repository Structure|REPOSITORY-STRUCTURE]]

## Top-Level Categories

### Getting Started

- [[Installation]]
- [[Quick Start|getting-started/Quick-Start]]
- [[Configuration|guides/Configuration]]

### Commands (Complete Reference)

- [[Commands|COMMANDS]]
- [[File Processing Commands|commands/File-Processing]]
- [[GitHub Commands|commands/GitHub]]
- [[ID.me Commands|commands/IDme]]
- [[Jobs Commands|commands/Jobs]]
- [[Notifications Commands|commands/Notifications]]
- [[Realtime Commands|commands/Realtime]]
- [[Shopify Commands|commands/Shopify]]
- [[Stripe Commands|commands/Stripe]]

Command pages include action/subcommand syntax, argument shapes, and option flags from source.

### Plugin Documentation

See [[All Plugins|#all-plugins-62-total]] below for the complete catalog of 62 plugins organized by category.

**Popular Plugins:**
- [[Stripe|plugins/Stripe]] - Payment processing and subscription management
- [[GitHub|plugins/GitHub]] - Repository, issue, and workflow integration
- [[Shopify|plugins/Shopify]] - E-commerce store synchronization
- [[AI|plugins/AI]] - Multi-provider LLM gateway with embeddings and semantic search
- [[Jobs|plugins/Jobs]] - Background job queue with BullMQ
- [[Notifications|plugins/Notifications]] - Multi-channel notification delivery
- [[Auth|plugins/Auth]] - Advanced authentication with OAuth, WebAuthn, TOTP 2FA
- [[File Processing|plugins/FileProcessing]] - Image/video processing and optimization
- [[Realtime|plugins/Realtime]] - WebSocket server with presence tracking
- [[Analytics|plugins/Analytics]] - Event tracking and funnel analytics

### Architecture and API

- [[Plugin System|architecture/Plugin-System]]
- [[REST API|api/REST-API]]

### Engineering Guides

- [[Plugin Development|DEVELOPMENT]]
- [[TypeScript Plugin Guide|TYPESCRIPT_PLUGIN_GUIDE]]
- [[Multi-App Setup|guides/Multi-App-Setup]] 🆕 **All plugins support multi-app isolation**
- [[Deployment|guides/Deployment]]
- [[Migration|guides/Migration]]
- [[Best Practices|guides/Best-Practices]]
- [[Troubleshooting FAQ|troubleshooting/FAQ]]

### Governance and Reference

- [[Repository Structure|REPOSITORY-STRUCTURE]]
- [[Security]]
- [[Contributing|CONTRIBUTING]]
- [[Planned Plugins|PLANNED]]
- [[Changelog|CHANGELOG]]
- [[License]]

## Root Structure Policy (Canonical)

Root should remain intentionally minimal:

- AI agent directories (private, gitignored)
- `.github/`
- `.wiki/`
- `plugins/`
- `shared/`
- `registry.json`
- `registry-schema.json`
- `README.md`
- `LICENSE`
- required meta files (for example `.gitignore`)

Allowed infrastructure exception:

- `.workers/` for registry publishing.

Legacy `docs/` is retired. Public docs belong in `/.wiki` only.

## SPORT Rules

1. If behavior changes, docs must be updated in `/.wiki` in the same change set.
2. Commands in `COMMANDS.md` and `commands/*.md` must match action/CLI source files.
3. Any drift between code and docs is treated as a defect.

## All Plugins (62 Total)

| Plugin | Port | Category | Description |
|--------|------|----------|-------------|
| [access-controls](plugins/Access-Controls) | N/A | authentication | Role-based and attribute-based access control (RBAC + ABAC) with policy engine |
| [auth](plugins/Auth) | N/A | authentication | Advanced authentication: OAuth, WebAuthn/passkeys, TOTP 2FA, magic links, device-code flow |
| [idme](plugins/Idme) | N/A | authentication | ID.me OAuth authentication with government-grade identity verification for 7 groups |
| [bots](plugins/Bots) | N/A | automation | Bot framework for nself-chat - commands, subscriptions, marketplace, API keys, reviews |
| [workflows](plugins/Workflows) | N/A | automation | Automation engine providing trigger-action workflow chains, conditional logic, scheduled tasks, webhook integrations, and cross-plugin orchestration |
| [donorbox](plugins/Donorbox) | N/A | commerce | Donorbox donation data sync with webhook handling |
| [entitlements](plugins/Entitlements) | N/A | commerce | Feature gating, subscription plan management, usage quota tracking, and metered billing |
| [paypal](plugins/Paypal) | N/A | commerce | PayPal payment data sync with webhook handling |
| [shopify](plugins/Shopify) | N/A | commerce | Shopify store, orders, and product synchronization |
| [stripe](plugins/Stripe) | N/A | commerce | Stripe billing data sync with webhook handling |
| [chat](plugins/Chat) | N/A | communication | Chat and messaging data management with conversation, messages, participants, and moderation |
| [invitations](plugins/Invitations) | N/A | communication | Invitation management system with email/SMS delivery and tracking |
| [livekit](plugins/Livekit) | N/A | communication | LiveKit voice/video infrastructure - room management, participant tracking, recording/egress, quality monitoring |
| [streaming](plugins/Streaming) | N/A | communication | Live streaming and broadcasting with RTMP/HLS, viewer analytics, chat integration, multi-quality streams, DVR playback, and moderation |
| [webhooks](plugins/Webhooks) | N/A | communication | Outbound webhook delivery service with retry logic, HMAC signing, and dead-letter queue |
| [compliance](plugins/Compliance) | N/A | compliance | Comprehensive compliance and audit platform with GDPR/CCPA/HIPAA/SOC2/PCI management, DSARs, consent tracking, data retention, breach notification, immutable audit logging, SIEM integration, and compliance reporting |
| [activity-feed](plugins/Activity-Feed) | N/A | content | Universal activity feed system with fan-out-on-read/write, aggregation, and subscriptions |
| [calendar](plugins/Calendar) | N/A | content | Calendar and event management with recurring events, iCal export, and RSVP tracking |
| [cms](plugins/Cms) | N/A | content | Headless CMS plugin with content types, posts, categories, tags, and versioning |
| [knowledge-base](plugins/Knowledge-Base) | N/A | content | Knowledge base with documentation, FAQ, semantic search, versioning, translations, and analytics |
| [link-preview](plugins/Link-Preview) | N/A | content | URL metadata extraction with Open Graph, Twitter Cards, oEmbed support, custom previews, and caching |
| [moderation](plugins/Moderation) | N/A | content | Unified content moderation platform with profanity filtering, toxicity detection, AI-powered review, rule-based policies, automated actions, manual review workflows, user strikes, and appeals management |
| [social](plugins/Social) | N/A | content | Universal social features plugin with posts, comments, reactions, follows, and bookmarks |
| [support](plugins/Support) | N/A | content | Helpdesk and customer support for nself-chat - ticketing, SLA, canned responses, knowledge base, analytics |
| [data-operations](plugins/Data-Operations) | N/A | data | Comprehensive data operations platform with GDPR-compliant export/deletion, bulk import/export, cross-platform migration, backup/restore, and data portability |
| [documents](plugins/Documents) | N/A | data | Document management and generation service with templates, versioning, and sharing |
| [geolocation](plugins/Geolocation) | N/A | data | Real-time location sharing, history tracking, geofencing, and proximity queries |
| [github](plugins/Github) | N/A | development | GitHub repository, issue, and workflow integration |
| [meetings](plugins/Meetings) | N/A | development | Calendar integration and meeting management with room booking, Google/Outlook sync, recurring meetings, and availability tracking |
| [admin-api](plugins/Admin-Api) | 3214 | infrastructure | Comprehensive admin dashboard API for system health, user management, and metrics |
| [analytics](plugins/Analytics) | N/A | infrastructure | Event tracking, counters, funnels, and quota management analytics engine |
| [backup](plugins/Backup) | N/A | infrastructure | PostgreSQL backup and restore automation with scheduling |
| [cdn](plugins/Cdn) | N/A | infrastructure | CDN management and integration plugin - cache purging, signed URLs, analytics |
| [cloudflare](plugins/Cloudflare) | N/A | infrastructure | Cloudflare zone, DNS, R2, cache, and analytics management |
| [feature-flags](plugins/Feature-Flags) | N/A | infrastructure | Feature flags service with targeting rules, segments, and evaluation engine |
| [file-processing](plugins/File-Processing) | N/A | infrastructure | File processing with thumbnails, optimization, and virus scanning for MinIO/S3/GCS/R2/B2/Azure |
| [geocoding](plugins/Geocoding) | N/A | infrastructure | Geocoding and location services plugin - forward/reverse geocoding, place search, geofences |
| [jobs](plugins/Jobs) | N/A | infrastructure | BullMQ background job queue with priorities, scheduling, retries, and BullBoard dashboard |
| [notifications](plugins/Notifications) | N/A | infrastructure | Multi-channel notifications (email, push, SMS) with templates, preferences, and delivery tracking |
| [object-storage](plugins/Object-Storage) | N/A | infrastructure | Multi-provider object storage with S3-compatible API, local storage, presigned URLs, and multipart uploads |
| [observability](plugins/Observability) | 3215 | infrastructure | Unified observability platform with Prometheus metrics, structured logging to Loki, and distributed tracing to Tempo |
| [realtime](plugins/Realtime) | N/A | infrastructure | Socket.io real-time server with presence tracking, typing indicators, and room management |
| [search](plugins/Search) | N/A | infrastructure | Full-text search engine with PostgreSQL FTS and MeiliSearch support |
| [vpn](plugins/Vpn) | N/A | infrastructure | Multi-provider VPN management with P2P optimization, server carousel, kill switch, and leak protection |
| [ai](plugins/Ai) | N/A | integrations | Unified AI gateway with multi-provider LLM support, embeddings, semantic search, prompt templates, and usage tracking |
| [web3](plugins/Web3) | N/A | integrations | Blockchain integration, NFT support, token-gated access, DAO governance, and decentralized identity |
| [content-acquisition](plugins/Content-Acquisition) | N/A | media | Automated content acquisition with RSS monitoring, release calendar, quality profiles, and download pipeline orchestration |
| [content-progress](plugins/Content-Progress) | N/A | media | Track video, audio, and content playback progress with continue watching, watchlists, and favorites |
| [epg](plugins/Epg) | N/A | media | Electronic program guide with XMLTV import, channel management, and schedule queries |
| [media-processing](plugins/Media-Processing) | N/A | media | FFmpeg-based media encoding and processing with HLS streaming support |
| [metadata-enrichment](plugins/Metadata-Enrichment) | N/A | media | Metadata enrichment with TMDB, TVDB, and MusicBrainz integration for movies, TV shows, and music |
| [photos](plugins/Photos) | N/A | media | Photo album management with EXIF extraction, tagging, face grouping, and thumbnails |
| [retro-gaming](plugins/Retro-Gaming) | N/A | media | Retro gaming ROM library management, emulator core serving, save state synchronization, and controller configuration |
| [rom-discovery](plugins/Rom-Discovery) | N/A | media | ROM metadata database, search, discovery, automated download orchestration, and multi-source scraping for nself-tv |
| [subtitle-manager](plugins/Subtitle-Manager) | N/A | media | Subtitle search, download, and sync verification via OpenSubtitles and multi-source providers |
| [tmdb](plugins/Tmdb) | N/A | media | Media metadata enrichment from TMDB/IMDb with auto-matching and manual review queue |
| [tokens](plugins/Tokens) | N/A | media | Secure content delivery tokens, HLS encryption key management, and entitlement checks |
| [torrent-manager](plugins/Torrent-Manager) | N/A | media | Torrent downloading with Transmission/qBittorrent integration, multi-source search, and VPN enforcement |
| [sports](plugins/Sports) | N/A | sports | Comprehensive sports data plugin with live scores, schedules, standings, team rosters, player stats, and real-time game updates |
| [devices](plugins/Devices) | N/A | streaming | IoT device enrollment, trust management, and command dispatch service. |
| [recording](plugins/Recording) | N/A | streaming | Recording orchestration and archive management service. |
| [stream-gateway](plugins/Stream-Gateway) | N/A | streaming | Stream admission and governance service. |
