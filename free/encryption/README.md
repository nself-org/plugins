# encryption

Bring Your Own Key (BYOK) per-tenant envelope encryption for nSelf Cloud.
Each tenant supplies a Customer Managed Key (CMK) hosted in AWS KMS, GCP
Cloud KMS, or HashiCorp Vault Transit; nSelf wraps its Data Encryption Keys
(DEKs) with that CMK.

**Tier:** Pro (MIT) — requires an ɳSelf+ or Enterprise license
(`NSELF_BYOK=true`).

## Installation

```bash
nself plugin install encryption
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table of its own. Installing it places the
`nself-encryption` binary at `~/.nself/plugins/bin/nself-encryption`. From
then on, `nself encryption ...` routes to it exactly as it did when
`encryption` was a core command (pre-CLI-R11).

## Usage

```bash
nself encryption configure --provider aws --key-id arn:aws:kms:us-east-1:123456:key/abc123
nself encryption verify
nself encryption rotate --dry-run
nself encryption status
nself encryption key-events
```

The plugin talks to the BYOK plugin's own HTTP API (`BYOK_PLUGIN_URL`,
falling back to `NSELF_API_URL`, falling back to `http://localhost:3741`).

## History

Extracted from `cli/cmd/commands/encryption.go` under CLI-R11. The command
had zero dependencies on the core CLI's `internal/*` packages before
extraction (it only ever spoke to the BYOK plugin's HTTP API), so this is a
straight file move with no shim code needed.

## Development

```bash
go build -o nself-encryption ./cmd/
go test ./...
```
