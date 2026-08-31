# mail

Send transactional and broadcast email through the nSelf stack: the mux +
Postmark pipeline via `ping_api`, template management, and DKIM
verification.

**Tier:** Pro (MIT) — requires an ɳSelf+ or ɳClaw bundle
license (the bundle that ships the Postmark plugin).

## Installation

```bash
nself plugin install mail
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table of its own. Installing it places the
`nself-mail` binary at `~/.nself/plugins/bin/nself-mail`. From then on,
`nself mail ...` routes to it exactly as it did when `mail` was a core
command (pre-CLI-R11).

## Usage

```bash
nself mail send --to a@example.com --subject "Hi" --body "Hello there"
nself mail broadcast --list <list-id> --template <tpl-id>
nself mail status --message-id <id>
nself mail templates list
nself mail dkim verify --domain example.com
```

Every subcommand accepts `--json` for machine-readable output. Without a
configured license key, every subcommand exits 2 with:
`nself mail requires nSelf+ or nClaw bundle (Postmark plugin) — run 'nself license add <key>'`.

## History

Extracted from `cli/cmd/commands/mail.go`, `mail_transactional.go`, and
`mail_admin.go` under CLI-R11. The domain package (`internal/mail`: the
ping_api HTTP client, tests included) moved wholesale, unchanged. Two small
pieces the CLI's `internal/*` packages provided were reimplemented
standalone, since the plugin cannot import them across the module boundary:

- `internal/tui` in place of the CLI's `internal/ui` (terminal output,
  including the box-drawing table renderer `mail templates list` uses).
- `internal/licensekeys` in place of `internal/license.CollectLicenseKeys` —
  a byte-for-byte copy of just the key-collection logic, not the full
  license package (key validation, product detection, remote checks are
  never called by `mail.go`).

The exit-code-2-on-no-license behavior (`internal/plugin.ExitCodeError` in
core) is reproduced with a small local `exitCodeError` type in `cmd/main.go`.

## Development

```bash
go build -o nself-mail ./cmd/
go test ./...
```
