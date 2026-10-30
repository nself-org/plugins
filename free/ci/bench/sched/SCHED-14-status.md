# SCHED-14 measurement prerequisites

Status: incomplete. No performance budget or acceptance result is claimed.

Available measurements:

- `internal/sched/bench_test.go`: complete Place decisions over 1/10/100
  runners crossed with 10/100/1000 jobs. Each runner has one slot, so the
  workload includes successful placement and capacity refusal. Setup is untimed;
  explanation construction and allocations are included.
- `internal/dispatch/bench_test.go`: world.Build (including snapshot and digest),
  Place and Admit against a fresh private on-disk store per iteration, so world
  cardinality does not grow with b.N. Store setup, queue setup and successful
  settlement are untimed. No transport, service or job execution is simulated
  as end-to-end pipeline time.
- `ci-bench.yml`: sequential Linux measurements on public GitHub-hosted
  ubuntu-latest, two discarded warmup runs and ten recorded runs, with raw
  benchmark output, revision, machine and command retained as artifacts.

The local one-iteration smoke run passed on darwin/arm64, Apple M5 Max.
This establishes executable fixtures, not a statistical baseline. Use:

```sh
CGO_ENABLED=0 go -C free/ci test -mod=vendor -run '^$' -bench 'Benchmark(Place|Admit)$' -benchmem -count=2 ./internal/sched/ ./internal/dispatch/
CGO_ENABLED=0 go -C free/ci test -mod=vendor -run '^$' -bench 'Benchmark(Place|Admit)$' -benchmem -count=10 ./internal/sched/ ./internal/dispatch/
```

## Required inputs for completion

The upstream `bench/CI-46-status.md` records incomplete end-to-end probes and
an unavailable perfbench contract. This tree contains neither
`bench/cmd/cibench` nor `bench/perf-budget.json`. Although schedbench has an
architecture row, cibench does not. The ticket prohibits editing the layer
registry and accessing the CLI or HQ checkout outside this working tree.

The lead must supply the pinned CLI result and budget schemas plus its
origin/main SHA, the completed CI-46 harness and measured budget, and a real
tiny-repository end-to-end probe with scheduling instrumentation. Pair the
scheduling numerator with the same run's end-to-end denominator. A detached
Place microbenchmark divided by unrelated pipeline samples is not the required
`sched.share_tiny_pct` measurement.

After those inputs arrive, implement the registered schedbench entry point,
retain ten samples after two warmups, calculate p50/p95 and variance, and derive
each platform budget from its measured p95 times 1.25. Extend the workflow with
the pinned CLI checks for both cibench and schedbench and prove rejection of an
injected slowdown. The current workflow collects microbenchmarks only and is
not a perfbench budget gate.

Record Linux results only from that workflow, and macos-arm64 results from the
operator's Mac. Supply the lead with all three scheduling metric rows for
performance-slos.md, including revision, machine, command, raw samples and
pinned CLI SHA. The requested HQ evidence and SLO document are outside this
ticket's write scope; neither has been updated. No owner exception exists here.
