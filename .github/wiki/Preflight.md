# Served-bytes preflight

The preflight answers one question for the free tier: do the bytes that
`plugins.nself.org` serves match the registry, and can each one be installed
and built from its own tarball? It exists because registry text and old probes
have passed on stale bytes. Install-based acceptance (Epic P7-PLUG, D4) waits
for a fresh report.

## What is measured

For every entry of the served free registry (`/registry.json?tier=free`) and of
`registry.json` at `main`:

1. `GET /plugins/<slug>/tarball` must answer 302 (served source only). The
   per-item HTTP status is recorded, never a `curl -f` summary.
2. The tarball download must answer 200.
3. Its sha256 must equal the registry `checksum`.
4. It must extract safely and contain a `plugin.json` whose `name` and
   `version` equal the registry entry.
5. A service plugin must carry `docker-compose.plugin.yml` and `Dockerfile`.
6. `docker build` of the extracted tree alone must succeed (one build at a
   time; identical bytes are built once).
7. The served registry entry must equal the one at main.

Entries declared `installable:false` with no tarball are asserted, not skipped.

## Failure classes

`404`, `checksum`, `extract`, `manifest`, `fragment`, `build`, `drift`. The
first failing stage names the class. A skipped stage is a skip, and a skip is a
failure (`summary.pass` is `false` whenever any entry is `fail` or `skip`).

## Run it

```bash
bash scripts/preflight/run.sh --tier free --out "$TMPDIR/preflight.json" --md "$TMPDIR/preflight.md"
jq -e '(.entries|length) == .summary.expected' "$TMPDIR/preflight.json"
```

Needs `curl jq tar git docker`. It never writes to production and needs no
licence key. A cold run takes one to two hours. Add `--strict` to exit 1 when
the report is not a full pass, and `--log-dir DIR` to keep failing build logs.
Details: `scripts/preflight/README.md`.

## Read the report

- `summary.expected` is the sum of both registries' own counts; `entries`
  must have exactly that many items.
- `summary.counts` and `summary.by_class` show where the failures sit.
- Each entry has `source` (`served` or `main`), `slug`, `version`, `result`
  (`pass`, `fail`, `skip`), `class`, a fixed `reason`, `http.route`,
  `http.download` and the measured `sha256`.
- The markdown summary names a follow-up per failing entry: served drift goes
  to P7-PLUG-35 (worker deploy), a plugin goes to its batch Ticket
  (P7-PLUG-40 to P7-PLUG-47), an unowned slug needs a new debt id.
- Two runs over the same served bytes are identical once `header` is removed
  (`diff <(jq -S 'del(.header)' a.json) <(jq -S 'del(.header)' b.json)`).

The licensed tier is measured separately (P7-PLUG-39). The latest evidence is
kept in the hq plan evidence folder (`PLUG-02-preflight.md`).
