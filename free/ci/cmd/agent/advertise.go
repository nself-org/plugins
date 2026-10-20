package main

// Purpose: `nself-ci-agent join --advertise` (P7-NODE-12, Epic D12): publish one LAN record during the
//          pairing window so `nself ci nodes discover` can list this node as a candidate.
// Inputs:  the `--advertise` flag of join; NSELF_CI_DISCOVERY and ci.discovery in the user config.
// Outputs: for the window only, one mDNS record (random instance and host name, one TXT record: a random
//          8-character id) and a line on stderr naming that id; the advertisement stops when join
//          finishes enrolling, when the window (the pairing code lifetime) ends, or when the process ends.
// Constraints: join declares the flag through RegisterJoinFlags, so `join -h` lists it; the start step runs
//          after the PreStart hooks, so the production guard has already run once (an E667 or E663 refusal
//          stops join before anything is published). The stop hook is registered always and does nothing
//          unless an advertisement is live.

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/nself-org/plugins/free/ci/internal/discovery"
	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
)

const advertiseName = "advertise"

// advertiser is the join invocation's advertisement state; advertiseGuard is replaced by tests of this binary.
var advertiser struct {
	live *discovery.Advertisement
	out  io.Writer
}

var advertiseGuard = &discovery.Guard{}

func init() {
	must(RegisterJoinFlags(advertiseName, declareAdvertise))
	must(pairing.RegisterJoinHook(advertiseName, advertiseStop))
}

// declareAdvertise adds --advertise to join and returns the step that publishes the record when it is set.
func declareAdvertise(fs *flag.FlagSet) JoinStart {
	on := fs.Bool("advertise", false, "publish a LAN record during the pairing window so `nself ci nodes discover` lists this node")
	return func(ctx context.Context, sio IO) error {
		advertiser.live, advertiser.out = nil, sio.Stderr
		if !*on {
			return nil
		}
		a, err := advertiseGuard.Advertise(ctx, pairing.OfferTTL, discovery.SSHPort, nil)
		if err != nil {
			return err
		}
		advertiser.live = a
		a.Announce(sio.Stderr)
		return nil
	}
}

// advertiseStop ends the advertisement once the enrollment is saved.
func advertiseStop(_ context.Context, _ enroll.JoinResult, _ string) error {
	if advertiser.live != nil {
		advertiser.live.Stop()
		fmt.Fprintln(advertiser.out, "LAN discovery stopped.")
	}
	return nil
}
