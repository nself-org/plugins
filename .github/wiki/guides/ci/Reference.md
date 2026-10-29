# CI reference

<!-- Source: scripts/generate-ci-agent-reference.py; do not edit this page directly. -->

This page covers the pull agent and its refusal and transport reasons. It is a focused reference pending the full CI reference generator.

Regenerate with `python3 scripts/generate-ci-agent-reference.py`; validate with `python3 scripts/generate-ci-agent-reference.py --check`.

## `nself-ci-agent pull`

After `nself-ci-agent join`, keep an outbound TLS 1.3 mTLS connection to the enrolled coordinator and execute its leases. The enrollment pins the coordinator root; `--coordinator` selects its local record ID, not a URL.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--slots` | `0` | lower the concurrent leases this process takes |
| `--coordinator` | `""` | id of the coordinator (default: derived from the pinned root) |
| `--project` | `""` | project root whose secret store this node resolves from (enables node-side secrets) |
| `--secrets-eligible` | `false` | this node may receive project and environment secrets |
| `--dedicated` | `false` | this node is dedicated to the project (also release and deploy secrets) |

A slots value of zero keeps the automatic limit; a positive value lowers it. Negative values are rejected. Secret eligibility requires the project secret store on this node. `--dedicated` also permits release and deploy secrets.

## E661: job token rejected

The agent must present its own lease token using the certificate and session that first used it. Restart the job when its token expires or the attempt ends. Never copy a token between jobs, nodes, certificates or sessions.

| Reason |
| --- |
| `expired` |
| `invalid` |
| `missing` |
| `replayed` |
| `terminal` |
| `wrong_certificate` |
| `wrong_job` |
| `wrong_runner` |
| `wrong_session` |

## E656: pull transport failure

These keys describe the pull carrier. Check node connectivity and enrollment for connection/configuration failures, coordinator capacity for a full queue, and lease cancellation or duplication for lease failures. A blob refusal cancels the lease; inspect its integrity, size and sink verification.

| Reason |
| --- |
| `transport.agent_blob` |
| `transport.agent_cancel_timeout` |
| `transport.agent_duplicate_lease` |
| `transport.agent_lease_lost` |
| `transport.agent_not_configured` |
| `transport.agent_not_connected` |
| `transport.agent_queue_full` |
