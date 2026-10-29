package main

// Purpose: the dirty-tree offload of a dev run (P7-AUTO-13, ADR 0032 D15).
// When `nself ci --where <node|pool|auto|all>` runs on a tree with uncommitted
// work, nodes check out a snapshot commit (internal/offload) held under a
// private ref, so the NODE-03 bundle transport carries the user's exact tree;
// the run still records the commit it started from, dirty. This file
// registers the run's OffloadHook slot.
// Inputs:  the run root (Options.Root), the --where target the invocation
//          resolved (whereChosen), git through the exec funnel.
// Outputs: an OffloadSnapshot whose Release deletes the private ref; the names
//          of untracked files left out go to stderr, never their content.
// Constraints: only a target that can reach a registered node takes a
//          snapshot: no --where with nothing remote registered, `local`,
//          `hosted`, and pool/auto/all with no node keep the HEAD path
//          unchanged (a hosted runner fetches the revision from the forge,
//          where a snapshot never exists). A clean tree keeps HEAD. A
//          tracked secret-shaped or oversized file refuses the run (E807). The
//          snapshot run stays dirty: unbound evidence, no status of record.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/offload"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/where"
)

// Seams for tests.
var (
	offloadCapture offload.Capture = report.Capture
	offloadErr     io.Writer       // nil: os.Stderr
)

func init() {
	cirun.RegisterOffload(cirun.SnapshotOffload{Build: buildOffload, Excluded: reportExcluded})
}

// offloadWanted reports whether a run with this --where target may execute on
// a registered node, the only kind of runner that fetches the revision by
// bundle. A pool, auto or all target with no node registered (only hosted
// providers) keeps HEAD.
func offloadWanted(ctx context.Context, kind string) bool {
	switch kind {
	case where.KindNode:
		return true
	case where.KindPool, where.KindAuto, where.KindAll:
		return len(whereKnown(ctx, true).Nodes) > 0
	}
	return false
}

// buildOffload snapshots the dirty tree at root, or returns nil to keep HEAD.
func buildOffload(ctx context.Context, root string) (*cirun.OffloadSnapshot, error) {
	if root == "" || !offloadWanted(ctx, whereChosen.Kind) {
		return nil, nil
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	scratch, err := os.MkdirTemp("", "nself-ci-offload-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratch) }() // the temporary index and path list
	capture := offloadCapture
	s, err := offload.Take(ctx, capture, root, scratch, hex.EncodeToString(nonce[:]))
	if err != nil {
		return nil, err
	}
	if s.Clean {
		return nil, nil
	}
	return &cirun.OffloadSnapshot{SHA: s.SHA, Excluded: s.Excluded,
		Release: func(ctx context.Context) error { return offload.Release(ctx, capture, root, s) }}, nil
}

// reportExcluded names the untracked files the snapshot left out.
func reportExcluded(names []string) {
	w := offloadErr
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, "nself ci: offload: not sent to the node (secret-shaped or over %d MiB, untracked): %s\n",
		offload.MaxFileBytes>>20, strings.Join(names, ", "))
}
