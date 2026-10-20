package runnerjob

// Purpose: the agent-side half of the restricted network scope for a runner job (Epic D6, review F6).
// A restricted job may reach only the package registries and the hosts policy declares; a forge runner
// must also reach its forge. Hook is a job hook that notes, for the attempts whose lease carries a runner
// credential, the provider's host list, and AttemptHosts is the hosts source for the scope planner that
// answers only for those attempts (the agent's composition root wires both, cmd/agent/runnerjob_wire.go). Every other restricted job keeps the unchanged allowlist.
// Inputs:  the lease (the runner credential kind and the provider marker of the job); the attempt id.
// Outputs: AttemptHosts for run.RegisterNetscopeHosts; the "runnerjob" job hook.
// Constraints: the hosts are exact names (the scope allowlist has no wildcard), so the numbered
//          `*.actions.githubusercontent.com` service hosts are not all expressible: a runner whose
//          region answers from an unlisted name cannot connect and fails closed (debt row on the scope
//          allowlist). The list is per provider and fixed in this file; nothing a demand or a workflow
//          carries adds a host. The note is removed when the lease ends.

import (
	"context"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// ProviderHosts is the host allowlist of a forge's runner, by provider id. GitHub: the web and API
// hosts, the Actions service hosts that have a fixed name, release and package downloads, and ghcr.io.
var ProviderHosts = map[string][]string{
	"github": {
		"github.com", "api.github.com", "codeload.github.com",
		"objects.githubusercontent.com", "pkg-containers.githubusercontent.com", "ghcr.io",
		"pipelines.actions.githubusercontent.com", "results-receiver.actions.githubusercontent.com",
		"broker.actions.githubusercontent.com", "vstoken.actions.githubusercontent.com",
		"token.actions.githubusercontent.com",
	},
}

var notes = struct {
	sync.Mutex
	hosts map[string][]string
}{hosts: map[string][]string{}}

// AttemptHosts is the extra restricted-scope host list of one attempt: the provider's list for a runner
// attempt, nil for any other.
func AttemptHosts(a isolation.Attempt) []string {
	notes.Lock()
	defer notes.Unlock()
	return append([]string(nil), notes.hosts[a.ID]...)
}

func note(l protocol.Lease) {
	if l.RunnerCredential == nil || l.Job.Env[EnvProvider] == "" {
		return
	}
	list := ProviderHosts[l.Job.Env[EnvProvider]]
	if len(list) == 0 {
		return
	}
	notes.Lock()
	notes.hosts[l.AttemptID] = append([]string(nil), list...)
	notes.Unlock()
}

func unnote(attempt string) {
	notes.Lock()
	delete(notes.hosts, attempt)
	notes.Unlock()
}

// Hook is the agent job hook; InstallAgent registers it.
func Hook() session.JobHook {
	return session.JobHook{
		Name: "runnerjob",
		Before: func(_ context.Context, l protocol.Lease, _ session.JobRoot) (session.JobSetup, error) {
			note(l)
			return session.JobSetup{}, nil
		},
		After: func(_ context.Context, l protocol.Lease, _ session.JobRoot, _ session.Outcome, _ session.Uploader) error {
			unnote(l.AttemptID)
			return nil
		},
	}
}
