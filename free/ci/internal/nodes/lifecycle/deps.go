package lifecycle

// Purpose: the seams of the node lifecycle: everything that touches ssh, the clock of a terminal or
// the registry goes through Deps, so unit tests run with fakes and the integration test runs with
// the real ssh against the simharness.
// Inputs:  the CI home, the current project id, the admin actor, the store and registry.
// Outputs: Deps with production defaults from NewDeps.
// Constraints: ssh and scp start only through cli sdk/go/remote (no os/exec in this package); the pin
//          file is $NSELF_CI_HOME/known_hosts (0600), never ~/.ssh/known_hosts; the actor is the admin
//          running the command (never "agent" or "probe").

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/nodes/probe"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// agentDir is where the agent lives under the SSH user's home (Epic D4).
const agentDir = ".nself/ci-agent"

// agentPath is the installed agent binary, relative to the SSH user's home.
const agentPath = agentDir + "/bin/nself-ci-agent"

// Deps carries the collaborators of every lifecycle command.
type Deps struct {
	Registry *registry.Registry
	Store    *store.Store
	// Home is $NSELF_CI_HOME: the pin file is Home/known_hosts.
	Home string
	// Project is the id of the current project; `nodes add` authorizes that project only.
	Project string
	// Actor is the admin running the command.
	Actor          string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	// TTY is true when Stdin is a terminal: only then may `nodes add` ask for the `yes`.
	TTY bool

	SSHVersion func(ctx context.Context) (remote.Version, error)
	Scan       func(ctx context.Context, t remote.Target, nodeID string) ([]remote.HostKey, error)
	Run        func(ctx context.Context, t remote.Target, argv ...string) (string, error)
	Copy       func(ctx context.Context, t remote.Target, local, remotePath string) error
	// NewProber builds the prober of a registry; the default is probe.SSHProber.
	NewProber  func(r *registry.Registry, pinnedFile string, v remote.Version) probe.Prober
	Separation SeparationProber
}

// NewDeps wires the production collaborators around an open store.
func NewDeps(st *store.Store, home, project, actor string) *Deps {
	return &Deps{
		Registry: registry.New(st), Store: st, Home: home, Project: project, Actor: actor,
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, TTY: isTerminal(os.Stdin),
		SSHVersion: remote.SSHVersion, Scan: remote.ScanHostKeysSSH, Run: remote.RunArgv, Copy: remote.CopyTo,
		NewProber: func(r *registry.Registry, pinned string, v remote.Version) probe.Prober {
			return &probe.SSHProber{Registry: r, PinnedFile: pinned, Version: v}
		},
		Separation: AgentSeparation{},
	}
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// pinFile is the pinned host key file.
func (d *Deps) pinFile() string { return filepath.Join(d.Home, "known_hosts") }

// pins is the pinned host key store.
func (d *Deps) pins() remote.PinnedHostKeys { return remote.PinnedHostKeys{Path: d.pinFile()} }

// pinAlias is the HostKeyAlias of a node (the D4 option set uses the same string).
func pinAlias(nodeID string) string { return "nself-ci-" + nodeID }
