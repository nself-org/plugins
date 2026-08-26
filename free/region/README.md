# nself-region

Multi-region management, moved out of the CLI core under CLI-R11.

## Install

```bash
nself install region
```

## Commands

```bash
nself region add --region hel1 --pg-url postgres://...
nself region list
nself region status
nself region promote --region hel1
```

## Requirements

Gated behind the `multi_region_enabled` feature flag, read from the
feature-flags plugin. Enable it with:

```bash
nself flags set multi_region_enabled true
```

## License

MIT.
