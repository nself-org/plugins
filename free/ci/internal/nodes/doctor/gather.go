package doctor

// Purpose: read the facts the node checks need from the machine's own state: the node registry, certificates,
// leases and coordinator mode in the run store, the deploy-host exclusion state, the discovery switch and the
// binary's limits. The network and ssh readings are in gather_net.go.
// Inputs:  Deps (every outside reader is a field, so tests run with fakes); the CI home.
// Outputs: Facts.
// Constraints: read only. The store is opened only when its file already exists (a doctor run never creates a
//          state database); a store that cannot be read becomes StoreErr, not a panic; nothing is written to the registry.

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/discovery"
	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Limits are what the binary knows about itself.
type Limits struct {
	MinAgentVersion string
	MinProtocol     int
	PinnedRelease   func() string // nil or "" when no agent release is pinned
}

// Settings are the binary's limits; cmd/doctor_nodes.go fills them from the build.
var Settings = Limits{MinProtocol: 1}

// Deps are the outside readers of Gather.
type Deps struct {
	Now        func() time.Time
	OpenStore  func(path string) (*store.Store, error)
	SSHVersion func(ctx context.Context) (remote.Version, error)
	Dial       func(ctx context.Context, addr string) error
	RunRemote  func(ctx context.Context, t remote.Target, argv ...string) (string, error)
}

// DefaultDeps reads the real machine.
func DefaultDeps() Deps {
	return Deps{
		Now: time.Now, OpenStore: openExisting, SSHVersion: remote.SSHVersion,
		Dial: dialTCP, RunRemote: remote.RunArgv,
	}
}

// openExisting opens the state database with a short busy retry. The caller has checked the file exists.
func openExisting(path string) (*store.Store, error) {
	var st *store.Store
	var err error
	for end := time.Now().Add(3 * time.Second); ; time.Sleep(25 * time.Millisecond) {
		st, err = store.Open(path, store.Options{})
		busy := err != nil && (strings.Contains(err.Error(), "SQLITE_BUSY") || strings.Contains(err.Error(), "database is locked"))
		if err == nil || !busy || time.Now().After(end) {
			return st, err
		}
	}
}

// Home is the CI home: NSELF_CI_HOME, else ~/.nself/ci ("" when neither is known).
func Home(getenv func(string) string) string {
	if h := getenv("NSELF_CI_HOME"); h != "" {
		return h
	}
	u, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(u, ".nself", "ci")
}

// Gather reads the facts for one run.
func Gather(ctx context.Context, env doctor.Env, d Deps) Facts {
	now := d.Now()
	f := Facts{
		Now: now, MinAgentVersion: Settings.MinAgentVersion, MinProtocol: Settings.MinProtocol,
		Reach: map[string]error{}, Pins: map[string][]string{}, CertExpiry: map[string]time.Time{},
		Skew: map[string]time.Duration{}, SkewErr: map[string]error{}, ExpiredLeases: map[string]int{},
	}
	if Settings.PinnedRelease != nil {
		f.PinnedRelease = Settings.PinnedRelease()
	}
	home := Home(env.Getenv)
	f.SSHVersion, f.SSHVersionErr = d.SSHVersion(ctx)
	f.readDiscovery(home, env.Getenv)
	if home == "" {
		f.StoreNote = "the CI home is unknown"
	} else {
		f.readStore(ctx, filepath.Join(home, "state.db"), home, d)
	}
	f.readNet(ctx, env, home, d)
	return f
}

// readDiscovery asks the discovery guard the same question `nodes discover` asks.
func (f *Facts) readDiscovery(home string, getenv func(string) string) {
	err := (&discovery.Guard{Home: home, Getenv: getenv}).Allowed()
	if err == nil {
		return
	}
	f.DiscoveryOff = true
	var de *discovery.Error
	if errors.As(err, &de) && de.Reason == discovery.ReasonConfigInvalid {
		f.DiscoveryErr = err
	}
}

// readStore fills the registry-backed facts.
func (f *Facts) readStore(ctx context.Context, path, home string, d Deps) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		f.StoreNote = "no state database yet"
		return
	} else if err != nil {
		f.StoreErr = err
		return
	}
	st, err := d.OpenStore(path)
	if err != nil {
		f.StoreErr = err
		return
	}
	defer func() { _ = st.Close() }()
	nodes, err := registry.New(st).List(ctx)
	if err != nil {
		f.StoreErr = err
		return
	}
	for _, n := range nodes {
		if n.Record.State == "revoked" || n.Capability.Lifecycle.RevokedAt != nil {
			continue
		}
		f.Nodes = append(f.Nodes, Node{Cap: n.Capability, State: n.Record.State, Reason: n.Record.Reason})
	}
	f.readCerts(ctx, st)
	f.readLeases(ctx, st)
	f.ExclusionWarnings = deployguard.New(st, home, "doctor").Warnings(ctx)
	if mode, found, mErr := st.CoordinatorModeGet(ctx); mErr == nil && found && mode == store.CoordinatorTeam {
		f.TeamMode = true
	}
}

// readCerts records, per agent node, the latest expiry among certificates that are not on the deny list.
func (f *Facts) readCerts(ctx context.Context, st *store.Store) {
	for _, n := range f.agentNodes() {
		certs, err := st.PKICertsOfNode(ctx, n.ID())
		if err != nil {
			f.CertsErr = err
			return
		}
		var latest int64
		for _, c := range certs {
			if denied, dErr := st.PKIIsDenied(ctx, c.Serial); dErr != nil || denied {
				continue
			}
			latest = max(latest, c.NotAfter)
		}
		if latest > 0 {
			f.CertExpiry[n.ID()] = time.Unix(0, latest)
		}
	}
}

// readLeases counts, per node, leases past their deadline whose attempt is still leased or running: the job
// root of such a lease is an orphan until the coordinator fences it. A lease belongs to a node when its runner
// id is the node id or starts with it followed by ":" or "/".
func (f *Facts) readLeases(ctx context.Context, st *store.Store) {
	leases, err := st.LeaseExpired(ctx, f.Now)
	if err != nil {
		return
	}
	for _, l := range leases {
		for _, n := range f.Nodes {
			id := n.ID()
			if l.RunnerID == id || strings.HasPrefix(l.RunnerID, id+":") || strings.HasPrefix(l.RunnerID, id+"/") {
				f.ExpiredLeases[id]++
			}
		}
	}
}
