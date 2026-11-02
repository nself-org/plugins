# CI resource evidence

Planned container jobs collect host cgroup counters while running and once
more at exit, before explicit container removal. Final discovery also accepts
an exited container's runtime ID when its init PID is zero.

`pids.events` and `memory.events` are read independently. Each retains its
successful sample count and read errors; failed reads do not supply zero
samples or erase earlier evidence. Read errors are included in host diagnostics.

The executor requires host runtime or cgroup evidence for `resource_limit`.
Diagnosis uses host executor errors, signals, or the resource reason for
retryable infrastructure diagnoses. Job output, including dependency prepare
markers and allocation errors, cannot authorize an infrastructure retry.
Log-only install hints are nonretryable.
