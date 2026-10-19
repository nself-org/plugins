package bootstrap

// Purpose: the registry of OS families. A family knows how to turn one kind of node (apt, dnf, apk
// Linux; P7-NODE-31 adds darwin) into a system node: the agent user, one job user per slot, the sudoers
// rules, the agent directory and the service unit.
// Inputs:  RegisterFamily calls from init(); an OS key to look up (a distro id such as "debian", or
// "darwin").
// Outputs: the matching Family.
// Constraints: a name registers once; the first registered family that matches wins; the registry is
//          guarded by a mutex because tests register at run time. An OS no family matches is not an
//          error here: Lookup reports false and the caller falls back (darwin prints brew commands).

import (
	"context"
	"sync"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// Family installs and removes the system-node layout on one kind of OS.
type Family interface {
	// Name is the registry key ("apt", "dnf", "apk", "darwin").
	Name() string
	// Match reports whether this family handles os: a distro id ("debian", "alpine") or a GOOS ("darwin").
	Match(os string) bool
	// Install brings the node to the system layout; it is idempotent (a second run with more slots adds
	// only the missing users and rules).
	Install(ctx context.Context, t remote.Target, o Options) error
	// Uninstall removes everything Install created. o.Target names the node.
	Uninstall(ctx context.Context, o Options) error
}

var (
	regMu    sync.Mutex
	families []Family
)

// RegisterFamily adds a family. A nil family, an empty name or a duplicate name is an E669.
func RegisterFamily(f Family) error {
	if f == nil || f.Name() == "" {
		return NewError(ReasonFamilyEmpty, "a bootstrap family needs a name")
	}
	regMu.Lock()
	defer regMu.Unlock()
	for _, have := range families {
		if have.Name() == f.Name() {
			return NewError(ReasonDuplicate, "bootstrap family %q is already registered", f.Name())
		}
	}
	families = append(families, f)
	return nil
}

// Lookup returns the first registered family that matches os.
func Lookup(os string) (Family, bool) {
	regMu.Lock()
	defer regMu.Unlock()
	for _, f := range families {
		if f.Match(os) {
			return f, true
		}
	}
	return nil, false
}

// unregister removes a family by name (tests).
func unregister(name string) {
	regMu.Lock()
	defer regMu.Unlock()
	for i, f := range families {
		if f.Name() == name {
			families = append(families[:i], families[i+1:]...)
			return
		}
	}
}

func init() {
	for _, f := range linuxFamilies() {
		if err := RegisterFamily(f); err != nil {
			panic(err)
		}
	}
}
