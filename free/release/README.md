# nself-release

Orchestrates the nSelf project's own 12-step release cascade: tagging the cli and
plugins-pro repos, building and pushing the admin image, and opening the Homebrew
formula PR.

This is maintainer tooling for the nSelf project itself. A self-hosted user never
runs it, which is why its 1,523 lines no longer ship inside the CLI binary.

## Install

```bash
nself install release
```

`nself release ...` then works exactly as before — the CLI proxies the command here.

## License

MIT.
