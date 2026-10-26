# D-1483: Golden Path checksum failure

Public read-only investigation on 2026-10-26, against fresh plugins main
`99abe4448118a2f8b30ca49441baa3c28e6aa38a`.

## Evidence

`gh run view 40261854806 -R nself-org/cli --log-failed` reports that step 9,
`nself plugin install ai`, expected
`d2c8c7345babbb367c42fbef9efa1a91e424b401f36452bdcccb005b686599e6`
but downloaded
`03071f9d6c13cca200ca43c2c22dc5ad6c9f722c94d388f89e2a55feb9264598`.

The public `https://plugins.nself.org/registry.json` still advertises the
expected hash for `ai`, including with `?nocache=1`. Its row has version
`1.1.3`, releaseTag `v1.2.10`, tier `max`, requires_license `true`, and
download_url `https://ping.nself.org/plugins/ai/download`.

The public `https://plugins.nself.org/catalog.json` advertises exactly the
hash CI downloaded for licensed `ai` version `1.1.3`. The committed catalog
agrees. Its release_signature is null. This is a proven discrepancy between
the registry and catalog metadata, not proof that either checksum can safely
replace the other without checking the signed release of record.

`ai` is absent from this repository's free registry and releases.json. The
public free release `v1.2.10` has no `ai-*` asset. Both the worker tarball route
and the ping download refuse unauthenticated access (HTTP 401). No credentials
were used to bypass that boundary.

All 130 live free registry entries were downloaded through their advertised
download_url and hashed with SHA-256. All matched the live registry, checked-out
registry, and free catalog entries (catalog comparisons must include tier,
because cron and notify each have a separate licensed entry). All downloads
resolved to GitHub release assets. No free plugin mismatched or failed to fetch.

## Cause and owner action

Licensed `ai` registry/catalog metadata has diverged. Stale licensed registry
data is plausible; public evidence cannot distinguish stale upstream data from
a deployed cache, or establish whether a licensed tarball was rebuilt. The
nocache request did not change the advertised hash. Do not rebuild or replace
release assets to make a checksum pass.

The owner must reconcile the private bundles `ai` release metadata with the
exact bytes its authenticated download serves, preserving or regenerating the
release signature over the chosen published artifact. This investigation cannot
provide a verified private-repository repair command. After that correction is
reviewed and merged, refresh the worker using the existing authorized workflow:

```sh
gh workflow run registry-sync.yml -R nself-org/plugins --ref main
```

Then re-read the public registry and catalog and retry Golden Path:

```sh
gh run rerun 40261854806 -R nself-org/cli --failed
```

These owner commands were not executed. No release asset, worker, KV, secret,
tag, version, or production configuration was changed. The public free-plugin
verification gate prevents a separate class of regression; it cannot repair
this licensed download failure.
