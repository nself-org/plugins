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

See [[All Plugins|#all-plugins-64-total]] below for the complete catalog of 64 plugins organized across 16 categories.

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
- `.github/` (includes `.github/wiki/` — canonical wiki source)
- `plugins/`
- `shared/`
- `registry.json`
- `registry-schema.json`
- `README.md`
- `LICENSE`
- required meta files (for example `.gitignore`)

Allowed infrastructure exception:

- `.workers/` for registry publishing.

Legacy `docs/` is retired. Public docs belong in `.github/wiki/` only.

## SPORT Rules

1. If behavior changes, docs must be updated in `.github/wiki/` in the same change set.
2. Commands in `Commands.md` and `commands/*.md` must match action/CLI source files.
3. Any drift between code and docs is treated as a defect.

## All Plugins (64 Total)

Organized across 16 categories: **Admin**, **Authentication**, **Automation**, **Commerce**, **Communication**, **Compliance**, **Content**, **Data**, **Development**, **Infrastructure**, **Integrations**, **Media**, **Monitoring**, **Networking**, **Sports**, **Streaming**

| Plugin | Port | Category | Description |
|--------|------|----------|-------------|
| [access-controls](plugins/Access-Controls) | 3027 | authentication | Role-based and attribute-based access control (RBAC + ABAC) with policy engine |
| [activity-feed](plugins/Activity-Feed) | 3209 | content | Universal activity feed system with fan-out-on-read/write, aggregation, and subscriptions |
| [admin-api](plugins/Admin-Api) | 3212 | admin | Admin API service providing aggregated metrics, system health, session counts, storage breakdown, and real-time dashboard endpoints |
| [ai](plugins/Ai) | 3101 | integrations | Unified AI gateway with multi-provider LLM support, embeddings, semantic search, prompt templates, and usage tracking |
| [analytics](plugins/Analytics) | 3206 | infrastructure | Event tracking, counters, funnels, and quota management analytics engine |
| [auth](plugins/Auth) | 3014 | authentication | Advanced authentication: OAuth, WebAuthn/passkeys, TOTP 2FA, magic links, device-code flow |
| [backup](plugins/Backup) | 3210 | infrastructure | PostgreSQL backup and restore automation with scheduling |
| [bots](plugins/Bots) | 3103 | automation | Bot framework for nself-chat - commands, subscriptions, marketplace, API keys, reviews |
| [calendar](plugins/Calendar) | 3105 | content | Calendar and event management with recurring events, iCal export, and RSVP tracking |
| [cdn](plugins/Cdn) | 3036 | infrastructure | CDN management and integration plugin - cache purging, signed URLs, analytics |
| [chat](plugins/Chat) | 3401 | communication | Chat and messaging data management with conversation, messages, participants, and moderation |
| [cloudflare](plugins/Cloudflare) | 3024 | infrastructure | Cloudflare zone, DNS, R2, cache, and analytics management |
| [cms](plugins/Cms) | 3501 | content | Headless CMS plugin with content types, posts, categories, tags, and versioning |
| [compliance](plugins/Compliance) | 3211 | compliance | Comprehensive compliance and audit platform with GDPR/CCPA/HIPAA/SOC2/PCI management, DSARs, consent tracking, data retention, breach notification, immutable audit logging, SIEM integration, and compliance reporting |
| [content-acquisition](plugins/Content-Acquisition) | 3202 | media | Automated content acquisition with RSS monitoring, release calendar, and download rules engine |
| [content-progress](plugins/Content-Progress) | 3022 | media | Track video, audio, and content playback progress with continue watching, watchlists, and favorites |
| [ddns](plugins/Ddns) | 3217 | networking | Dynamic DNS updater with multi-provider support and external IP monitoring |
| [devices](plugins/Devices) | 3603 | streaming | IoT device enrollment, trust management, and command dispatch service |
| [documents](plugins/Documents) | 3106 | data | Document management and generation service with templates, versioning, and sharing |
| [donorbox](plugins/Donorbox) | 3005 | commerce | Donorbox donation data sync with webhook handling |
| [entitlements](plugins/Entitlements) | 3714 | commerce | Feature gating, subscription plan management, usage quota tracking, and metered billing |
| [epg](plugins/Epg) | 3031 | media | Electronic program guide with XMLTV import, channel management, and schedule queries |
| [feature-flags](plugins/Feature-Flags) | 3207 | infrastructure | Feature flags service with targeting rules, segments, and evaluation engine |
| [file-processing](plugins/File-Processing) | 3104 | infrastructure | File processing with thumbnails, optimization, and virus scanning for MinIO/S3/GCS/R2/B2/Azure |
| [game-metadata](plugins/Game-Metadata) | 3211 | media | Game metadata service with IGDB integration, ROM hash matching, tier requirements, and artwork management |
| [geocoding](plugins/Geocoding) | 3203 | infrastructure | Geocoding and location services plugin - forward/reverse geocoding, place search, geofences |
| [geolocation](plugins/Geolocation) | 3026 | data | Real-time location sharing, history tracking, geofencing, and proximity queries |
| [github](plugins/Github) | 3002 | development | GitHub repository, issue, and workflow integration |
| [idme](plugins/Idme) | 3010 | authentication | ID.me OAuth authentication with government-grade identity verification for 7 groups |
| [invitations](plugins/Invitations) | 3402 | communication | Invitation management system with email/SMS delivery and tracking |
| [jobs](plugins/Jobs) | 3105 | infrastructure | BullMQ background job queue with priorities, scheduling, retries, and BullBoard dashboard |
| [knowledge-base](plugins/Knowledge-Base) | 3713 | content | Knowledge base with documentation, FAQ, semantic search, versioning, translations, and analytics |
| [link-preview](plugins/Link-Preview) | 3718 | content | URL metadata extraction with Open Graph, Twitter Cards, oEmbed support, custom previews, and caching |
| [livekit](plugins/Livekit) | 3107 | communication | LiveKit voice/video infrastructure - room management, participant tracking, recording/egress, quality monitoring |
| [mdns](plugins/Mdns) | 3216 | networking | mDNS/Bonjour service discovery for zero-config LAN advertising |
| [media-processing](plugins/Media-Processing) | 3019 | media | FFmpeg-based media encoding and processing with HLS streaming support |
| [meetings](plugins/Meetings) | 3710 | development | Calendar integration and meeting management with room booking, Google/Outlook sync, recurring meetings, and availability tracking |
| [moderation](plugins/Moderation) | 3208 | content | Unified content moderation platform with profanity filtering, toxicity detection, AI-powered review, rule-based policies, automated actions, manual review workflows, user strikes, and appeals management |
| [notifications](plugins/Notifications) | 3102 | infrastructure | Multi-channel notifications with email, FCM/APNs push, and SMS delivery, templates, preferences, and tracking |
| [object-storage](plugins/Object-Storage) | 3301 | infrastructure | Multi-provider object storage with S3-compatible API, local storage, presigned URLs, and multipart uploads |
| [observability](plugins/Observability) | 3215 | monitoring | Unified observability service with health probes, watchdog timers, service auto-discovery, and systemd integration |
| [paypal](plugins/Paypal) | 3004 | commerce | PayPal payment data sync with webhook handling |
| [photos](plugins/Photos) | 3108 | media | Photo album management with EXIF extraction, tagging, face grouping, and thumbnails |
| [podcast](plugins/Podcast) | 3210 | media | Podcast service with RSS feed parsing, episode management, playback position sync, and subscription management |
| [realtime](plugins/Realtime) | 3109 | infrastructure | Socket.io real-time server with presence tracking, typing indicators, and room management |
| [recording](plugins/Recording) | 3602 | streaming | Recording orchestration and archive management service |
| [retro-gaming](plugins/Retro-Gaming) | 3033 | media | Retro gaming ROM library management, emulator core serving, save state synchronization, play sessions, and controller configuration for nself-tv |
| [rom-discovery](plugins/Rom-Discovery) | 3034 | media | ROM metadata database, search, discovery, automated download orchestration, and multi-source scraping for nself-tv |
| [search](plugins/Search) | 3110 | infrastructure | Full-text search engine with PostgreSQL FTS and MeiliSearch support |
| [shopify](plugins/Shopify) | 3003 | commerce | Shopify store, orders, and product synchronization |
| [social](plugins/Social) | 3502 | content | Universal social features plugin with posts, comments, reactions, follows, and bookmarks |
| [sports](plugins/Sports) | 3035 | sports | Comprehensive sports data plugin with live scores, schedules, standings, team rosters, player stats, and real-time game updates |
| [stream-gateway](plugins/Stream-Gateway) | 3601 | streaming | Stream admission and governance service |
| [streaming](plugins/Streaming) | 3711 | communication | Live streaming and broadcasting with RTMP/HLS, viewer analytics, chat integration, multi-quality streams, DVR playback, and moderation |
| [stripe](plugins/Stripe) | 3001 | commerce | Stripe billing data sync with webhook handling |
| [subtitle-manager](plugins/Subtitle-Manager) | 3204 | media | Subtitle search, download, and sync verification via OpenSubtitles |
| [support](plugins/Support) | 3111 | content | Helpdesk and customer support for nself-chat - ticketing, SLA, canned responses, knowledge base, analytics |
| [tmdb](plugins/Tmdb) | 3032 | media | Comprehensive media metadata enrichment from TMDB/IMDb/TVDB/MusicBrainz with auto-matching, manual review queue, and multi-provider support |
| [tokens](plugins/Tokens) | 3107 | media | Secure content delivery tokens, HLS encryption key management, and entitlement checks |
| [torrent-manager](plugins/Torrent-Manager) | 3201 | media | Torrent downloading with Transmission/qBittorrent integration, multi-source search, seeding policies, and VPN enforcement |
| [vpn](plugins/Vpn) | 3200 | authentication | Multi-provider VPN management (3 VPN providers) and torrent downloads with P2P optimization, server carousel, kill switch, and leak protection |
| [web3](plugins/Web3) | 3112 | integrations | Blockchain integration, NFT support, token-gated access, DAO governance, and decentralized identity |
| [webhooks](plugins/Webhooks) | 3403 | communication | Outbound webhook delivery service with retry logic, HMAC signing, and dead-letter queue |
| [workflows](plugins/Workflows) | 3712 | automation | Automation engine providing trigger-action workflow chains, conditional logic, scheduled tasks, webhook integrations, and cross-plugin orchestration |
