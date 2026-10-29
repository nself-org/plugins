# Evidence and machine output

<!-- BEGIN GENERATED:evidence-fields -->
#### Evidence document (root)

| Field | Type | Required | Values |
|---|---|---|---|
| `schema` | string | yes |  |
| `revision` | string | yes |  |
| `repo_id` | string | yes |  |
| `run_id` | string | yes |  |
| `pipeline` | `PipelineRef` | yes |  |
| `job` | string or null | no |  |
| `attempt` | `AttemptRef` or null | no |  |
| `trigger` | string | yes | `push`, `pull_request`, `tag`, `manual`, `schedule`, `api`, `local` |
| `source_trust` | string | yes | `owner`, `collaborator`, `internal`, `untrusted` |
| `policy_digest` | string | yes |  |
| `input_digests` | `InputDigests` | yes |  |
| `worktree_state` | string | yes | `clean`, `dirty` |
| `tracked_changes_digest` | string or null | yes |  |
| `binding` | `Binding` | yes |  |
| `runner` | `RunnerRef` | yes |  |
| `producer` | `Producer` | yes |  |
| `timings` | `Timings` | yes |  |
| `tool_versions` | map of string | yes |  |
| `checks` | list of `Check` | yes |  |
| `artifacts` | list of `Artifact` | yes |  |
| `retries` | list of `Retry` | yes |  |
| `selection` | `Selection` | yes |  |
| `placement_ref` | string or null | no |  |
| `result` | string | yes | `pass`, `fail`, `error`, `cancelled` |
| `counts` | `Counts` | yes |  |
| `freshness` | `Freshness` | yes |  |
| `signature` | `Signature` | yes |  |
| `provider_attestation` | `ProviderAttestation` or null | no |  |
| `attempts` | list of `AttemptEvidence` | no |  |
| `errors` | list of `EvidenceError` | no |  |

#### `PipelineRef`

| Field | Type | Required | Values |
|---|---|---|---|
| `id` | string | yes |  |
| `name` | string | yes |  |
| `kind` | string | yes | `default`, `named`, `drill`, `adhoc` |

#### `AttemptRef`

| Field | Type | Required | Values |
|---|---|---|---|
| `id` | string | yes |  |
| `n` | integer | yes |  |

#### `InputDigests`

| Field | Type | Required | Values |
|---|---|---|---|
| `config` | string | yes |  |
| `lockfiles` | list of `DigestRef` | yes |  |
| `generated` | list of `DigestRef` | yes |  |

#### `Binding`

| Field | Type | Required | Values |
|---|---|---|---|
| `state` | string | yes | `bound`, `unbound` |
| `reason` | string or null | yes | `dirty`, `sha_mismatch` |

#### `RunnerRef`

| Field | Type | Required | Values |
|---|---|---|---|
| `id` | string | yes |  |
| `provider` | string | yes |  |
| `platform` | string | yes |  |
| `isolation` | string | yes | `process`, `container`, `sandboxed-container`, `vm`, `ephemeral-vm`, `hosted-disposable` |

#### `Producer`

| Field | Type | Required | Values |
|---|---|---|---|
| `kind` | string | yes | `coordinator-recorded`, `signed-remote`, `self-reported` |
| `version` | string | yes |  |

#### `Timings`

| Field | Type | Required | Values |
|---|---|---|---|
| `queued_at` | string or null | yes |  |
| `started_at` | string or null | yes |  |
| `finished_at` | string or null | yes |  |
| `duration_ms` | integer or null | yes |  |

#### `Check`

| Field | Type | Required | Values |
|---|---|---|---|
| `id` | string | yes |  |
| `job_id` | string | yes |  |
| `kind` | string | yes |  |
| `tool` | string | yes |  |
| `tool_version` | string or null | yes |  |
| `required` | boolean | yes |  |
| `substantive` | boolean | yes |  |
| `allow_failure` | boolean | no |  |
| `result` | string | yes | `pass`, `fail`, `skip`, `error` |
| `reason` | string | no | `pipeline.excluded`, `selection.no_base`, `selection.full_required`, `selection.affected`, `when.branch`, `when.paths`, `when.trigger`, `when.env_missing`, `dep.failed`, `fail_fast.cancelled`, `unplaceable.platform`, `unplaceable.docker`, `unplaceable.tool`, `unplaceable.service`, `unplaceable.label`, `unplaceable.resources`, `admission.battery`, `admission.reservation`, `advisory.tool_missing`, `advisory.data_offline` |
| `failure_class` | string | no | `code`, `infra`, `timeout`, `cancelled`, `lost` |
| `findings` | integer | yes |  |
| `excerpt` | string | yes |  |
| `log_ref` | string or null | yes |  |
| `exit` | integer or null | no |  |
| `diagnosis` | string | no |  |
| `remediation` | string | no |  |
| `flaky` | `Flaky` or null | no |  |

#### `Artifact`

| Field | Type | Required | Values |
|---|---|---|---|
| `path` | string | yes |  |
| `sha256` | string | yes |  |
| `size` | integer | yes |  |
| `committed` | boolean | yes |  |

#### `Retry`

| Field | Type | Required | Values |
|---|---|---|---|
| `job_id` | string | yes |  |
| `attempt` | integer | yes |  |
| `class` | string | yes | `code`, `infra`, `timeout`, `cancelled`, `lost` |
| `diagnosis` | string | no |  |
| `excerpt` | string | no |  |

#### `Selection`

| Field | Type | Required | Values |
|---|---|---|---|
| `mode` | string | yes | `full`, `affected` |
| `reason` | string | yes | `pipeline.excluded`, `selection.no_base`, `selection.full_required`, `selection.affected`, `when.branch`, `when.paths`, `when.trigger`, `when.env_missing`, `dep.failed`, `fail_fast.cancelled`, `unplaceable.platform`, `unplaceable.docker`, `unplaceable.tool`, `unplaceable.service`, `unplaceable.label`, `unplaceable.resources`, `admission.battery`, `admission.reservation`, `advisory.tool_missing`, `advisory.data_offline` |
| `digest` | string | yes |  |

#### `Counts`

| Field | Type | Required | Values |
|---|---|---|---|
| `pass` | integer | yes |  |
| `fail` | integer | yes |  |
| `skip` | integer | yes |  |
| `error` | integer | yes |  |
| `total` | integer | yes |  |

#### `Freshness`

| Field | Type | Required | Values |
|---|---|---|---|
| `state` | string | yes | `current`, `stale`, `unknown` |
| `reason` | string or null | yes |  |

#### `Signature`

| Field | Type | Required | Values |
|---|---|---|---|
| `kind` | string | yes | `coordinator-recorded`, `runner-ed25519` |
| `key_id` | string or null | no |  |
| `value` | string or null | no |  |

#### `ProviderAttestation`

| Field | Type | Required | Values |
|---|---|---|---|
| `provider` | string | yes |  |
| `run_id` | string | yes |  |
| `run_attempt` | integer | yes |  |
| `ref` | string | yes |  |
| `workflow_path` | string | yes |  |
| `workflow_sha` | string | yes |  |
| `workflow_digest` | string | yes |  |
| `template_version` | string | yes |  |
| `nonce` | string | yes |  |
| `artifact_id` | string | yes |  |
| `artifact_sha256` | string | yes |  |
| `verified_at` | string | yes |  |

#### `AttemptEvidence`

| Field | Type | Required | Values |
|---|---|---|---|
| `job_id` | string | yes |  |
| `attempt` | integer | yes |  |
| `runner` | `AttemptRunner` | yes |  |
| `timings` | `AttemptTimings` or null | no |  |

#### `EvidenceError`

| Field | Type | Required | Values |
|---|---|---|---|
| `code` | string | yes |  |
| `job_id` | string | no |  |
| `attempt` | integer | no |  |
| `detail` | string | no |  |

#### `DigestRef`

| Field | Type | Required | Values |
|---|---|---|---|
| `path` | string | yes |  |
| `sha256` | string | yes |  |

#### `Flaky`

| Field | Type | Required | Values |
|---|---|---|---|
| `source` | string | yes |  |
| `passes` | integer | yes |  |
| `fails` | integer | yes |  |

#### `AttemptRunner`

| Field | Type | Required | Values |
|---|---|---|---|
| `kind` | string | yes |  |
| `tier` | string | yes |  |
| `pool` | string | yes |  |
| `rank` | integer | yes |  |

#### `AttemptTimings`

| Field | Type | Required | Values |
|---|---|---|---|
| `queue_ms` | integer or null | no |  |
| `transfer_ms` | integer or null | no |  |
| `run_ms` | integer or null | no |  |
<!-- END GENERATED:evidence-fields -->
