# D-0649 / D-0653 Go advisory sweep

Baseline: origin/main `93cad14e1384f0270f61ec9d28d81043dc86008a`.
All 175 tracked Go modules, including SDK copies, scripts and fixtures, were scanned with
`GOWORK=off go run golang.org/x/vuln/cmd/govulncheck@latest -json ./...`.
Only findings with a function in the first trace frame are counted as reachable.

The baseline contained 202 reachable findings in 101 modules, covering 22 advisory IDs.
The final scan has zero reachable findings and zero scan errors.
[Before](d0649-d0653-govulncheck-before.json) and
[after](d0649-d0653-govulncheck-after.json) record every scanned module and its findings.
The after snapshot combines the complete rescan with a warehouse rescan after its v2 migration.

## Root cause and fixes

Old requirements were present in standalone modules, copied SDK modules and vendored dependencies.
Updating the root SDK alone does not update those copies. Requirements were updated with `go get`,
then tidied in dependency order, including consumers with local replacements. Existing vendors were regenerated.
118 modules were processed. The boot-migration codemod now adds the fixed `x/text` floor when
creating its copied SDK, so regeneration preserves the fix; the alpha fixture and golden requirements match.

The fixed versions below come from govulncheck. The individual module paths and found versions
are in the before snapshot.

| Dependency | Reachable advisory IDs | Fixed version |
|---|---|---|
| `github.com/go-chi/chi/v5` | GO-2026-5774, GO-2026-5775, GO-2026-5777 | v5.3.0 |
| `github.com/jackc/pgx/v5` | GO-2026-5004 | v5.9.2 |
| `github.com/rabbitmq/amqp091-go` | GO-2026-6372, GO-2026-6491, GO-2026-6492, GO-2026-6494, GO-2026-6496, GO-2026-6498, GO-2026-6500, GO-2026-6503 | v1.13.0 |
| `github.com/snowflakedb/gosnowflake` | GO-2026-6665 | No v1 fix; migrate to `/v2` v2.2.0 |
| `github.com/yuin/goldmark` | GO-2026-5320 | v1.7.17 |
| `golang.org/x/net` | GO-2026-6603, GO-2026-6610, GO-2026-6611, GO-2026-6612, GO-2026-6617 | v0.60.0 |
| `golang.org/x/text` | GO-2026-5970 | v0.39.0 |
| `golang.org/x/text` | GO-2026-6629 | v0.41.0 |
| `google.golang.org/grpc` | GO-2026-6348 | v1.83.1 |

`x/text` was raised to at least v0.41.0; modules requiring newer `x/net` or `x/crypto`
select v0.42.0 through their dependency graph. The held D-0649 pins were also updated:
`x/net` v0.38.0/v0.55.0 to v0.60.0, and `x/crypto` v0.53.0 to v0.57.0.

The Snowflake v1 advisory lists no fixed v1 release. Its
[v2.2.0 release](https://github.com/snowflakedb/gosnowflake/pull/1844) contains the logging fix.
The warehouse now imports `github.com/snowflakedb/gosnowflake/v2` v2.2.0 and uses its
`WithFilePutStream` upload API. Warehouse build, vet, tests and its rescan passed.
Live Snowflake integration was not exercised.

## Builder alignment

Official builder images use `GOTOOLCHAIN=local`, so a newer module requirement cannot be
satisfied by downloading another compiler. Existing image variants were preserved.

| Builder | Before | After |
|---|---|---|
| byok, claw-cli, event-bus, functions-v8 | 1.25-alpine | 1.26-alpine |
| storage-transform | 1.25-bookworm | 1.26-bookworm |
| e2ee | 1.25-alpine | 1.26-alpine |
| email, plugin-pty | 1.22-alpine | 1.25-alpine |
| new-plugin scaffold | 1.25-alpine | 1.26-alpine |

The scaffold's generated Go directive now matches the SDK's 1.26.4 floor.
GitHub and Stripe builders were already on 1.26 in the baseline.

## Verification

- Every tracked module was scanned before and after; reachable findings: 202 to 0.
- SDK build and tests passed.
- The codemod's generated SDK go.mod/go.sum match the alpha golden files using the pinned CLI source.
- Shell syntax and whitespace checks passed.
- The initial required plugin gate hit D-0655: `TestInstallRunnerInstance_InheritedXtraceDoesNotLeak`.
- Final required plugin gate: blocked by the same D-0655 xtrace test failure; no tests were excluded.
- All eight changed builder tags satisfy their modules' Go directives.
- Full fixture execution on ps1 was blocked by its inability to create the required `/Volumes/D5/tmp` scratch path.
- Builder compilation on ps1 was blocked by Docker socket permissions.
- The broad secret scan timed out. The added-line scan (6.93 MB, with the repository config and a positive control) reported two generic-api-key false positives: the public module checksums for cloud.google.com/go/auth/oauth2adapt v0.2.8 and github.com/aws/aws-sdk-go-v2/credentials v1.19.30. Both match go mod download metadata; no allowlist was changed.

No release scripts, allowlists, test exclusions or project versions were changed.
