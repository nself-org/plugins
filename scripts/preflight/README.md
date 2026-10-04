# Served-bytes preflight (free tier)

Measures what `https://plugins.nself.org` actually serves for the free plugins,
before any install-based acceptance trusts a registry. P6 probes and registry
text are not evidence; the bytes are. Ticket P7-PLUG-02; wiki page
`.github/wiki/Preflight.md`.

```bash
bash scripts/preflight/run.sh --tier free --out "$TMPDIR/preflight.json" [--md summary.md] \
  [--strict] [--no-build] [--log-dir DIR] [--build-cache DIR] [--served-url URL] [--main-ref REF]
```

Credential-free. HTTP is read-only; the only local side effects are the docker builds described under Docker scope. Requires `curl jq tar awk git docker`
(`timeout`/`gtimeout` optional: caps each build at `PF_BUILD_TIMEOUT`, default 1800 s).
Everything is sequential, one docker build at a time. `--build-cache DIR` keeps build outcomes by tarball sha256 so an interrupted run resumes (never use it for the run you want measured from scratch; the report header and markdown state `build_cache_used`). A docker daemon outage is retried and then aborts the run with exit 2; it is never recorded as a plugin failure. A full cold run builds
every Dockerfile once (identical bytes are built once, not twice) and takes
roughly one to two hours; set `PF_PRUNE_EVERY` (default 10) to change how often
the build cache is trimmed to 4 GB.

## What is covered

Two sources, every entry of each:

| source | entries | tarball taken from |
|---|---|---|
| `served` | `registry.json?tier=free` from the served host | `GET <host>/plugins/<slug>/tarball` (the 302 target) |
| `main` | `registry.json` at `--main-ref` (default `origin/main`) | the entry's own `tarball` URL |

`summary.expected` is the two registries' own counts added (`pluginCount.free`,
`plugins_count`). `run.sh` exits 3 when the measured entries differ from it.

## Per-entry stages and classes

The first failing stage names the class.

| class | meaning |
|---|---|
| `404` | route answered other than 302, no URL, or a download answered other than 200 (per-item codes are in `http`) |
| `checksum` | sha256 of the downloaded bytes differs from the registry's `checksum` (or none is present) |
| `extract` | not a gzip tarball, an absolute or `..` member path, or `tar -x` failed |
| `manifest` | no `plugin.json`, not a JSON object, or `name`/`version` differ from the registry entry |
| `fragment` | a service plugin (`pluginType` is not `cli`, no bare `binaryName`) lacks `docker-compose.plugin.yml` or `Dockerfile` |
| `build` | `docker build` of the extracted tree alone failed (or a CLI plugin has no `Dockerfile`) |
| `drift` | served registry differs from `registry.json` at main (version, checksum, or slug absent). Checked first for every entry and it wins: an entry that also fails another stage is reported as `drift` with `; also <class>: <reason>`, so P7-PLUG-35 routing is never masked |

`installable:false` entries with no `tarball` are asserted (declared, nothing to
download) and pass. Anything not measured is `skip`, and a skip is not a pass
(Constitution 8.4): `--no-build` turns every build into a skip, so it can never
report `pass: true`.

## Report

`--out` is JSON with sorted keys: `header` (`run_at`, `served_url`, `main_ref`,
`main_sha`), `summary` (`expected`, per-source expected, `counts`, `by_class`,
`pass`) and `entries` sorted by `(source, slug)` with `result`, `class`,
`reason`, `http`, `sha256`. Everything except `header.run_at` is a function of
the served bytes, so two runs over the same bytes are identical after
`jq -S 'del(.header)'`. `summary.pass` is true only when no entry is `fail` or
`skip`. The markdown (`--md`, default next to `--out`) lists every non-pass
entry with its follow-up: served drift goes to P7-PLUG-35, a plugin goes to its
batch Ticket (`owners.tsv`, P7-PLUG-40..47), an unowned slug needs a new debt id.

Exit status, read this before gating on it: **0 means the report was written,
not that the tier passes.** Gate on `--strict` (exit 1 when `summary.pass` is
false) or on `jq -e '.summary.pass'`, never on the plain exit status alone.
2 is a usage or infrastructure error: `--out` and `--md` are deleted at the
start of every run, so a stale report is never left behind. 3 is a coverage
failure: an empty registry, or fewer entries measured than a registry counts or
lists as installable (`summary.coverage`, `summary.by_class.coverage`); zero
entries can never pass.

## Docker scope

No global prune ever runs. The run creates a buildx builder
`nself-preflight-<run-id>` and labels every image `nself.preflight=<run-id>`;
cleanup removes only that builder (and its build cache), the labelled images and
a buildkit image it had to pull. Without buildx it uses the default builder and
never prunes the build cache. The run id is in `header.run_id`.

## Determinism limit

`reason` strings quote docker output (for example the Go version a Dockerfile
image runs), so identical served bytes give identical reports only while the
upstream base-image tags resolve to the same content.

## Not measured

Compose fragment validity (`docker compose config`), image boot, signatures
(P7-PLUG-23/69), per-platform binary tarballs, and the licensed tier
(P7-PLUG-39). A `docker build` runs on the host architecture only.
