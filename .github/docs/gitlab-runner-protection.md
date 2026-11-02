# GitLab runner protection

GitLab shell jobs can read their runner authentication token. Pausing a runner
after its running Job Hook reduces token reuse time, but the hook arrives after
the job starts. Pausing alone cannot isolate protected jobs from unprotected code.

Every runner registration therefore uses `access_level: ref_protected`, including
the generic registrar interface and registrations with zero mint options. The
launcher requires explicit operator opt-in through
`providers.gitlab.runner.protected_refs_only`, which defaults to `false`. Minting
is refused until the operator sets it to `true`, accepting that unprotected
branches, merge request refs and fork refs cannot obtain these runners. Enabling
the setting requires an API assessment proving that the demand names a protected
branch or tag. Missing, unprotected, merge request and unreadable refs cannot
fall back to an unrestricted runner. API errors propagate; unproved protection
is refused before registration. Unprotected jobs require a separate runner setup.

The GitLab webhook source authenticates running Job Hooks and binds their project
identity before calling `Registrar.OnRunning`, regardless of job or runner tags
in the payload. The API ownership check decides whether the runner is ours.
A bound project ID takes precedence
over the project path. The registrar checks the API job status, assigned runner
and runner ownership before pausing. Handled running hooks enqueue no work;
replays are idempotent and real API errors propagate through ingress. Running
hooks without configured runners, or for foreign or
mismatched runners decline with HTTP 200 and enqueue nothing. HTTP 403 is
reserved for a hook for our runner that fails verification.

## Residual debt

| ID | Status | Type | Severity | Remaining work |
| --- | --- | --- | --- | --- |
| D-1390 | PARTLY | security | high | Running-hook handling and protected-ref runner enforcement are implemented; the protected-to-protected token reuse risk remains in D-1390-R1. |
| D-1390-R1 | open | security | high | A protected job can read its runner token and poll for another protected pending job in the same project, including a deploy job with environment-scoped variables. Pausing narrows the race after job start but does not eliminate it. Enforce single-use runner tokens before marking this debt and D-1390 complete. |

Focused coverage: `TestRunningHookIngress`, `TestMinterRequiresProtectedRef`,
`TestMintCannotIssueUnrestrictedToken` and `TestAccessLevel` in the GitLab webhook
and runners packages. This uses protected-ref isolation rather than claiming
that runner tokens are single-use.
