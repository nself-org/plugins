# CI-46 measurement prerequisites

Status: incomplete; no performance acceptance or owner exception is claimed.

The declared `bench/sched` package contains actual detector, planner and store
microbenchmarks, a sampling primitive, and a relative-budget check. Its internal
observation JSON is not asserted to implement `contract:cli.perfbench`.

The requested `bench/cmd/cibench` package has no row in
`free/ci/architecture/layers.yaml`. The ticket requires a row before introducing
a package and excludes that file from write scope. Register the command and its
measurement package in the architecture ticket before adding that entry point.

The CLI perfbench result and budget schemas are not available in this working
tree. Confirm them from the pinned CLI checkout before implementing an adapter.
Do not create a guessed contract or an empty budget that would pass without
checking measurements.

Remaining acceptance work:

- Wire real plugin subprocesses for startup and both end-to-end fixture sizes,
  preserving the frozen legacy gate and alternating the two engine versions.
- Measure per-job executor overhead with a no-op job and account for the job's
  own measured runtime.
- Wire warm and cold service probes to a dedicated disposable image and daemon;
  cold timing must include the first pull and service readiness. Never remove
  shared images to obtain a cold result.
- Collect complete runs on linux-amd64 and macos-arm64 with revision, machine
  specifications and raw samples. Retain failures instead of substituting
  estimates or skipping unavailable metrics.
- Generate the absolute budget from recorded baseline p95 values with the
  required margin. Evaluate the tiny relative budget on each platform.
- Add the pinned CLI regression workflow and demonstrate both acceptance of
  measured branch results and rejection of an injected slowdown.
- Record any owner-accepted miss, with cause, gap and expiry, in the specified
  HQ evidence file. That file is outside this ticket's write scope.

Run the available microbenchmarks from the repository root:

```sh
CGO_ENABLED=0 go -C free/ci test -mod=vendor -run='^$' -bench=. -count=10 ./bench/...
```

These microbenchmarks do not establish the process-level fast-path budget.
