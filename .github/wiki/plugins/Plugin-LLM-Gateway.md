# Plugin LLM Gateway Plugin

> ClawDE-facing LLM gateway: per-tenant token quota, Redis response caching, session context injection, and SSRF guard over nself-ai-gateway (port 3761). **Free — MIT licensed.**

## Install

```bash
nself plugin install plugin-llm-gateway
```

No license key required.

## Description

ClawDE-facing LLM gateway: per-tenant token quota, Redis response caching, session context injection, and SSRF guard over nself-ai-gateway (port 3761). Simplifies ClawDE client LLM calls.

Category: `infrastructure`. Current version: `0.1.0`.

## Ports

| Port | Purpose |
|------|---------|
| 8090 | Plugin LLM Gateway service port |

## Database Schema

2 table(s) added to your Postgres database:

- `np_llm_gateway_requests`
- `np_llm_gateway_quota_usage`

## Examples

```bash
nself plugin install plugin-llm-gateway
```

## Source

[`plugins/plugin-llm-gateway/`](https://github.com/nself-org/plugins/tree/main/plugin-llm-gateway)

Manifest: [`plugins/plugin-llm-gateway/plugin.json`](https://github.com/nself-org/plugins/tree/main/plugin-llm-gateway/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
