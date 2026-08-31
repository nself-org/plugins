# CI Matrix

How free plugins get tested automatically, and what does not.

## Two workflows, two jobs

| Workflow | Covers |
|---|---|
| `build-test.yml` ("Go Build & Test (free plugins)") | Compiles, `go vet`s, and `go test`s every `free/*/` directory that has a `go.mod` |
| `registry-check.yml` ("Registry Consistency Check") | Registry/schema consistency: disk-vs-`registry.json` drift, `plugin.json` required fields, version consistency, `bundles[]` shape, plugin counts, and non-Go plugin config validation |

## The 59 / 1 split

As of P6, `free/` holds 60 plugin directories. 59 of them have a `go.mod` and
are built, vetted, and unit-tested by `build-test.yml`'s loop
(`for d in free/*/; if [ -f "$d/go.mod" ]; then ... fi`). Adding a new Go
plugin needs no workflow edit — drop a `go.mod` in `free/<name>/` and the
loop picks it up on the next run.

The 1 exclusion is **`forgejo`** — a docker-compose + shell plugin with no Go
code at all. That is a correct, deliberate exclusion, not a bug in the glob.

**Note on `monitoring`:** `registry.json` labels `monitoring`'s
`language`/`implementation.language` as `"config"`, which reads as if it
were the second non-Go plugin. It is not — `monitoring` has a real
`go.mod`, `cmd/main.go`, `cmd/main_test.go`, and
`internal/render/render.go`, and it IS built and tested by `build-test.yml`
like any other Go plugin. The registry's `language` field for `monitoring`
is stale/wrong; treat the actual `go.mod` presence on disk, not that field,
as the source of truth for which plugins are in the Go matrix.

## Non-Go plugin coverage

`registry-check.yml`'s "Validate non-Go plugin config shape" step is the
config-tier equivalent for whatever plugins genuinely have no `go.mod`
(currently just `forgejo`). It parses every top-level `*.yml`/`*.yaml` file
in that plugin's directory as YAML and fails the job if any file is missing
or malformed. It targets any `go.mod`-less plugin generically, not a
hardcoded name, so it keeps working if the split changes.

**Known gap, stated honestly:** this is syntax validation only — it does not
run the compose stack, does not check Hasura metadata correctness, and does
not exercise the plugin at runtime. There is no generic runtime test harness
for non-Go plugins today. A new non-Go plugin gets this YAML-syntax check
automatically, but nothing deeper, until such a harness exists.

## Plugin counts gate

`registry-check.yml`'s "Verify plugin counts pipeline" step runs
`cli/scripts/plugin-counts.sh` (from `nself-org/cli`, checked out as a
sibling) to compare the free and pro registries against disk. It has two
modes, selected automatically by whether the `PLUGINS_PRO_CHECKOUT_TOKEN`
secret is set:

- **Mode A** (secret present): checks out `nself-org/plugins-pro` and
  `nself-org/cli` as siblings, then runs the full script — free + pro counts,
  and the pro disk-vs-registry drift check.
- **Mode B** (secret absent, current default): the pro-repo checkout is
  skipped (no cross-repo credential exists in this org today); this
  workflow's own "Check all free/ plugins have registry.json entries" step
  already enforces the free-side half of the same guarantee (disk ==
  registry.json, zero drift), and the step logs which mode ran so it is
  never a silent no-op.

Current verified baseline (see `cli/scripts/plugin-counts.sh` output): free
60, pro 127, total 187, zero drift on either side.
