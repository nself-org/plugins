# Boot Test Harness

Boot migrations and table-upgrade tests for nSelf plugins. Two test suites:

## `run.sh`

Fresh-boot verification: ensures a plugin boots cleanly on an empty database with all migrations applied.

## `tables-upgrade.sh`

Three-pass verification per plugin:

1. **Fresh pass**: build the converted plugin, boot on empty database, verify migrations and manifest tables.
2. **Legacy pass** (optional): run legacy.sql fixtures, boot converted image, verify legacy table renames.
3. **Upgrade pass**: boot basis (pre-conversion) plugin, seed rows, boot converted image, verify rows survive.

### Fixtures (optional, under `free/<plugin>/tests/boot/`)

- `env` — KEY=value lines passed to every boot.
- `legacy.sql` — TS-era unprefixed tables with fixture rows (legacy pass only).
- `legacy-renames` — `old new` pairs for legacy table renames (legacy pass verification).
- `seed.sql` — rows written through the basis schema before the upgrade.
- `basis-from` — image ref to rewrite the basis Dockerfile's first FROM line.
- `basis-absent` — manifest tables the basis build never created (not seeded or verified before upgrade).
- `extra-tables` — Additional qualified table names (one per line, # comments allowed) to include in existence and row-survival checks. Use this for tables removed from `.tables` in plugin.json but still present in seed.sql.

Row survival is checked by identity (primary-key values); tables without a primary key fall back to row count.

Cleans all containers, networks and images it creates. Requires docker and jq.
