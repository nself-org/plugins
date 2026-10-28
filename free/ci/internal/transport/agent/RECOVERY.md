# Pull delivery after a coordinator restart

The store owns lease fences, attempt state and deadlines. The pull driver's
queues, routes and pending control-stream cancels are process-local delivery
bookkeeping, not a durable work queue. Recreating a driver does not replay them.

A dispatcher that resumes an attempt must register its Run with the existing
lease id, attempt id and epoch before accepting its agent's reconnect. This
rebuilds the route and offers the lease again; it must not grant a second fence.
`TestPullCoordinatorKillResumesWithoutDuplicates` exercises this explicit
re-adoption with a running lease and durable logs. Recovery does not promise
that every abandoned dispatch is automatically re-adopted.

An undelivered lease is absent from the new long-poll queue. It can be explicitly
re-enqueued with its existing fence, otherwise the durable lease TTL bounds how
long it occupies capacity. A running agent reports its held lease on reconnect.
If no run adopted it, Admit refuses its heartbeat as `agent_lease_lost`; agentapi
sends a cancel for that lease while keeping the stream and other leases alive.
`TestPullALeaseNoRunWaitsForIsCancelledAndTheStreamGoesOn` covers the wire path.

A pending cancel is also lost. The same unknown-route refusal stops a reported
lease even when the old process never delivered its cancel. The durable attempt
epoch/cancel intent remains authoritative; delivery bookkeeping cannot undo it.
A disconnected agent that never reports again cannot renew and expires by TTL.
`TestRestartWithQueuedRunningAndPendingCancel` closes and reopens the real store
with all three cases and checks explicit re-adoption, lost refusals and expiry.
