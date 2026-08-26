# nself-forgejo

Self-hosted Forgejo git forge + Forgejo Actions runner. Provides offline CI that executes .github/workflows/*.yml YAML on self-hosted compute — zero GitHub Actions quota consumed. Designed for the ops profile (ops server on staging/prod).

Port 3844.

## Install

```bash
nself install forgejo
```

Then rebuild so the service is wired into your stack:

```bash
nself build && nself start
```

## What it is for

Offline CI. It runs `.github/workflows` on hardware you own, which matters for
private repositories where GitHub-hosted minutes are metered.

## License

MIT.
