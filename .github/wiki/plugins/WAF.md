# WAF Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.6 |
| **Install** | `nself plugin install waf` |
| **Commands** | `nself waf` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

> Turns on Coraza with the OWASP Core Rule Set and switches between detect and block mode.

## Install

```bash
nself plugin install waf
```

No license key required.

## Description

Web Application Firewall management: enable Coraza with the OWASP Core Rule Set, switch between detection and blocking mode, and review recent WAF events.

This is a CLI plugin: it installs the `nself-waf` binary into your plugin path and runs as a command, not a background service.

Category: `compliance`. Current version: `1.0.0`.

## Commands

`nself-waf` subcommands (installed alongside the plugin):

- `nself-waf enable`
- `nself-waf report`

## Examples

### Enable

```bash
nself-waf enable
```

### Report

```bash
nself-waf report
```

## Source

[`plugins/waf/`](https://github.com/nself-org/plugins/tree/main/waf)

Manifest: [`plugins/waf/plugin.json`](https://github.com/nself-org/plugins/tree/main/waf/plugin.json)

## See Also

- [[Audit-Log]] — tamper-evident mutation log
- [[GDPR]] — data portability and erasure

← [[Home]] →
