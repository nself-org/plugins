# Storage Plugin

> S3-compatible file storage: bucket management, object PUT/GET/DELETE/LIST, presigned URLs, per-tenant isolation. **Free — MIT licensed.**

## Install

```bash
nself plugin install storage
```

No license key required.

## Description

S3-compatible file storage: bucket management, object PUT/GET/DELETE/LIST, presigned URLs, per-tenant isolation.

Category: `infrastructure`. Current version: `1.0.0`.

## Ports

| Port | Purpose |
|------|---------|
| 9007 | Storage service port |

## Database Schema

3 table(s) added to your Postgres database:

- `np_storage_buckets`
- `np_storage_objects`
- `np_storage_metadata`

## Examples

```bash
nself plugin install storage
```

## Source

[`plugins/storage/`](https://github.com/nself-org/plugins/tree/main/storage)

Manifest: [`plugins/storage/plugin.json`](https://github.com/nself-org/plugins/tree/main/storage/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
