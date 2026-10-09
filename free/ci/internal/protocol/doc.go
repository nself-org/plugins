// Package protocol defines contract:ci.runner-protocol v1. A session carries
// newline-delimited JSON frames no larger than 1 MiB including the newline.
// Each direction starts at seq 1 and increments without gaps. Epoch and lease
// ID fence lease-scoped work; an agent must reject a stale epoch.
//
// Agent to coordinator: hello, log, artifact-ref, result, cache-ref. Coordinator
// to agent: welcome, reject, lease, secrets, cancel, drain, upgrade. Both:
// heartbeat, chunk, ack, digest, offset, resume. Digest, offset and resume are
// reserved transfer/reconnect messages. A value-bearing secrets frame is valid
// only on SSH carrier A with a PERSONAL coordinator and operator-owned project
// or environment secrets. Carrier B transfers secret references only.
//
// State: disconnected -> hello -> welcome -> active -> drain -> disconnected;
// hello -> reject -> disconnected. Carrier A writes NSELF-CI-AGENT/1 before
// hello. Carrier B may resume an active session after reconnect. SSH session
// loss makes the attempt lost; it cannot resume. Reject E652 means incompatible
// version and names min_agent_version. E653 means malformed frame or policy.
// Both sides accept N and N-1. The minimum agent version rises only in a minor
// release after a one-minor warning. Only additive fields are allowed in v1.
package protocol
