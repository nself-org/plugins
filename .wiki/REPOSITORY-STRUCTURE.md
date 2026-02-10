# Repository Structure Policy

This repository follows a strict structure-first policy.

## Root Allowlist

Root should contain only:

- `.claude/` (private, gitignored)
- `.codex/` (private, gitignored)
- `.github/`
- `.wiki/`
- `plugins/`
- `shared/`
- `registry.json`
- `registry-schema.json`
- `README.md`
- `LICENSE`
- Required meta files (for example `.gitignore`)

Allowed exception:
- `.workers/` is maintained only because it is required for registry publishing infra.

## Private vs Public

- Private control-plane artifacts belong in `.claude/` or `.codex/`.
- Public docs belong in `.wiki/` only.
- The legacy `docs/` directory is retired.
- `LICENSE` remains at root for GitHub license detection.
- Public discoverability mirrors are maintained in `.wiki/License.md` and `.wiki/CHANGELOG.md`.

## Planning and Temp Files

- Planning notes, QA notes, task boards, and temporary run artifacts must stay in `.claude/` or `.codex/`.
- Organize per run/version when needed (for example `.claude/v09/`, `.codex/v09/`).
- Do not place temp, planning, or scratch files at repository root.

## Documentation Policy

- Wiki source of truth is `/.wiki`.
- GitHub Wiki is generated/synced from `/.wiki` by workflow.
- Changelog and license reference pages are maintained in `/.wiki` for public discoverability.
