package main

// Purpose: link the forge-runner launcher's agent half (P7-HOST-19) into nself-ci-agent: the job hook
// and the hosts source that give a runner job's restricted network scope its forge host allowlist.
// Both act only for a lease that carries a runner credential, so every other job is unchanged.

import "github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"

func init() { must(runnerjob.InstallAgent()) }
