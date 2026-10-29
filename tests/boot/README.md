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
- `legacy-probe.sql` — SQL run on the legacy database after the converted boot and the rename checks (legacy pass only); a failing statement fails the pass, a pass line ends with `probe ok`. Requires `legacy-renames`: the conformance rule `legacy-renames` fails a probe that would never run. The same rule requires `legacy-renames` to list exactly the table renames the plugin's migrations perform (and the `legacyTables` list in its Go source, when it has one).
- `seed.sql` — rows written through the basis schema before the upgrade.
- `basis-from` — image ref to rewrite the basis Dockerfile's first FROM line.
- `basis-absent` — manifest tables the basis build never created (not seeded or verified before upgrade).
- `extra-tables` — Additional qualified table names (one per line, # comments allowed) to include in existence and row-survival checks. Use this for tables removed from `.tables` in plugin.json but still present in seed.sql.

Row survival is checked by identity (primary-key values); tables without a primary key fall back to row count.

Both kits start a Redis sidecar for job-queue and wait for `PING` before booting the plugin. The sidecar has no published host port; `free/job-queue/tests/boot/env` selects it through the network alias `redis`. `bash tests/boot/redis_test.sh` checks sidecar selection and failure handling without Docker.

Cleans all containers, networks and images it creates. Requires docker and jq.
