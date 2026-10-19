package discovery

// Purpose: the per-window identity an advertisement carries: a random 8-character id (the only TXT
//          record), a random instance name and a random host name (the SRV target and the A-record name).
// Inputs:  an entropy reader (crypto/rand in production).
// Outputs: Identity.
// Constraints: nothing stable, nothing derived from the machine: no host name, user name, MAC, machine id
//          or address enters any of the three values, and every window draws new ones (RFC 8882 privacy).

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
)

// Identity is what one advertisement window publishes.
type Identity struct {
	ID       string // 8 hex characters, printed by `join`, the only TXT record
	Instance string // 16 hex characters, the DNS-SD instance label
	Host     string // 16 hex characters + ".local.", the SRV target and A-record name
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}$`)

func newIdentity(r io.Reader) (Identity, error) {
	if r == nil {
		r = rand.Reader
	}
	b := make([]byte, 4+8+8)
	if _, err := io.ReadFull(r, b); err != nil {
		return Identity{}, fmt.Errorf("discovery: random window identity: %w", err)
	}
	return Identity{ID: hex.EncodeToString(b[:4]), Instance: hex.EncodeToString(b[4:12]), Host: hex.EncodeToString(b[12:]) + "." + Domain}, nil
}
