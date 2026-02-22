# nself Plugins — Free & Community

[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](https://opensource.org/licenses/MIT)
[![nself](https://img.shields.io/badge/nself-compatible-blue)](https://nself.org)

Free and open-source plugins for [nself](https://nself.org) — the self-hosted backend CLI.

## Install a Plugin

```bash
nself plugin install <name>
```

## Free Plugins

| Plugin | Description | Category |
| ------ | ----------- | -------- |
| [content-acquisition](./free/content-acquisition/) | Content ingestion and acquisition | Content |
| [content-progress](./free/content-progress/) | Track user content progress | Content |
| [feature-flags](./free/feature-flags/) | Feature flag management | Dev Tools |
| [github](./free/github/) | GitHub integration and OAuth | Auth |
| [invitations](./free/invitations/) | User invitation flows | Users |
| [jobs](./free/jobs/) | Background job processing | Infrastructure |
| [link-preview](./free/link-preview/) | Link preview generation | Utilities |
| [mdns](./free/mdns/) | Local network mDNS discovery | Networking |
| [notifications](./free/notifications/) | Notification system | Communication |
| [search](./free/search/) | Full-text search via MeiliSearch | Search |
| [subtitle-manager](./free/subtitle-manager/) | Subtitle file management | Media |
| [tokens](./free/tokens/) | API token management | Auth |
| [torrent-manager](./free/torrent-manager/) | Torrent client integration | Media |
| [vpn](./free/vpn/) | VPN configuration management | Networking |
| [webhooks](./free/webhooks/) | Webhook handling and routing | Infrastructure |

## Community Plugins

Have a plugin to share? Add it to the `community/` directory and open a PR.

See [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines.

## Pro Plugins — $9.99/year

49 production-grade plugins. Less than a dollar a month.

No other self-hosted backend ships anything close to this. These are not thin wrappers. Each plugin is a complete service with its own database schema, API surface, and production-tested logic built for the nself stack.

[Get Pro Plugins at nself.org/pricing](https://nself.org/pricing) · [Browse catalog at nself.org/plugins](https://nself.org/plugins)

### What $9.99/year includes

#### AI and intelligence

- `ai` — Multi-provider LLM gateway with embeddings, semantic search, prompt templates, and usage tracking. Works with OpenAI, Anthropic, Cohere, and more.
- `moderation` — Unified content moderation: profanity filtering, toxicity detection, AI-powered review queues, rules automation, manual workflows, strikes, and appeals.

#### Compliance and security

- `compliance` — Full GDPR, CCPA, HIPAA, SOC 2, and PCI-DSS coverage. DSARs, consent management, data retention schedules, breach notifications, SIEM integration, and audit reporting. Most companies pay $10K+/year for tooling that does half this.
- `access-controls` — RBAC and ABAC with a full policy engine.
- `auth` — WebAuthn/passkeys, TOTP 2FA, magic links, device-code flow, and government-grade identity verification via ID.me (7 verification groups).

#### Billing and commerce

- `stripe` — 23 database tables, 7 views, 60+ webhook events. Full sync including subscriptions, invoices, payment methods, disputes, and refunds.
- `paypal` — PayPal payment data sync with webhook handling.
- `donorbox` — Donation platform sync for nonprofits.
- `shopify` — Store, orders, and product synchronization.
- `entitlements` — Feature gating, subscription plan management, usage quota tracking, and metered billing.

#### Media and streaming

- `streaming` — Live streaming with RTMP/HLS ingest, viewer analytics, stream chat, multi-quality adaptive streaming, and DVR.
- `media-processing` — FFmpeg-based media encoding and HLS streaming.
- `livekit` — LiveKit voice/video infrastructure: room management, participant tracking, recording and egress, quality monitoring.
- `recording` — Recording orchestration and archive management.
- `photos` — Photo albums with EXIF extraction, face grouping, tagging, and thumbnails.
- `podcast` — RSS feed parsing, episode management, and transcription.
- `content-progress` — Video, audio, and document playback progress with continue watching, watchlists, and favorites.

#### TV and gaming (unique — no other platform has these)

- `epg` — Electronic program guide with XMLTV import, channel management, and schedule queries. Built for nself-tv and media apps.
- `tmdb` — Media metadata from TMDB, IMDb, TVDB, and MusicBrainz with auto-matching and a manual review queue.
- `retro-gaming` — ROM library management, emulator core serving, save state sync, and controller configuration.
- `rom-discovery` — ROM metadata database, multi-source auto-download orchestration, and scraping.
- `game-metadata` — Game metadata enrichment from IGDB, MobyGames, and more.
- `sports` — Live scores, schedules, standings, rosters, player stats, and real-time updates.

#### Social and community

- `social` — Posts, comments, reactions, follows, and bookmarks.
- `activity-feed` — Fan-out activity feeds with aggregation and subscriptions.
- `chat` — Chat and messaging data layer with conversations, participants, and moderation.
- `bots` — Bot framework with commands, marketplace, API keys, and reviews.
- `support` — Full helpdesk with ticketing, SLA management, canned responses, knowledge base, and analytics.
- `knowledge-base` — Documentation and FAQ with semantic search, versioning, translations, and analytics.
- `calendar` — Recurring events, iCal export, and RSVP tracking.
- `meetings` — Room booking, Google/Outlook sync, and availability management.

#### Infrastructure and developer tools

- `admin-api` — System health, user management, and metrics API for admin dashboards.
- `analytics` — Event tracking, counters, funnels, and quota management.
- `observability` — Prometheus metrics, Loki logging, and Tempo tracing.
- `backup` — PostgreSQL backup and restore automation with scheduling.
- `file-processing` — Thumbnails, optimization, and virus scanning across S3, GCS, R2, B2, Azure, and MinIO.
- `object-storage` — Multi-provider object storage with presigned URLs and multipart uploads.
- `workflows` — Automation engine with trigger-action chains, conditional logic, and scheduled tasks.
- `realtime` — Socket.io real-time server with presence tracking, typing indicators, and room management.
- `documents` — Document management with templates, versioning, and sharing.
- `cms` — Headless CMS with content types, versioning, categories, and tags.
- `cdn` — CDN management: cache purging, signed URLs, and analytics.
- `webhooks` — Outbound webhook delivery with retry logic, HMAC signing, and dead-letter queue.

#### Integrations and connectivity

- `cloudflare` — Zone, DNS, R2, cache, and analytics management.
- `github` — Repository, issue, and workflow integration (Pro tier with expanded access).
- `geocoding` — Forward and reverse geocoding, place search, and geofences.
- `geolocation` — Real-time location sharing, history tracking, geofencing, and proximity queries.
- `idme` — Government-grade identity verification via ID.me with 7 verification groups.
- `vpn` — Multi-provider VPN management with P2P optimization, server carousel, kill switch, and leak protection.
- `devices` — IoT device enrollment, trust management, and command dispatch.
- `web3` — NFT support, token-gated access, DAO governance, and decentralized identity.
- `torrent-manager` — Torrent client integration with Transmission and qBittorrent, multi-source search, VPN enforcement.

### Why $9.99/year

Building this yourself takes months. Some examples:

| What you'd need to build | Rough effort | Managed alternative cost |
| --- | --- | --- |
| Stripe webhook sync (23 tables, 60+ events) | 2–3 weeks | N/A (you still need the integration) |
| GDPR/HIPAA compliance tooling | 3–6 months | $100–$1,000+/month (Osano, OneTrust) |
| LiveKit voice/video integration | 1–2 weeks | $50–$500+/month |
| AI gateway with multi-provider support | 1–2 weeks | $25–$200+/month |
| Live streaming (RTMP/HLS + DVR) | 3–6 weeks | $100–$500+/month |
| EPG + media metadata enrichment | 2–4 weeks | No managed equivalent |

With nself Pro Plugins, you self-host everything and pay $9.99/year for the integration layer — not per-seat, not per-request, not per-service.

[Get Pro Plugins at nself.org/pricing](https://nself.org/pricing)

## Documentation

- [Plugin development guide](https://docs.nself.org/plugins)
- [Full plugin catalog](https://nself.org/plugins)
- [nself CLI docs](https://docs.nself.org)

## License

MIT — see [LICENSE](./LICENSE)
