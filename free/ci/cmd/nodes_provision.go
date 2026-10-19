package main

// Purpose: the "nodes provision" and "nodes verify" subcommands: build and
// audit self-hosted GitHub Actions runner hosts, ported from core
// `nself runner provision|verify` (P7-CANON-10). The implementation lives in
// internal/nodes/provision; this file registers the two keys in the
// subcommand table (P7-CANON-09) and, in V15 mode, wires the pinned runner
// and the registry record of a provisioned runner (P7-NODE-06).
// Inputs:  the flags of core `nself runner provision` and `nself runner verify`;
// V15 mode (NSELF_V15) adds `provision --json` and changes `verify --json`.
// Outputs: provisioning steps or the verify parity matrix on stdout; exit 0,
// or 1 for any failure and for a verify run that finds a failing check, an
// unreachable host or drift, as core. V15 --json: one v1 envelope.
// Constraints: every ssh call goes through the vendored cli sdk/go/remote;
// this package has no ssh or scp exec site (TestSingleSSHExecSite). With
// NSELF_V15 unset nothing here differs from P7-CANON-10 (parity-nodes.sh).

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/nodes/provision"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

// runnerPinName is the toolchain pin of the GitHub Actions runner.
const runnerPinName = "github-actions-runner"

func init() {
	register("nodes provision", func(args []string) int {
		return provision.RunProvision(args, provisionDeps())
	})
	register("nodes verify", func(args []string) int {
		return provision.RunVerify(args, provisionDeps())
	})
}

// provisionDeps is the real wiring; V15 adds the pin and the registry record.
func provisionDeps() provision.Deps {
	d := provision.DefaultDeps()
	// compat.V15(P7-NODE-06): no pin, no record -> pinned runner and a registry record (trust.accepts null)
	if compat.V15() {
		d.RunnerPin = runnerPin
		d.Record = recordRunnerNode
	}
	return d
}

// runnerPin maps the generated toolchain table into the provision package's pin.
func runnerPin() (provision.RunnerPin, error) {
	table, err := toolchain.Embedded()
	if err != nil {
		return provision.RunnerPin{}, err
	}
	row, ok := table.Row(runnerPinName)
	if !ok {
		return provision.RunnerPin{}, fmt.Errorf("the toolchain table has no %s row", runnerPinName)
	}
	pin := provision.RunnerPin{Version: row.Version, Sources: map[string]provision.RunnerSource{}}
	for _, s := range row.Sources {
		if s.OS == "linux" {
			pin.Sources[s.Arch] = provision.RunnerSource{URL: s.URL, SHA256: s.SHA256}
		}
	}
	return pin, pin.Validate()
}

// recordRunnerNode writes the provisioned host to the node registry; a host already recorded is left as is.
func recordRunnerNode(ctx context.Context, req provision.RecordRequest) (string, error) {
	d, closeStore, err := lifecycle.ProductionEnv(currentProject).Open(ctx, "")
	if err != nil {
		return "", err
	}
	defer closeStore()
	cap := provision.RunnerCapability(req, time.Now().UTC())
	id := cap.Identity.ID
	if _, gerr := d.Registry.Get(ctx, id); gerr == nil {
		return id, nil
	}
	doc, err := json.Marshal(cap)
	if err != nil {
		return "", err
	}
	admin, err := store.NewAdminAuthority(d.Actor)
	if err != nil {
		return "", fmt.Errorf("no admin identity for the audit row")
	}
	rec := store.NodeRecord{ID: id, Name: id, State: "online", Reason: "provisioned", Capability: doc}
	if err = d.Store.NodeAdd(ctx, rec, admin); err != nil {
		return "", err
	}
	return id, nil
}
