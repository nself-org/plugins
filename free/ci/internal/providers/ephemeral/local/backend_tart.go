package local

// Purpose: the Apple-silicon backend: one Tart clone per job at the `vm` level (P7-TRUST-06),
// with Apple's two-VM limit reported as capacity.
// Inputs:  the registered `vm` profile and the facts its probe reports (slots, running VMs).
// Outputs: an Offer whose Total is the VM slots the host has left for us (Running lists the clones that
//          hold them), Teardown through the profile (stop, delete, verified gone), and no Leftovers (Reclaim runs the profile sweep).
// Constraints: no Tart, a missing base image digest or a non-arm64 host is an E698, never a
//          weaker level; a host with both slots busy is not an error (Total tells the provider to
//          offer nothing now). Tart keeps its clones' owner in a locked marker file, so clones left by
//          a killed coordinator are removed by the profile's own lock-proven sweep (run by Prepare),
//          not by a label sweep: a clone is never listed with a coordinator it cannot prove.

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/isolation/tart"
)

type tartBackend struct {
	vm       isolation.Profile
	platform string
}

// NewTartBackend builds the macOS backend over the registered `vm` profile.
func NewTartBackend(vm isolation.Profile) Backend {
	return &tartBackend{vm: vm, platform: "darwin/" + runtime.GOARCH}
}

func (b *tartBackend) Platform() string { return b.platform }

func (b *tartBackend) Probe(ctx context.Context) (Offer, error) {
	if b.vm == nil {
		return Offer{}, unavailable(ReasonNoAdapter, nil, "the Tart vm profile is not registered")
	}
	av := b.vm.Probe(ctx)
	if av.Reason != tart.ReasonSlots && !av.Available() { // busy (every slot in use) is not unavailable
		return Offer{}, unavailable(ReasonNoTart, nil, "%s", reasonText(av))
	}
	// Total is what Apple's limit leaves after VMs that are not nself-ci clones
	total, foreign := atoi(av.Facts["slots_total"], tart.MaxSlots), atoi(av.Facts["foreign_running"], 0)
	off := Offer{Level: b.vm.Level(), Total: max(0, total-foreign)}
	if lister, ok := b.vm.(isolation.Lister); ok {
		names, err := lister.List(ctx, "")
		if err != nil {
			return Offer{}, err
		}
		for _, n := range names {
			off.Running = append(off.Running, strings.TrimPrefix(n, tart.ClonePrefix))
		}
	}
	return off, nil
}

func atoi(s string, dflt int) int {
	if n, err := strconv.Atoi(s); err == nil && n >= 0 {
		return n
	}
	return dflt
}

// slotHolder is a profile that can reserve a VM slot in its shared ledger ahead of Wrap (tart.Profile).
type slotHolder interface {
	Hold(ctx context.Context, a isolation.Attempt) error
}

// Prepare checks Tart and holds the grant's slot in the profile's shared ledger, so a non-burst vm job booting in
// this process cannot take it between the grant and the job's Wrap; no free slot is a burst.ErrQuota refusal.
func (b *tartBackend) Prepare(ctx context.Context, a isolation.Attempt) error {
	if err := b.vm.Prepare(ctx, a); err != nil {
		return err
	}
	h, ok := b.vm.(slotHolder)
	if !ok {
		return nil
	}
	err := h.Hold(ctx, a)
	var ie *isolation.Error
	if errors.As(err, &ie) && ie.Reason == tart.ReasonSlots {
		return fmt.Errorf("%w: %s", burst.ErrQuota, ie.Message)
	}
	return err
}

func (b *tartBackend) Teardown(ctx context.Context, a isolation.Attempt) error {
	return b.vm.Teardown(ctx, a)
}

// Reclaim runs the profile's lock-proven orphan sweep (Prepare sweeps after its probe); the outcome does not matter here.
func (b *tartBackend) Reclaim(ctx context.Context) {
	_ = b.vm.Prepare(ctx, isolation.Attempt{ID: "reclaim", CoordinatorID: "tart-reclaim"})
}

func (b *tartBackend) Leftovers(context.Context) ([]Found, error) { return nil, nil }

func reasonText(av isolation.Availability) string {
	if h := av.Facts["hint"]; h != "" {
		return av.Reason + ": " + h
	}
	return av.Reason
}
