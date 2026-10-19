package deployguard

// Purpose: the coordinator-side boundary: refresh the cached exclusion set from every known project and
// the never-list, match identities against it, and set each registered node's deploy_host fact.
// Inputs:  the store, a Reader, a Resolver, the CI home (user config) and the actor of audit rows.
// Outputs: Set, Match, Refresh, EvaluateNode, Warnings; the cached rows in the store.
// Constraints: dispatch reads the cache only (Set never runs nself); a failed read of a root keeps its
//          entries and records the failure (doctor warning); never-list entries are cached the same way
//          under a pseudo root; Refresh re-evaluates every node afterwards; refresh runs at nodes
//          add|pair|test, coordinator start and every 10 minutes (Run).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// NeverListRoot is the pseudo root under which the never-list is cached.
const NeverListRoot = "user-config:never-list"

// RefreshEvery is the cadence of the background refresh.
const RefreshEvery = 10 * time.Minute

// Guard is the coordinator-side boundary.
type Guard struct {
	Store    *store.Store
	Reader   Reader
	Resolver Resolver
	// Home is the CI home holding config.yaml; empty means $NSELF_CI_HOME or ~/.nself/ci.
	Home  string
	Actor string
}

// New returns a Guard with the production reader and resolver.
func New(st *store.Store, home, actor string) *Guard {
	return &Guard{Store: st, Reader: Reader{Run: FunnelRun}, Resolver: DefaultResolver(), Home: home, Actor: actor}
}

// Seen records a project root the fabric has met (the current project at nodes add|test).
func (g *Guard) Seen(ctx context.Context, root string) error {
	if root == "" {
		return nil
	}
	return g.Store.ExclusionProjectSeen(ctx, root)
}

// Set builds the exclusion set from the cache. It runs no process and reads no inventory.
func (g *Guard) Set(ctx context.Context) (Set, error) {
	rows, err := g.Store.ExclusionEntries(ctx)
	if err != nil {
		return Set{}, err
	}
	var s Set
	for _, r := range rows {
		s.Add(Entry{r.Kind, r.Value, r.Source})
	}
	return s, nil
}

// ExclusionSet is the read-only export for P7-TRUST-08 and P7-SCHED-08.
func (g *Guard) ExclusionSet(ctx context.Context) (View, error) {
	s, err := g.Set(ctx)
	return s.View(), err
}

// RefreshProject reads one root; only a success replaces its cached entries.
func (g *Guard) RefreshProject(ctx context.Context, root string) error {
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return g.fail(ctx, root, newErr(ReasonInventory, "project root %s is not a directory", root))
	}
	es, err := g.Reader.Read(ctx, root)
	if err != nil {
		return g.fail(ctx, root, err)
	}
	rows := make([]store.ExclusionEntry, 0, len(es))
	for _, e := range es {
		rows = append(rows, store.ExclusionEntry{Root: root, Kind: e.Kind, Value: e.Value, Source: e.Source})
	}
	return g.Store.ExclusionReplace(ctx, root, rows)
}

func (g *Guard) fail(ctx context.Context, root string, cause error) error {
	if err := g.Store.ExclusionFail(ctx, root, cause.Error()); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (g *Guard) refreshNeverList(ctx context.Context) error {
	es, err := LoadNeverList(ConfigPath(g.Home))
	if err != nil {
		return g.fail(ctx, NeverListRoot, err)
	}
	rows := make([]store.ExclusionEntry, 0, len(es))
	for _, e := range es {
		rows = append(rows, store.ExclusionEntry{Root: NeverListRoot, Kind: e.Kind, Value: e.Value, Source: e.Source})
	}
	return g.Store.ExclusionReplace(ctx, NeverListRoot, rows)
}

// Refresh reads every known project and the never-list, then re-evaluates every node. All failures are
// returned joined; the cache keeps whatever it had for a failed root.
func (g *Guard) Refresh(ctx context.Context) error {
	var errs []error
	roots, err := g.Store.ExclusionProjects(ctx)
	if err != nil {
		return err
	}
	if extra, cerr := LoadProjectRoots(ConfigPath(g.Home)); cerr == nil {
		for _, r := range extra {
			if serr := g.Seen(ctx, r); serr == nil {
				roots = append(roots, r)
			}
		}
	}
	done := map[string]bool{}
	for _, r := range roots {
		if done[r] {
			continue
		}
		done[r] = true
		if rerr := g.RefreshProject(ctx, r); rerr != nil {
			errs = append(errs, rerr)
		}
	}
	if nerr := g.refreshNeverList(ctx); nerr != nil {
		errs = append(errs, nerr)
	}
	if eerr := g.Reevaluate(ctx); eerr != nil {
		errs = append(errs, eerr)
	}
	return errors.Join(errs...)
}

// Run refreshes now and then every RefreshEvery until ctx ends. Errors go to onErr.
func (g *Guard) Run(ctx context.Context, onErr func(error)) {
	tick := time.NewTicker(RefreshEvery)
	defer tick.Stop()
	for {
		if err := g.Refresh(ctx); err != nil && onErr != nil {
			onErr(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Match resolves an identity and matches it against the cached set.
func (g *Guard) Match(ctx context.Context, id Identity) (Result, []string, error) {
	s, err := g.Set(ctx)
	if err != nil {
		return Result{}, nil, err
	}
	f, warn := g.Resolver.Facts(ctx, id)
	return s.Match(f), warn, nil
}

// IdentityOf is the identity a registered node's capability document carries.
func IdentityOf(c model.Capability) Identity {
	id := Identity{Names: []string{c.Identity.ID, c.Identity.Name}, Fingerprints: c.Identity.HostKeyFingerprints}
	if c.Identity.MachineID.Value != nil {
		id.MachineID = *c.Identity.MachineID.Value
	}
	if s := c.Identity.SSH; s != nil {
		id.Dest = s.Alias
		id.Names = append(id.Names, s.Hostname)
		if id.Dest == "" {
			id.Dest = s.Hostname
		}
	}
	return id
}

// DeployHostFor is the deploy_host fact a match result implies, keeping a standing override only while
// the node still matches.
func DeployHostFor(r Result, prev model.CapabilityDeployHost) model.CapabilityDeployHost {
	if !r.Matched {
		return model.CapabilityDeployHost{}
	}
	src := r.Source
	return model.CapabilityDeployHost{Matched: true, Source: &src, Override: prev.Override}
}

// EvaluateNode matches one registered node and writes its deploy_host fact.
func (g *Guard) EvaluateNode(ctx context.Context, nodeID string) (Result, error) {
	n, err := g.Store.NodeGet(ctx, nodeID)
	if err != nil {
		return Result{}, err
	}
	var c model.Capability
	if err = json.Unmarshal(n.Capability, &c); err != nil {
		return Result{}, err
	}
	r, _, err := g.Match(ctx, IdentityOf(c))
	if err != nil {
		return Result{}, err
	}
	want := DeployHostFor(r, c.DeployHost)
	detail := "evaluated"
	if r.Matched {
		detail = fmt.Sprintf("matched %s", r.Kind)
	}
	return r, g.Store.NodeSetDeployHost(ctx, nodeID, want, g.actor(), detail)
}

// Reevaluate evaluates every registered node.
func (g *Guard) Reevaluate(ctx context.Context) error {
	nodes, err := g.Store.NodeList(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, n := range nodes {
		if _, eerr := g.EvaluateNode(ctx, n.ID); eerr != nil {
			errs = append(errs, fmt.Errorf("node %s: %w", n.ID, eerr))
		}
	}
	return errors.Join(errs...)
}

func (g *Guard) actor() string {
	if g.Actor == "" {
		return "deployguard"
	}
	return g.Actor
}

// Warnings lists the doctor warnings: every root whose last refresh failed, with the age of the entries
// still in use.
func (g *Guard) Warnings(ctx context.Context) []string {
	states, err := g.Store.ExclusionStates(ctx)
	if err != nil {
		return []string{"deploy-host exclusion state unreadable"}
	}
	var out []string
	for _, st := range states {
		if st.OK {
			continue
		}
		use := "no cached entries yet"
		if st.LastOKAt > 0 {
			use = "using entries cached " + time.Unix(0, st.LastOKAt).UTC().Format(time.RFC3339)
		}
		out = append(out, fmt.Sprintf("deploy-host refresh failed for %s (%s); %s", st.Root, st.Error, use))
	}
	return out
}
