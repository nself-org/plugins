package main

// Purpose: `nself-ci-agent join --advertise` (P7-NODE-12, Epic D12): publish one LAN record during the
//          pairing window so `nself ci nodes discover` can list this node as a candidate.
// Inputs:  the `--advertise` flag of join; NSELF_CI_DISCOVERY and ci.discovery in the user config.
// Outputs: for the window only, one mDNS record (random instance and host name, one TXT record: a random
//          8-character id) and a line on stderr naming that id; the advertisement stops when join
//          finishes enrolling, when the window (the pairing code lifetime) ends, or when the process ends.
// Constraints: join.go does not declare the flag and is outside this Ticket's scope, so init removes it
//          from os.Args before join parses them (internal/discovery.StripAdvertiseFlag) and, only when the
//          flag was given, registers a PreStart hook that starts the advertisement and a join hook that
//          stops it. Without the flag nothing is registered (TestGuardIsRegisteredFirst keeps holding).
//          This file sorts before prodguard.go, so with the flag the hook runs the production guard itself
//          first (the guard then runs a second time as its own hook and may repeat a warning); an E667
//          refusal stops join before anything is published.

import (
	"context"
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
	if len(os.Args) >= 2 && os.Args[1] == "join" {
		rest, on := discovery.StripAdvertiseFlag(os.Args[2:])
		os.Args = append([]string{os.Args[0], "join"}, rest...)
		advertiser.requested = on
	}
	if !advertiser.requested {
		return // without the flag nothing is registered, so the PreStart order stays prodguard first
	}
	must(RegisterPreStart(advertiseName, advertisePreStart))
	must(pairing.RegisterJoinHook(advertiseName, advertiseStop))
}

func advertisePreStart(ctx context.Context, command string) error {
	if command != "join" {
		return nil
	}
	if err := prodGuard(ctx, command); err != nil {
		return err
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
