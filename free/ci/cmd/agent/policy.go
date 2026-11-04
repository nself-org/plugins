package main

// Purpose: `nself-ci-agent policy [--set <base64url JSON>]` (D-1226): the receiving end of `nself ci nodes
//          set` for a LAN node. It stores the node's contention policy in <state>/contention.json, which the
//          lease gate reads on every lease (contention_wire.go), and prints the stored policy.
// Inputs:  --set: base64url (no padding) of one store.NodeSettings JSON object.
// Outputs: the stored policy as one JSON line, or "none"; exit 0, 1 (invalid policy, state), 2 (usage).
// Constraints: the value is decoded strictly and validated before it replaces the file (contention.SavePolicy
//          writes it atomically, 0600); a refused value leaves the old policy in place.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/agent/contention"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
)

func init() {
	must(RegisterSubcommand("policy", "show or set (--set) this node's contention policy (reservation, battery, interactive)", runPolicy))
}

func runPolicy(_ context.Context, args []string, sio IO) int {
	fs := flag.NewFlagSet("policy", flag.ContinueOnError)
	fs.SetOutput(sio.Stderr)
	set := fs.String("set", "", "base64url JSON of the policy to store")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent policy [--set BASE64URL]")
		return 2
	}
	state, err := stateDir()
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	if *set != "" {
		s, err := decodePolicy(*set)
		if err == nil {
			err = contention.SavePolicy(state, s)
		}
		if err != nil {
			_, _ = fmt.Fprintln(sio.Stderr, "error: policy:", err)
			return 1
		}
	}
	s, ok, err := contention.LoadPolicy(state)
	switch {
	case err != nil:
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	case !ok:
		_, _ = fmt.Fprintln(sio.Stdout, "none")
		return 0
	}
	raw, _ := json.Marshal(s)
	_, _ = fmt.Fprintln(sio.Stdout, string(raw))
	return 0
}

func decodePolicy(arg string) (reservation.Settings, error) {
	var s reservation.Settings
	raw, err := base64.RawURLEncoding.DecodeString(arg)
	if err != nil {
		return s, fmt.Errorf("not base64url: %w", err)
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&s); err != nil {
		return s, err
	}
	return s, s.Validate()
}
