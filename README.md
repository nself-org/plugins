# nself Plugins — Free & Community

[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](https://opensource.org/licenses/MIT)
[![nself](https://img.shields.io/badge/nself-compatible-blue)](https://nself.org)

Free and open-source plugins for [nself](https://nself.org) — the self-hosted backend CLI.

## Install a Plugin

```bash
nself plugin install <name>
```

## Free Plugins (129)

<!-- Count generated from registry.json (`jq '.plugins|length' registry.json`), verified via cli/scripts/plugin-counts.sh — do not hand-type this number. -->
All 129 plugins are MIT-licensed and listed below (generated from `registry.json` via `cli/scripts/plugin-counts.sh` — see `.claude/docs/sport/F03-PLUGIN-INVENTORY-FREE.md` for narrative detail, currently pending its own regen per P6-E9-W4-S3-T2).

| Plugin | Description | Category |
| ------ | ----------- | -------- |
| [access-controls](./free/access-controls/) | Role-based and attribute-based access control (RBAC + ABAC) with policy engine | Authentication |
| [admin-api](./free/admin-api/) | Admin API service providing aggregated metrics, system health, session counts, storage breakdown, and real-time dashboard endpoints | Infrastructure |
| [ai-cli](./free/ai-cli/) | AI operations for nSelf: chat, local Ollama model management, and Gemini API key pool provisioning and rotation. | Automation |
| [ai-studio](./free/ai-studio/) | Google AI Studio integration for local nSelf instances via a secure Cloudflare Tunnel. | Integrations |
| [alerts](./free/alerts/) | Manage Prometheus alert rules and Alertmanager silences: list, silence, and send synthetic test alerts. | Infrastructure |
| [analytics](./free/analytics/) | Event tracking, counters, funnels, and quota management analytics engine | Infrastructure |
| [api](./free/api/) | Inspect the nSelf plugin API surface: endpoint probes, deprecation calendar, and the API changelog. | Development |
| [audit](./free/audit/) | Ecosystem documentation audit: banned words, dead links, and missing anchors across READMEs, wiki, docs, SPORT, PPI, and PRI. | Infrastructure |
| [audit-analytics](./free/audit-analytics/) | Advanced audit analytics over np_audit_log: anomaly detection (z-score baseline), user behaviour heatmaps, privileged-action review queue, and webhook/email alerts. ɳSelf+ required. Free tier retains full audit capture and basic search. | Compliance |
| [audit-log](./free/audit-log/) | Append-only audit log for security-relevant events: auth, privilege change, secret access, plugin install/uninstall. Queryable from Admin with filters by event type, actor, severity, and time range. | Compliance |
| [auth-enterprise](./free/auth-enterprise/) | MFA enforcement (TOTP + WebAuthn policy) and SSO via SAML 2.0 and OIDC for Google Workspace, Okta, and Microsoft Entra ID. | Authentication |
| [backup](./free/backup/) | PostgreSQL backup and restore automation with scheduling | Infrastructure |
| [byok](./free/byok/) | Bring Your Own Key (BYOK) per-tenant encryption. Envelope encryption with customer-managed keys (CMK) via AWS KMS, GCP Cloud KMS, or HashiCorp Vault Transit. DEK wrapped by CMK. Satisfies HIPAA, FedRAMP High, FFIEC, and DORA key-control requirements. Enterprise-only. | Compliance |
| [cdc](./free/cdc/) | Change Data Capture — streams Postgres WAL events to downstream consumers via webhooks or message queues. | Infrastructure |
| [cdn](./free/cdn/) | CDN management and integration plugin - cache purging, signed URLs. Planned: analytics sync from Cloudflare/BunnyCDN | Infrastructure |
| [ci](./free/ci/) | Local CI gate runner: detects repo stack (Go/Node/Flutter/Dart), runs lint+test+build, scans secrets with gitleaks, then posts a GitHub commit status (nself-ci) via gh OAuth. Replaces billing-blocked GitHub Actions as the merge gate. | Development |
| [claw-cli](./free/claw-cli/) | CLI client for the nClaw AI assistant: prompt, chat, pairing, keys, memories, topics, sessions, MCP server, OpenAI-compatible proxy, and schema migrations. | Integrations |
| [cloudflare](./free/cloudflare/) | Cloudflare zone, DNS, R2, cache, and analytics management | Infrastructure |
| [compliance](./free/compliance/) | Comprehensive compliance and audit platform with GDPR/CCPA/HIPAA/SOC2/PCI management, DSARs, consent tracking, data retention, breach notification, immutable audit logging, SIEM integration, and compliance reporting | Compliance |
| [content-acquisition](./free/content-acquisition/) | Content acquisition with download rules engine. Planned: RSS feed monitoring, release calendar integration, automated download orchestration | Media |
| [content-progress](./free/content-progress/) | Track video, audio, and content playback progress with continue watching, watchlists, and favorites | Media |
| [content-safety](./free/content-safety/) | Trust-safety evidence, legal holds, spam detection, raid protection, and abuse scoring | Security |
| [costs](./free/costs/) | Show estimated per-install operational costs: Hetzner VPS, Cloudflare, Vercel, Stripe fees, and installed paid plugin licenses. | Infrastructure |
| [crdt](./free/crdt/) | CRDT offline-first primitives. Self-hosted Yjs (y-websocket protocol) and automerge sync server with Postgres persistence. Drop-in replacement for Liveblocks/PartyKit with zero extra infra. | Infrastructure |
| [cron](./free/cron/) | Cron job scheduler. Register jobs with standard cron syntax, execute via HTTP callbacks, track run history. | Automation |
| [ddns](./free/ddns/) | Dynamic DNS plugin with external IP monitoring. Planned: DNS provider API integration (Cloudflare, Route53) for automated record updates | Infrastructure |
| [devices](./free/devices/) | IoT device enrollment, trust management, and command dispatch service. | Streaming |
| [dlq](./free/dlq/) | Manage dead-letter queues for nSelf plugins: re-enqueue rows that failed processing back to the work queue, with safe row limits and dry-run preview. | Infrastructure |
| [documents](./free/documents/) | Document management and generation service with templates, versioning, and sharing | Data |
| [dogfood](./free/dogfood/) | Production dogfood audit and reporting: 21 read-only checks covering backups, DR, tenancy, licensing, secrets, migrations, monitoring, security, watchdog, and queue health. | Infrastructure |
| [donorbox](./free/donorbox/) | Donorbox donation data sync with webhook handling | Commerce |
| [dr](./free/dr/) | Disaster recovery: promote a standby, fence the old primary, run drills, and install the systemd units DR needs. | Infrastructure |
| [e2ee](./free/e2ee/) | End-to-end encryption key directory: X3DH prekey distribution + Kyber-1024 (ML-KEM-1024) post-quantum prekeys for nchat. Server stores PUBLIC keys only; private keys never leave the client. | Authentication |
| [email](./free/email/) | Transactional email via Elastic Email. Send, template, and track emails. | Communication |
| [encryption](./free/encryption/) | Bring Your Own Key (BYOK) per-tenant envelope encryption for nSelf Cloud: AWS KMS, GCP Cloud KMS, and HashiCorp Vault Transit, with key rotation and an audit trail. | Security |
| [entitlements](./free/entitlements/) | Feature gating, subscription plan management, usage quota tracking, and metered billing | Commerce |
| [event-bus](./free/event-bus/) | Internal event bus with pub/sub, fan-out delivery, dead-letter queue, and replay for inter-plugin messaging. | Infrastructure |
| [family-ancestry](./free/family-ancestry/) | Ancestry.com → nFamily migration helper (PLANNED). Imports profiles, photos, documents, sources into the family plugin. Pattern mirrors family-geni. | Social |
| [family-familysearch](./free/family-familysearch/) | FamilySearch → nFamily migration helper (PLANNED). Free public API from FamilySearch (LDS). Lowest-friction next-step importer after family-geni. | Social |
| [family-gedcom](./free/family-gedcom/) | Generic GEDCOM file importer for the family plugin (PLANNED). Accepts any GEDCOM 5.5.1 / 7.0 file from any provider, with optional photo-folder upload. Free MIT plugin — showcases the core + helper-importer pattern. | Content |
| [family-myheritage](./free/family-myheritage/) | MyHeritage → nFamily migration helper (PLANNED). MyHeritage is Geni.com's parent company — lowest legal risk, potential partnership path. | Social |
| [family-wikitree](./free/family-wikitree/) | WikiTree → nFamily migration helper (PLANNED). WikiTree is a free public genealogy wiki with a REST API (Apps API). | Social |
| [feature-flags](./free/feature-flags/) | Feature flags service with targeting rules, segments, and evaluation engine | Infrastructure |
| [federation](./free/federation/) | Manage GraphQL Federation: compose an Apollo Router supergraph from installed plugin subgraphs, check subgraph health, and introspect the composed schema. | Infrastructure |
| [file-processing](./free/file-processing/) | File processing with thumbnails and optimization for MinIO/S3/GCS/R2/B2/Azure. Planned: Inbound webhook support for storage provider notifications | Infrastructure |
| [flags](./free/flags/) | Manage feature flags served by the feature-flags plugin: list, get, set, history, canary rollouts and kill switches. | Development |
| [forgejo](./free/forgejo/) | Self-hosted Forgejo git forge + Forgejo Actions runner. Provides offline CI that executes .github/workflows/*.yml YAML on self-hosted compute — zero GitHub Actions quota consumed. Designed for the ops profile (ops server on staging/prod). | Development |
| [functions-v8](./free/functions-v8/) | Edge Functions V8 Runtime. Deploy short-lived TypeScript functions with a Deno V8 isolate pool. HTTP-trigger, <50ms cold-start, allowlist-only env injection, Prometheus metrics, SSE log streaming. | Infrastructure |
| [game-metadata](./free/game-metadata/) | Game metadata service with IGDB integration, ROM hash matching, tier requirements, and artwork management | Media |
| [gateway](./free/gateway/) | Manage the nSelf AI gateway (nself-ai-gateway, port 3761): service health, provider key vault, quota usage, and routing rules. | Integrations |
| [gauth](./free/gauth/) | Manage Google OAuth tokens for nSelf AI services: status, refresh, and revoke against plugin-gauth. | Integrations |
| [gdpr](./free/gdpr/) | GDPR data portability (Art. 20) and right-to-erasure (Art. 17) tools for self-hosted nSelf instances. | Compliance |
| [geocoding](./free/geocoding/) | Geocoding plugin with geofence storage. Planned: Google Maps API integration for forward/reverse geocoding and place search | Infrastructure |
| [github](./free/github/) | GitHub repository, issue, and workflow integration | Development |
| [github-runner](./free/github-runner/) | GitHub Actions self-hosted runner. Registers with your GitHub org and picks up CI jobs tagged `runs-on: ubuntu-latest` — enabling private repos to run CI without GitHub-hosted runners. | Development |
| [hipaa](./free/hipaa/) | HIPAA compliance add-on: PHI column registry, PHI access logging with 6-year retention, de-identification helpers (masking + tokenization), encryption-at-rest audit, and BAA workflow. Requires ɳSelf+ license. | Compliance |
| [home](./free/home/) | Home automation bridge. Connects Home Assistant and MQTT to ɳSelf, enabling smart device control, state monitoring, scene activation, and command logging. | Integrations |
| [idme](./free/idme/) | ID.me OAuth authentication with government-grade identity verification for 7 groups | Authentication |
| [infra](./free/infra/) | Provision nSelf infrastructure with Terraform: plan, apply and destroy modules for aws, gcp, azure, hetzner, do and linode. | Infrastructure |
| [invitations](./free/invitations/) | Invitation management system with email/SMS delivery and tracking | Communication |
| [job-queue](./free/job-queue/) | Durable background job queue with priorities, retries, scheduled execution, and per-job progress tracking. | Infrastructure |
| [jobs](./free/jobs/) | PostgreSQL-backed background job queue with priorities, scheduling, retries, and REST API. Simplified Go implementation using database polling instead of Redis/BullMQ. | Infrastructure |
| [k8s](./free/k8s/) | Deploy and manage nSelf on any Kubernetes cluster via the official Helm chart: install, upgrade, and status commands wrapping helm. | Infrastructure |
| [link-preview](./free/link-preview/) | URL metadata extraction with Open Graph, Twitter Cards, and caching | Content |
| [linkedin](./free/linkedin/) | LinkedIn publishing integration. OAuth 2.0 connection, post to LinkedIn feed with optional image attachments, post history, and Claw tool descriptor. | Content |
| [mail](./free/mail/) | Send transactional and broadcast email through the nSelf stack: mux + Postmark pipeline via ping_api, template management, and DKIM verification. | Communication |
| [maintenance](./free/maintenance/) | Maintenance utilities: disk cleanup, log rotation and the maintenance scheduler. | Infrastructure |
| [mdns](./free/mdns/) | mDNS/Bonjour service discovery for zero-config LAN advertising | Infrastructure |
| [media-processing](./free/media-processing/) | FFmpeg-based media encoding and processing with HLS streaming support | Media |
| [meetings](./free/meetings/) | Calendar integration and meeting management with room booking, recurring meetings, and availability tracking. External calendar sync (Google/Outlook) planned for future release. | Development |
| [mlflow](./free/mlflow/) | MLflow experiment tracking and model registry | Data |
| [model](./free/model/) | Manage local AI models via Ollama: list, pull, remove, update, benchmark, plus the legacy ollama command tree. | Integrations |
| [monitor](./free/monitor/) | Monitoring stack management: upgrade the bundled Grafana dashboards. | Infrastructure |
| [monitoring](./free/monitoring/) | Full monitoring stack: Prometheus, Grafana, Loki, Promtail, Tempo, Alertmanager, and exporters | Infrastructure |
| [notifications](./free/notifications/) | Multi-channel notification service. Channels: Email (SMTP), Push (placeholder), SMS (placeholder). HTTP endpoints for sending notifications, managing templates, and user notification preferences. | Communication |
| [notify](./free/notify/) | Multi-channel notification service. Channels: Email (SMTP), Webhook (HMAC-signed). HTTP endpoints for sending notifications, managing templates, and viewing delivery history. | Communication |
| [nself-cloud](./free/nself-cloud/) | Internal ɳCloud managed hosting infrastructure plugin — provisions isolated nSelf instances for Cloud customers | Infrastructure |
| [nself-eval-gate](./free/nself-eval-gate/) | Eval harness and autonomy-tier gate for nSelf. Three-mode scoring (exact, semantic via BGE-M3, rubric via LLM-as-judge), recall-quality precision/recall/fact_f1 metrics, CI integration via nself ci eval, and autonomy-tier threshold enforcement. | Ci |
| [nself-geo](./free/nself-geo/) | Forward and reverse geocoding with provider-agnostic caching layer. Nominatim (free, OSM) is the default; Google Places and Mapbox are premium fallbacks. Exposes geocodeAddress, reverseGeocode, geocodeBatch, clearGeoCache via Hasura Remote Schema. | Integrations |
| [nself-image](./free/nself-image/) | Server-side image processing plugin for nSelf: resize, crop, format conversion (WebP/AVIF/JPEG/PNG), EXIF strip, and MinIO-integrated upload pipeline. Replaces per-app Sharp/Node.js usage across nFamily, nChat, and any consumer app needing image normalization. | Media |
| [nself-pdf](./free/nself-pdf/) | Server-side PDF generation from HTML templates (Handlebars/Nunjucks) with MinIO output and Hasura Action trigger | Content |
| [nself-scan](./free/nself-scan/) | Server-side file scanning for MinIO uploads: magic-byte MIME validation, ClamAV virus/malware scanning (always free, Security-Always-Free Doctrine), and optional CSAM hash detection (deferred — requires partner agreement) | Compliance |
| [nself-sync](./free/nself-sync/) | Event-log sync engine for nClaw. Multi-device state synchronization via hybrid logical clocks (HLC) and last-write-wins (LWW) conflict resolution. JWT-authenticated push/pull/snapshot/subscribe with Ed25519-signed events. | Infrastructure |
| [nself-vault](./free/nself-vault/) | nSelf-managed envelope encryption KMS. Provides per-row/column selective encryption with key rotation, audit logging, and Hasura Action surface. Eliminates ad-hoc per-team AES wrappers. | Infrastructure |
| [object-storage](./free/object-storage/) | Multi-provider object storage with S3-compatible API, local storage, presigned URLs, and multipart uploads | Infrastructure |
| [observability](./free/observability/) | Unified observability service with health probes, watchdog timers, service auto-discovery, and systemd integration | Infrastructure |
| [ollama](./free/ollama/) | One-click offline LLM stack. Stands up an Ollama container, auto-pulls gemma-3-4b on first start, and registers as a provider in plugin-ai. All nSelf AI features route through Ollama when NSELF_AI_PROVIDER=ollama. Zero cloud dependency, zero API key, zero usage cost after install. | Integrations |
| [payments](./free/payments/) | Unified payments abstraction supporting Stripe, PayPal, and Apple/Google Pay with webhook normalization. | Commerce |
| [paypal](./free/paypal/) | PayPal payment data sync with webhook handling | Commerce |
| [pentest](./free/pentest/) | Penetration-test readiness kit. Generates structured scope documents, provisions pentest credentials, tracks findings, and manages remediation. ɳSelf+ tier. | Compliance |
| [pentest-kit](./free/pentest-kit/) | CLI front end for the pentest plugin: generate scope documents, provision test credentials, import findings, and check remediation status from `nself pentest-kit`. Business+ tier. | Compliance |
| [plugin-clawde](./free/plugin-clawde/) | ClawDE daemon integration backend. Manages session lifecycle, tracks daemon health, and streams events via SSE for the ClawDE AI development environment. | Ai |
| [plugin-gauth](./free/plugin-gauth/) | Headless server-side Google OAuth token refresh for nSelf AI services | Ai |
| [plugin-llm-gateway](./free/plugin-llm-gateway/) | ClawDE-facing LLM gateway: per-tenant token quota, Redis response caching, session context injection, and SSRF guard over nself-ai-gateway (port 3761). Simplifies ClawDE client LLM calls. | Ai |
| [plugin-pty](./free/plugin-pty/) | Pseudo-terminal bridge for ClawDE AI sessions. Spawns, manages, and relays PTY processes with per-tenant resource limits and WebSocket I/O. | Ai |
| [plugin-retrieval](./free/plugin-retrieval/) | Hybrid retrieval plugin: pgvector ANN + tsvector BM25 merged with Reciprocal Rank Fusion (RRF). Provides the search backend for ɳClaw memory and nself-ai-mcp search/recall tools. | Ai |
| [podcast](./free/podcast/) | Podcast service with RSS feed parsing, episode management, playback position sync, and subscription management | Media |
| [post](./free/post/) | Multi-platform content publishing. Publish to WordPress, Ghost, Twitter/X, LinkedIn, Telegram channels, Dev.to, and Hashnode with optional scheduling. | Content |
| [push](./free/push/) | APNs + FCM push notification relay. Hasura event-trigger fan-out, delivery state tracking, exponential backoff retry. Handles iOS (Apple Push Notification service) and Android (Firebase Cloud Messaging v1 API). | Communication |
| [queue](./free/queue/) | Inspect and manage nSelf background job queues: depth, stuck jobs, retries and purges. | Infrastructure |
| [region](./free/region/) | Multi-region management: add replica regions, list and inspect their status, and promote a region to primary. | Infrastructure |
| [release](./free/release/) | Orchestrate the nSelf project's own 12-step release cascade: tag and release cli and plugins-pro, build and push the admin image, and open the Homebrew formula PR. | Development |
| [retro-gaming](./free/retro-gaming/) | Retro gaming ROM library management, emulator core serving, save state synchronization, play sessions, and controller configuration for nself-tv | Media |
| [rom-discovery](./free/rom-discovery/) | ROM metadata database, search, discovery, automated download orchestration, and multi-source scraping for nself-tv | Media |
| [search](./free/search/) | Full-text search engine with PostgreSQL FTS and MeiliSearch support | Infrastructure |
| [sentry-cli](./free/sentry-cli/) | ɳSentry operations: monitors, incidents, status pages, alerts, cloud login, and provisioning a self-hosted sentry server. | Infrastructure |
| [shared-utils](./free/shared-utils/) | Shared Go utilities (request-ID tracing middleware, HTTP client propagation, server lifecycle helpers) used internally by multiple free nself plugins. Not installable directly. | Infrastructure |
| [shopify](./free/shopify/) | Shopify store, orders, and product synchronization | Commerce |
| [siem](./free/siem/) | Forward nSelf audit logs and security events to external SIEM platforms: Datadog, Splunk HEC, Elastic, Loki, and custom webhooks. OCSF/ECS schema normalization. ɳSelf+ required for external destinations. | Infrastructure |
| [sms](./free/sms/) | SMS messaging via Twilio. Send, track, and manage opt-outs. | Communication |
| [soak](./free/soak/) | Manage soak testing lifecycle: abort an active soak and roll back to a prior version. | Infrastructure |
| [sports](./free/sports/) | Sports data plugin with storage for scores, schedules, and standings. Planned: Live data provider integration (ESPN, The Sports DB) for real-time scores and stats | Sports |
| [storage](./free/storage/) | S3-compatible file storage: bucket management, object PUT/GET/DELETE/LIST, presigned URLs, per-tenant isolation. | Infrastructure |
| [storage-transform](./free/storage-transform/) | On-the-fly image transformation CDN: resize, crop, format convert (WebP/AVIF/JPEG/PNG), quality, and device-pixel-ratio support. URL-param driven, Redis LRU cache, Nginx cache headers, rate limiting. | Infrastructure |
| [stripe](./free/stripe/) | Stripe billing data sync with webhook handling | Commerce |
| [subtitle-manager](./free/subtitle-manager/) | Subtitle search, download, and sync verification via OpenSubtitles | Media |
| [tenant](./free/tenant/) | Multi-tenant operations: create, suspend, upgrade and destroy tenants, plus per-tenant usage metering and billing reports. | Infrastructure |
| [tenant-controller](./free/tenant-controller/) | Multi-tenant master controller for nCloud. Manages N isolated nSelf project instances behind a single deploy: per-project Postgres schema, Hasura metadata namespace, Nginx vhost, JWT secret, Redis key prefix, and MinIO bucket. Enables 50 projects on one Hetzner CX21. | Infrastructure |
| [tmdb](./free/tmdb/) | Comprehensive media metadata enrichment from TMDB/IMDb/TVDB/MusicBrainz with auto-matching, manual review queue, and multi-provider support | Media |
| [tokens](./free/tokens/) | Secure content delivery tokens, HLS encryption key management, and entitlement checks | Media |
| [torrent-manager](./free/torrent-manager/) | Torrent downloading with Transmission/qBittorrent integration, multi-source search, seeding policies, and VPN enforcement | Media |
| [transactional-email](./free/transactional-email/) | Provider-agnostic transactional email: template rendering, per-tenant domain management, SPF/DKIM reporting, delivery webhook relay. | Communication |
| [vpn](./free/vpn/) | Multi-provider VPN management (NordVPN, PIA, Mullvad) with P2P optimization, server carousel, kill switch, and leak protection. Torrent download forwarding requires the torrent-manager plugin. | Authentication |
| [waf](./free/waf/) | Web Application Firewall management: enable Coraza with the OWASP Core Rule Set, switch between detection and blocking mode, and review recent WAF events. | Compliance |
| [warehouse](./free/warehouse/) | Data warehouse sync — exports nself table data to BigQuery, Snowflake, or Redshift on a configurable schedule. | Infrastructure |
| [watchdog](./free/watchdog/) | Self-healing container watchdog with circuit breaker: status, resets, event history, and TG/email escalation alerts. | Infrastructure |
| [web3](./free/web3/) | Blockchain integration, NFT support, token-gated access, DAO governance, and decentralized identity | Integrations |
| [webhooks](./free/webhooks/) | Outbound webhook delivery service with retry logic, HMAC signing, and dead-letter queue | Communication |
| [workflows](./free/workflows/) | Automation engine providing trigger-action workflow chains, conditional logic, scheduled tasks, webhook integrations, and cross-plugin orchestration | Automation |

Install any plugin with `nself plugin install <name>`. No license key required.

> Note: `notifications` (older, 695-line implementation) and `notify` (newer, focused on Email + Webhook channels) are two separate plugins. New projects should prefer `notify`.

**Building plugins for nClaw?** See the [nClaw plugin author guide](../nclaw/.github/wiki/plugin-author-guide.md).

## Monitoring Services (10)

The `monitoring` plugin bundles 10 individual observability services (per F05 PLUGIN-INVENTORY-MONITORING). They are wired together by the parent plugin's docker-compose templates and ship as one install.

| Service | Purpose | Default Port |
| ------- | ------- | ------------ |
| [alertmanager](./monitoring/alertmanager/) | Alert routing and deduplication for Prometheus | 9093 |
| [glitchtip](./monitoring/glitchtip/) | Open-source Sentry-compatible error tracking | 8000 |
| [grafana](./monitoring/grafana/) | Metrics, logs, and traces dashboards | 3000 |
| [loki](./monitoring/loki/) | Log aggregation backend | 3100 |
| [otel-collector](./monitoring/otel-collector/) | OpenTelemetry traces, metrics, and logs ingest | 4317 (gRPC), 4318 (HTTP) |
| [prometheus](./monitoring/prometheus/) | Metrics scraping and storage | 9090 |
| [promtail](./monitoring/promtail/) | Log shipping agent (sends to Loki) | 9080 |
| [status](./monitoring/status/) | Public status page generator | internal |
| [synthetics](./monitoring/synthetics/) | k6-based synthetic flow probes | internal |
| [web-vitals](./monitoring/web-vitals/) | Core Web Vitals client beacon and ingest | internal |

Install the bundle with `nself plugin install monitoring`. SLO definitions live in `./monitoring/docs/slos.md`.

## Community Plugins

Have a plugin to share? Add it to the `community/` directory and open a PR.

See [Contributing Guide](./.github/wiki/Contributing.md) for guidelines.

## Pro Plugins

62 production-grade plugins starting at $0.99/month ($9.99/year).

No other self-hosted backend ships anything close to this. These are not thin wrappers. Each plugin is a complete service with its own database schema, API surface, and production-tested logic built for the nself stack.

[See pricing tiers at nself.org/pricing](https://nself.org/pricing) · [Browse catalog at nself.org/plugins](https://nself.org/plugins)

### What Pro Plugins include

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

### Why Pro Plugins

Building this yourself takes months. Some examples:

| What you'd need to build | Rough effort | Managed alternative cost |
| --- | --- | --- |
| Stripe webhook sync (23 tables, 60+ events) | 2-3 weeks | N/A (you still need the integration) |
| GDPR/HIPAA compliance tooling | 3-6 months | $100-$1,000+/month (Osano, OneTrust) |
| LiveKit voice/video integration | 1-2 weeks | $50-$500+/month |
| AI gateway with multi-provider support | 1-2 weeks | $25-$200+/month |
| Live streaming (RTMP/HLS + DVR) | 3-6 weeks | $100-$500+/month |
| EPG + media metadata enrichment | 2-4 weeks | No managed equivalent |

With nself Pro Plugins, you self-host everything. The Basic tier ($0.99/mo or $9.99/yr) covers all 62 pro plugins, not per-seat, not per-request, not per-service. Higher tiers (Pro, Elite, Business, Business+, Enterprise) add AI suite, support levels, and managed DevOps.

[See pricing tiers at nself.org/pricing](https://nself.org/pricing)

## Documentation

- [Plugin development guide](https://nself.org/docs/plugins)
- [Full plugin catalog](https://nself.org/plugins)
- [nself CLI docs](https://nself.org/docs)

## License

MIT — see [LICENSE](./LICENSE)
