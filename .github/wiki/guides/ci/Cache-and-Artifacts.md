# Cache and artifacts

This guide covers what `nself ci` keeps between jobs and between runs: the dependency cache, job artifacts, the coordinator's store for remote nodes, signed releases, and the supply chain evidence a release carries. It ends with the threats the design covers and where the performance evidence lives.

Command names, flags, exit codes and error codes are not repeated here. They come from the generated [CI reference](Reference.md). Read [the command table](Reference.md#commands) first, [the exit codes](Reference.md#exit-codes) for what a status means, and [the error codes](Reference.md#error-codes) for every `E7xx` code named below. The cache and artifact codes are E684 and E711 to E719.

All of it needs `NSELF_V15=1`. The command blocks in this guide use `text` fences, not runnable ones, because they need an installed `nself` and a real store.

## Cache

The cache holds downloads and compiler caches that a package manager refills on every run. It never holds build output. A cargo `target/` directory, for example, is never cached.

### Keys

A cache key is a sha256 over length-prefixed fields: the namespace, the spec id and version, the platform, the isolation level, the toolchain (the image digest for container jobs, probed tool versions for process jobs), a digest of the allowed environment, the paths, the lockfile digests and any extras. Environment values are hashed and never stored.

Lookup is by exact key. There is no prefix match and no restore-key fallback. A changed lockfile is a miss, not a near hit.

### Namespaces: per trust and per PR

Every entry lives in a namespace made of the project, the trust class of the source, whether the ref is protected, and, for untrusted code only, the head repository and the pull request. Two pull requests never share a namespace. A fork never shares one with the default branch.

Trust classes come from facts the forge reports. See [source trust](Trust-Security-and-Privacy.md#source-trust-who-made-the-code).

### Access modes

| Mode | Meaning |
|---|---|
| `rw-own` | Read and write the job's own namespace. |
| `ro-snapshot` | Restore only, from a protected namespace of the same project and a strictly higher trust class. |
| `none` | Everything else, including any attempt that has ended. |

A lower class may read the snapshot of a higher class. It never writes it. A refused write reports E684 with a reason key such as `cache.namespace`, `cache.epoch`, `cache.attempt_state`, `cache.result` or `cache.mode`. The keys are listed under [reason codes](Reference.md#reason-codes).

### Snapshot rules

An entry can serve as a read-only snapshot only when its writer:

- declared no secrets,
- passed the scan for registered secret values before the save,
- belongs to a project with no private dependencies.

Each fact is a positive record made at save time. A missing or unknown fact means the entry is not eligible.

### When a save happens

A save commits only from a passing attempt, in the `finalizing` state, with the current lease epoch, into its own namespace and a planned key. Anything else is refused with E711. A restore re-hashes every blob. A corrupt or missing blob drops the entry and reads as a miss.

### Config

The cache switches on from the detected stack. A project can turn it off, or narrow it per job, in `.ci.yaml`:

```yaml
cache:
  disable: false
jobs:
  test:
    cache:
      disable: false
      keys: [extra-key-input]
      paths: [.cache/tool]
```

`NSELF_CI_CACHE_DISABLE` turns it off for one run or one job through the environment.

### Caps

Three environment variables bound the store. Defaults are set in the code and shown by `nself ci artifacts --cache list`.

| Variable | Bounds |
|---|---|
| `NSELF_CI_CACHE_MAX_BYTES` | Total cache bytes. Pruning evicts the least recently used entry first. |
| `NSELF_CI_CACHE_MAX_AGE` | How long an unused entry stays. |
| `NSELF_CI_STORE_MIN_FREE` | The free-space floor of the store volume. |

An unparsable value keeps the default and prints one warning. Inspect and trim the cache with:

```text
nself ci artifacts --cache list
nself ci artifacts --cache show --run <run>
nself ci artifacts --cache verify
nself ci artifacts --cache prune --dry-run
```

`verify` re-hashes every blob and drops nothing. `prune` needs the admin role, because the byte cap is store wide.

## Artifacts

### Declaring outputs

A job names the outputs it must produce. The attempt cannot pass until every declared output is committed:

```yaml
jobs:
  package:
    artifacts:
      - path: dist/app.tar.gz
        name: app
        kind: package
        retention: 30d
        visibility: project
```

`kind` is one of package, binary, archive, image, report, coverage, log, benchmark, sbom or other. `visibility` is `project` or `restricted`. A path must exist inside the job workdir. The collector refuses a file above `NSELF_CI_ARTIFACT_MAX_FILE`, a total above `NSELF_CI_ARTIFACT_MAX_BYTES`, a low disk floor, and any file that holds a registered secret value (E712).

Every blob is re-hashed from the stored bytes before the attempt may leave `finalizing`. Two attempts that write the same path make two records. Neither overwrites the other.

### Images are OCI tarballs

The fabric never pushes an image. Declare an image output as an OCI layout tarball, for example from `docker buildx build --output type=oci,dest=app.oci.tar`. The collector reads the index digest and the per-platform manifest digests from the tarball without extracting it or contacting a registry.

### List, show, get and prune

```text
nself ci artifacts list --run <run>
nself ci artifacts show <id>
nself ci artifacts get <id> --dest <dir>
nself ci artifacts prune --dry-run
```

`get` writes only into a fresh, empty directory outside any git tree. A destination that is the working directory, not empty or under `.git` is refused (E683). An artifact you may not see reads as not found, the same as one that does not exist (E713). `prune` never touches release records unless you pass `--release`.

## Remote nodes

A remote node never reads the coordinator's disk. The coordinator hosts one store, and nodes get and give blobs through the lease.

- **Hand-off.** A remote attempt sends its artifacts to the coordinator over the lease's carrier. Each blob is announced with its digest and size, accepted in chunks, and published only when the whole-blob digest matches. A restarted sender resumes from the bytes already held.
- **Refusals come first.** The file cap, the lease byte budget, the disk floor and the lease epoch are all checked before a byte is stored. A stale epoch is refused on every write and at close. These failures report E714.
- **Read-through copies.** A node keeps a bounded copy of the cache blobs it fetched, isolated per realm. Every use re-verifies the digest. A corrupt copy is deleted and fetched again. Eviction runs at lease start and before a new copy is published.
- **Locality.** The scheduler is told how many cache bytes each runner is missing for each job, so work lands where its caches already are. Read [scheduling and cost](Scheduling-and-Cost.md) for how that score weighs against load.

## Release workflows

### Release jobs

A job with `kind: release` is admitted only when the evidence of its dependencies passes the release gate. The dependencies must have passed on a clean, committed tree, for the protected identity of the run. An untrusted source is never promoted (E715).

A release job receives `NSELF_CI_IDEMPOTENCY_KEY`. CI never gives a release job deploy credentials. A job that declares a variable starting with `DEPLOY_` or `NSELF_DEPLOY_` is refused.

### Idempotency and `runs resolve`

A release job runs at most once per run. If an attempt is killed after it started publishing, the run stops in `needs-operator`. Nothing retries it for you, because the outside world may already have changed.

You decide. Find the attempt id with `nself ci runs <run>`. The key is the one the job received, which its publish step logs:

```text
nself ci runs resolve <attempt> --idempotency-key <key> --rerun
nself ci runs resolve <attempt> --idempotency-key <key> --abandon --reason "checked the registry"
```

A wrong key is refused before anything is written (E716). A release attempt that has reached `finalizing` can only be abandoned.

### Publication

After a passing release run on a PERSONAL coordinator, the fabric builds the release directory itself: the artifacts, checksums, the evidence of the run, SBOMs, a SLSA provenance statement and a release manifest. It signs the provenance and the manifest with the release key, verifies its own output, and records the result.

Publication is deterministic. Running it again writes the same bytes and signs nothing new when the release is intact. That is also the recovery path:

```text
nself ci artifacts sign <run>
```

Run it when a publication failed or was killed after the evidence was recorded. It completes the release. It refuses a digest that is not a committed record of the run (E717).

### Export

```text
nself ci artifacts release <run> --out <dir>
```

The export goes into a fresh empty directory, is verified again, and is private operator material. It contains `evidence.json` with raw runner ids, so a public project does not publish that file.

### Recovery

| Symptom | What to do |
|---|---|
| The run is `needs-operator` | `runs resolve` with the attempt id and key, as above. |
| Publication failed or was killed | `artifacts sign <run>`. It is safe to repeat. |
| No release key | `artifacts key init` once (E717). |
| SBOM tool missing | `artifacts tools install`, then retry (E718). |
| Verification failed | Read the reason key. `artifacts verify <dir> --json` lists every check (E719). |

### TEAM: signing on the operator's machine

A TEAM coordinator holds secret references only, so it never signs. The release is signed on the operator's own machine:

```text
nself ci artifacts sign <run> --coordinator https://<host> --out <dir> --project <checkout>
```

The command fetches the run's release inputs and blobs from the coordinator with GET requests only, re-hashes every blob, rebuilds the provenance and manifest with the same code the coordinator uses, signs with the operator's own release key, and writes the verified directory to `--out`. The key stays in that process. Nothing is uploaded back. A member cannot do this for inputs they may not see (E687).

### The Android lane on a laptop

`nself ci build --artifact android` is a release pipeline on the operator's own machine. The project's required jobs run first. Then the `android` release job runs, using the project's own job of that name or the synthesized `android-release` pipeline. The command prints the APK path and the release directory.

The keystore reaches the job only as the four `release/ANDROID_*` secret references, filled from the operator's environment on this machine. Any other node is refused (E677). The job decodes the keystore inside its own workspace and removes it when the script ends. Gradle runs with `--no-daemon`, so nothing outlives the attempt holding the keystore.

Upload is a separate, explicit step and never overwrites an existing release asset:

```text
nself ci build --artifact android --upload --tag <tag> --owner <owner> --repo <repo>
```

The APK uploaded is the verified copy in the release directory.

### The deploy hand-off

CI builds and signs. It does not deploy. A deploy tool outside CI decides what to trust. Print the public trust line for the release key:

```text
nself ci artifacts key
```

The line is `<key-id> <base64 public key>`. Commit it to `.nself/trust/ci-release.keys` on the protected default branch. A revoked key id goes in `.nself/trust/ci-release.revoked`. The operator's own copies live under `$NSELF_CI_HOME/trust/` and apply too. The deploy side then runs `nself ci artifacts verify` against the release and deploys only on exit 0.

## Supply chain

### One key per purpose

Signing keys are per purpose. The release key has purpose `ci-release` and an id of the form `ci-release-<16 hex>`. A manifest signed by a key of another purpose, such as a plugins or agent key, is refused (E719 `key_purpose`).

```text
nself ci artifacts key init
nself ci artifacts key
nself ci artifacts key rotate --yes
```

- `key init` creates the key once. It refuses when one exists.
- `key` prints the public trust line. Both values are public.
- `key rotate` replaces the key, prints the new trust line and prints the old key id for the `.revoked` file. It asks for confirmation unless you pass `--yes`.

### Signer isolation, and its limit

Every key operation runs in a separate signer process that the coordinator starts after the run. The signer sits outside every job's process group, namespace and container. It resolves the key by reference, uses it in memory and zeroes it. The key never reaches argv, stdout, a file or a child's environment. Before it signs, it re-checks the request against the stored run: the revision, the repository, the passed release attempt and every digest. One failing item fails the whole request, and the key is not touched.

The limit: the signer runs as the same operating-system user as the coordinator. It keeps jobs away from the key. It does not keep away another process that runs as that user with enough rights to read memory or files. Anyone who can act as the coordinator's user can reach the key. Treat that user as the trust boundary, and see [Trust, security and privacy](Trust-Security-and-Privacy.md) for the isolation levels. A TEAM coordinator avoids the question: it never holds the key.

### SBOM formats

Each release artifact gets two SBOMs from one pinned syft run: CycloneDX 1.6 JSON and SPDX 2.3 JSON, side by side. Both pass a structural check before they are committed. The pinned syft is placed by an explicit step. No job downloads a tool.

```text
nself ci artifacts tools install
```

The pinned version is in [the pinned versions table](Reference.md#pinned-versions). The download is checked against a recorded sha256, and a mismatch installs nothing (E718).

### Provenance fields

The provenance is an in-toto Statement v1 with a SLSA provenance v1 predicate, written as canonical JSON so the same facts give the same bytes.

| Field | Content |
|---|---|
| `subject` | Each released artifact by name and sha256. |
| `buildDefinition.buildType` | The release build type. |
| `externalParameters` | Repository, revision, pipeline, job and config digest. |
| `internalParameters` | Isolation level and the `nself ci` version. |
| `resolvedDependencies` | The source git commit and the lockfile digests. |
| `runDetails.builder.id` | The builder. For a repository not known to be private, it is pseudonymous. |
| `runDetails.metadata` | The invocation id, `<run id>/<attempt id>`, and the start and finish times. |
| `runDetails.byproducts` | The digest of `evidence.json`. |

Building refuses a private address, host name, user name or home path in a public release. It never rewrites a name, because a subject must stay equal to its manifest entry.

### Verify

```text
nself ci artifacts verify <dir>
nself ci artifacts verify <run> --json
```

`verify` takes a release directory, its manifest or a run id. It writes nothing. The trust roots are the operator key files and the protected default branch at the pinned tip. The working tree and the released revision are never trusted. A pin that cannot be resolved fails the check. It never drops the branch's keys and revocations.

Exit 0 means verified. Exit 11 means invalid, and the reason key names the check that failed. Exit 1 means the run's evidence is failing or quarantined. The `--json` result always carries `slsa_build_level: "L1"`. Exit codes are in [the exit code table](Reference.md#exit-codes). An invalid result reports E719.

### The SLSA level: Build L1

Every published release, and every agent release, is SLSA Build L1. That is the whole claim, and the code cannot name another level.

Build L1 means the provenance exists, says how the artifact was built, and is available to the consumer. This fabric meets that and no more.

Build L2 asks for a hosted build service that signs the provenance itself, so the producer of the build cannot forge it. Build L3 asks for a build platform that isolates each build from the others and keeps the signing secret out of reach of the build steps. Neither is the case here, for these reasons:

- The coordinator is the operator's own machine or server. It is not a separate hosted service with its own identity.
- The signing key is held by a process of the same operating-system user as the coordinator (see the limit above). A build with that user's rights could reach it.
- On a PERSONAL coordinator and in the Android lane, the build and the signer share one machine owned by the person who ships the release.
- Job isolation depends on the level you configure. Not every level separates builds from each other to the degree a higher level needs.

So the output is a signed claim by the operator that this run built this revision. Verification checks the signature, the digests and the trust roots. It does not turn the operator into an independent builder.

## Threats covered

The cache, artifact and release threats are tested as one matrix in `free/ci/test/cachesec`, and mapped to rows of the project threat model. Each case lists the code, the reason key and the audit event it must produce.

| Threat row | What it covers | Codes in the matrix |
|---|---|---|
| TM-12 | A fork or another pull request poisons a namespace it does not own. A secret reaches a snapshot. | E684, E711 |
| TM-13 | A write from an ended, stale-epoch or failed attempt, local or remote. | E711, E714 |
| TM-14 | A crafted archive or destination: traversal, absolute paths, links, FIFOs, devices, zip bombs, `get` into a non-empty or git directory. | E683 |
| TM-15 | A corrupt, partial, overwritten or substituted blob, at rest or in transit. | E711, E712, E714 |
| TM-16 | A registered secret, raw or base64, saved into a cache or artifact. | E711, E712 |
| TM-17 | Release of an untrusted source or of failing dependencies. | E715 |
| TM-18 | A manifest signed with the wrong key purpose, or a sign request for an uncommitted digest. | E717, E719 |
| TM-20 | A revoked key. | E686 |
| TM-24 | A member or a job reaching signing inputs it is not allowed. | E687 |

Look up each code in [the error codes](Reference.md#error-codes). Every refusal writes an audit row, listed under [reason codes](Reference.md#reason-codes) where it has a stable key.

## Performance

The fabric measures cache restore and save time with a benchmark harness in `free/ci/bench/cache`. It covers key derivation, blob store cost, archive pack and unpack, and an end-to-end cold and warm restore, and it emits `perfbench/v1` JSON. The acceptance ceilings for restore and save are in [`perf-budget.json`](../../../../free/ci/bench/cache/perf-budget.json). The recorded per-path numbers are regression guards, not ceilings, in [`perf-regression-guards.json`](../../../../free/ci/bench/cache/perf-regression-guards.json); a pass against the guards does not mean the ceilings are met. No figure is copied into this page. Read the measured results from the evidence of the benchmark run, so the page cannot go stale.
