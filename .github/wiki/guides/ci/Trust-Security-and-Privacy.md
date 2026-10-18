# CI Trust, Security and Privacy

How `nself-ci` decides whether a result can be believed, and what it keeps off the network. Source: `free/ci/internal/attest`.

## Evidence

A run produces one evidence document (`ci.evidence/v1`): the result, every check, who produced it and which inputs it saw. A gate never trusts a result by name. It reads the document and judges it.

### Evidence binding

Evidence gates one exact revision, and only when it is bound:

- `revision` is the full commit id (40 or 64 hex digits).
- `binding.state` is `bound` and `worktree_state` is `clean`. A dirty worktree never gates, even if every check passed.
- `repo_id` and `policy_digest` match the gate. The gate builds both from its own protected policy and checkout, never from the document.
- Every required job is present and every check in it passed. The gate takes the required job list from the protected policy, never from the revision under test, so a change cannot remove its own required job.
- Ad hoc and drill pipelines never gate. Only the `default` and `named` pipeline kinds do.

### Freshness

Freshness is computed when the document is read. The `freshness` field stored in a document is ignored.

The gate compares the digests bound in the evidence with the digests of the inputs now: the effective config, each lockfile and each declared generated file. A changed, added or removed path makes the evidence `stale`. If the current digests are not supplied, or the evidence binds no config digest, the state is `unknown`. Unknown never satisfies a gate.

### Producer and trust class

| `producer.kind` | Meaning | Gate result |
|---|---|---|
| `coordinator-recorded` | The coordinator wrote it from its own store | Accepted |
| `signed-remote` | A remote runner signed it | Accepted only if a remote verifier is registered and the signature checks. With none registered it fails closed |
| `self-reported` | The job wrote it about itself | Accepted only by a default-class gate whose operator policy admits it. Never by a release gate |

Read documents from the coordinator's own store, never from a job's worktree. A file in a worktree proves nothing about who wrote it.

`source_trust` ranks `untrusted` < `collaborator` < `internal` < `owner`. A gate sets a minimum. An unknown class on either side never passes.

## GateAccept

`GateAccept(evidence, gate)` returns every reason the evidence fails, in a fixed order. An empty list means accepted. It is pure and fails closed: an empty or unknown gate field never matches, and a zero `Gate` accepts nothing. `GateAcceptDocument` decodes raw bytes and judges them in one step, so a signature verifier sees the exact bytes. A document that repeats a JSON key is refused.

A gate has a class. `default` is ordinary. Any other value is treated as `release`, which demands `selection.mode` of `full` and refuses self-reported evidence. A default gate can ask for a full selection too.

`Verify(doc)` is the local check. It proves a document is well formed and does not contradict itself (schema, strict decode, revision format, binding against worktree state, producer against signature kind, counts, result against checks). It does not prove who wrote it.

## Reason keys

A refusal is error `E685` ("evidence rejected"). Its reason keys are API: a consumer may branch on them. A key is never renamed or reused.

| Key | Meaning |
|---|---|
| `evidence.stale` | The inputs changed since the run, or freshness is unknown |
| `evidence.unbound` | Binding is not `bound`, or the worktree was not clean |
| `evidence.adhoc` | The pipeline was ad hoc or a drill |
| `evidence.repo_mismatch` | The evidence is about another repository |
| `evidence.policy_mismatch` | The policy digest differs from the gate's |
| `evidence.jobs_mismatch` | A required job is missing, or the gate requires none |
| `evidence.trust_below_min` | `source_trust` is below the gate minimum |
| `evidence.self_reported_not_admitted` | Self-reported evidence on a release gate, or not admitted by policy |
| `evidence.signature_unverifiable` | A signed-remote document has no verifier, or the check failed |
| `evidence.revision_mismatch` | The evidence is about another revision |
| `evidence.selection_partial` | The gate needs a full selection and the run was partial |
| `evidence.not_passing` | The result, or a required job's check, is not `pass` |
| `evidence.invalid` | The document is malformed or contradicts itself |

Fix: run the gate again on the exact revision with a clean worktree, or fix the reason named in the error.

## Publication profile

Evidence about a public repository may leave the machine, so it passes through the publication profile first (`Publish`, `PublishWith`).

- Runner and coordinator ids become stable pseudonyms. A pseudonym is stable inside one repository and differs between repositories.
- Private and local addresses, host names, user names, home paths and private URLs disappear from every free-text field: check excerpts, job names, paths, tool versions and provider attestation.
- A published URL keeps only `http` or `https`, with no credentials, query or fragment. A private or local host drops the whole URL.
- The host passes its own host name and user name, so a log excerpt cannot leak them even when they carry no private suffix.

The profile fails closed. Only the exact visibility `private` turns it off. An empty or unknown visibility scrubs, and so does a zero `Profile`.

A published copy no longer matches the original's signature. It is a report, not a gate input, and its freshness is `unknown`.

### Commit status

The status posted to a code host is built from the published evidence, so the description and `target_url` hold nothing the profile removes. A repository with unknown visibility is treated as public.

## See also

- [Adopting the nself-ci Gate](../Nself-CI-Adoption)
