package main

// Purpose: `nself-ci-agent join --advertise` (P7-NODE-12, Epic D12): publish one LAN record during the
//          pairing window so `nself ci nodes discover` can list this node as a candidate.
// Inputs:  the `--advertise` flag of join; NSELF_CI_DISCOVERY and ci.discovery in the user config.
// Outputs: for the window only, one mDNS record (random instance and host name, one TXT record: a random
//          8-character id) and a line on stderr naming that id; the advertisement stops when join
//          finishes enrolling, when the window (the pairing code lifetime) ends, or when the process ends.
// Constraints: join declares the flag through RegisterFlags (so `join -h` lists it); a PostStart hook
//          starts the advertisement after the PreStart hooks (the production guard, once) passed, and a join
//          hook stops it. Without the flag both hooks do nothing. An E667 refusal stops join before
//          anything is published. The window is pairing.OfferTTL and the discovery guard caps it at
//          discovery.MaxWindow (a test keeps MaxWindow >= OfferTTL).

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/discovery"
	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
)

const advertiseName = "advertise"

// advertiser is the join invocation's advertisement state; guard is replaced by tests of this binary.
var advertiser struct {
	requested bool
	live      *discovery.Advertisement
}

var advertiseGuard = &discovery.Guard{}

func init() {
	must(RegisterFlags("join", func(fs *flag.FlagSet) {
		fs.BoolVar(&advertiser.requested, "advertise", false, "publish one LAN record during the pairing window so nself ci nodes discover can list this node (needs LAN discovery on)")
	}))
	must(RegisterPostStart(advertiseName, advertiseStart))
	must(pairing.RegisterJoinHook(advertiseName, advertiseStop))
}

func advertiseStart(ctx context.Context, command string) error {
	if command != "join" || !advertiser.requested {
		return nil
	}
	a, err := advertiseGuard.Advertise(ctx, pairing.OfferTTL, discovery.SSHPort, nil)
	if err != nil {
		return err
	}
	advertiser.live = a
	a.Announce(os.Stderr)
	return nil
}

// advertiseStop ends the advertisement once the enrollment is saved.
func advertiseStop(_ context.Context, _ enroll.JoinResult, _ string) error {
	if advertiser.live != nil {
		advertiser.live.Stop()
		fmt.Fprintln(os.Stderr, "LAN discovery stopped.")
	}
	return nil
}
