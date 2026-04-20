---
name: v1.1.1 plugin API compatibility notice
about: "Pinned notice: v1.1.1 API changes for plugin authors"
title: "[NOTICE] v1.1.1 — plugin API compatibility (no action required)"
labels: ["notice", "pinned", "v1.1.1"]
assignees: []
---

## v1.1.1 Plugin Author Notice

**TL;DR: no action required.** All existing plugins built for v1.0.5 work on CLI v1.0.9 + plugins-pro v1.1.1 without changes.

---

### What changed

- Internal plugin loader updated for CLI v1.0.9 (Go rewrite).
- Registry schema version bumped to `1.1` — backward compatible with `1.0`.
- License validation errors are now surfaced clearly on `nself build`.

### What did NOT change

- `plugin.yaml` required fields: no additions, no removals.
- Lifecycle hook signatures: unchanged.
- Env var naming conventions: unchanged.
- GraphQL API shapes for `ai`, `claw`, `mux` plugins: no breaking changes.
- `PLUGIN_LICENSE_KEY` validation endpoint: unchanged.

### Compatibility

| CLI version | plugins-pro | Status |
|---|---|---|
| v1.0.9 | v1.1.1 | Current — supported |
| v1.0.7–v1.0.8 | v1.0.5 | Supported (patch) |
| < v1.0.0 | any | Unsupported |

### References

- [CHANGELOG.md](../blob/main/CHANGELOG.md) — full v1.1.1 diff
- [nSelf v1.0.9 LTS release notes](https://nself.org/blog/v1-0-9-lts)
- [Plugin development guide](https://docs.nself.org/plugins/development)

---

Questions? Comment here or open a new issue referencing this one.
