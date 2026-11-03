# CI resource evidence

Planned container jobs collect host cgroup counters while running and once
more at exit, before explicit container removal. Final discovery also accepts
an exited container's runtime ID when its init PID is zero.

`pids.events` and `memory.events` are read independently. Each retains its
successful sample count and read errors; failed reads do not supply zero
samples or erase earlier evidence. Read errors are included in host diagnostics.

The executor requires host runtime or cgroup evidence for `resource_limit`.
Memory/OOM and process/fork-limit diagnoses require host executor errors,
signals, or the resource reason. Exit 137, allocation errors, OOM text, and
fork-limit messages in job output alone cannot authorize a resource retry,
including during dependency preparation. Host resource evidence takes priority
over job-controlled prepare and pin markers.

Other infrastructure signatures retain their text-based classification and
retry behavior: disk, DNS, network, Docker, missing tools, and install/prepare
failures, including registry and node infrastructure opt-in. Existing output
windows and test-failure guards still apply. Timeout and lost-runner handling
also retain their existing behavior.
