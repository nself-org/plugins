package main

// Purpose: the composition root of the burst engine in `ci serve` (D-1329): when the operator policy
//          enables a burst provider this build can make, build the engine (store, providers, resolved spend
//          policy, TEAM flag, deploy-host exclusion set, node revoker, alert sink), hand it to the cloud
//          provider as the joiner and bind it, so the dispatcher's waiting jobs provision machines, the
//          enrollment calls NodeJoined and the sweeper and teardown loop run.
// Inputs:  the coordinator store and data dir serveWire already has, and the pairing service it registered.
// Outputs: a bound burst.Engine and the func that unbinds it (and stops its loop).
// Constraints: nothing is built unless an enabled provider exists (the policy is read at start; enabling a
//          provider takes a `serve` restart, like the enroll listener's gate); an unreadable policy builds
//          nothing (fail closed); the coordinator id is stored in the data dir so a restart sweeps its own
//          machines, never another coordinator's; the local provider (ephemeral-local) is not built here: a
//          local resource has no runner binding yet (follow-up of D-1329), so enabling it does nothing.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/mode"
	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/providers/ephemeral/cloudvm"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// burstCoordinatorFile holds the stable coordinator id under the data dir.
const burstCoordinatorFile = "burst-coordinator-id"

// defaultPairing is the pairing service serveWire registered with pairing.Use, seen through cloudvm.Pairing.
type defaultPairing struct{}

func (defaultPairing) Offer(ctx context.Context, o pairing.OfferOptions) (pairing.OfferResult, error) {
	return pairing.Offer(ctx, o)
}
func (defaultPairing) Withdraw(ctx context.Context, code string) error {
	return pairing.Withdraw(ctx, code)
}

// serveBurst builds and binds the engine; see the file comment. The returned func is always safe to call.
func serveBurst(ctx context.Context, st *store.Store, home string) (func(), error) {
	noop := func() {}
	sp, err := resolveSpend(ctx)
	if err != nil {
		enrollLogf("burst is off: the operator policy could not be read: %v", err)
		return noop, nil
	}
	if !sp.Burst.Enabled[cloudvm.DefaultProviderID] {
		if sp.Burst.Enabled["ephemeral-local"] {
			enrollLogf("burst: the local ephemeral provider is enabled but has no runner binding yet; it does nothing")
		}
		return noop, nil
	}
	coordinator, err := burstCoordinatorID(home)
	if err != nil {
		return noop, fmt.Errorf("burst coordinator id: %w", err)
	}
	cloud, err := cloudvm.New(cloudvm.Config{CoordinatorID: coordinator, Spend: resolveSpend, Pairing: defaultPairing{},
		Intents: cloudvm.StoreIntents{Store: st, Coordinator: coordinator}, Registry: registry.New(st), Home: home})
	if err != nil {
		return noop, fmt.Errorf("burst cloud provider: %w", err)
	}
	guard := deployguard.New(st, home, "burst")
	eng, err := burst.New(burst.Options{Store: st, Providers: []burst.Provider{cloud}, Coordinator: coordinator,
		Spend: func(c context.Context) (economics.Policy, error) { return resolveSpend(c) },
		Team: func() bool {
			f, ferr := mode.Facts(context.Background())
			return ferr == nil && f.Mode == string(mode.Team) // unreadable is not TEAM: a cloud machine is refused
		},
		Exclusion: guard.Set,
		Revoker:   burst.LifecycleRevoker{Deps: lifecycle.NewDeps(st, home, "", "burst:engine")},
		Alert: func(a burst.Alert) {
			enrollLogf("burst alert %s: provider %s intent %s resource %s: %s", a.Kind, a.Provider, a.Intent, a.Resource, a.Detail)
		}})
	if err != nil {
		return noop, fmt.Errorf("burst engine: %w", err)
	}
	cloud.SetJoiner(eng)
	return burst.Bind(eng), nil
}

// burstCoordinatorID reads the coordinator id stored in home, or makes and stores one: "ci-" and 16
// random hex digits (a machine label value: letters, digits, '-').
func burstCoordinatorID(home string) (string, error) {
	path := filepath.Join(home, burstCoordinatorFile)
	b, err := os.ReadFile(path)
	if err == nil {
		if id := strings.TrimSpace(string(b)); id != "" && len(id) <= burst.MaxLabelValue {
			return id, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	raw := make([]byte, 8)
	if _, err = rand.Read(raw); err != nil {
		return "", err
	}
	id := "ci-" + hex.EncodeToString(raw)
	if err = os.MkdirAll(home, 0o700); err != nil {
		return "", err
	}
	return id, os.WriteFile(path, []byte(id+"\n"), 0o600)
}
