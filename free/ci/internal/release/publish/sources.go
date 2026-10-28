package publish

// Purpose: the extension point of `nself ci artifacts sign <run>`: the local
// source is built in; P7-CACHE-17 registers `--coordinator` (fetch the inputs
// from a TEAM coordinator and sign locally) without editing cmd/artifacts_sign.go.
// Inputs:  RegisterSignSource(name, flags, fn) from an init().
// Outputs: the registered sources, in name order.
// Constraints: registrations happen at init, before any command runs; a name is
// registered once and is a plain flag name; the source whose flag the operator
// sets is the one that runs (at most one).

import (
	"context"
	"flag"
	"fmt"
	"regexp"
	"sort"
	"sync"
)

// SignSource is an alternative origin of the release inputs.
type SignSource struct {
	// Name is the flag that selects the source (for example "coordinator").
	Name string
	// Flags declares the source's flags, Name's own among them.
	Flags func(fs *flag.FlagSet)
	// Companions names the flags that only mean something with this source (for
	// example "out" of --coordinator); one set without the source flag is refused.
	Companions []string
	// Run signs the run using the values parsed into fs.
	Run func(ctx context.Context, runID string, fs *flag.FlagSet) (Release, error)
}

var (
	sourcesMu sync.Mutex
	sources   = map[string]SignSource{}
	nameRe    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
)

// RegisterSignSource adds a source; it panics on a duplicate or invalid name (init-time bug).
// companions are the flags that need the source's own flag (see SignSource.Companions).
func RegisterSignSource(name string, flags func(fs *flag.FlagSet), fn func(ctx context.Context, runID string, fs *flag.FlagSet) (Release, error), companions ...string) {
	sourcesMu.Lock()
	defer sourcesMu.Unlock()
	if _, dup := sources[name]; dup || !nameRe.MatchString(name) || fn == nil {
		panic(fmt.Sprintf("publish: invalid or duplicate sign source %q", name))
	}
	sources[name] = SignSource{Name: name, Flags: flags, Companions: companions, Run: fn}
}

// SignSources lists the registered sources in name order.
func SignSources() []SignSource {
	sourcesMu.Lock()
	defer sourcesMu.Unlock()
	out := make([]SignSource, 0, len(sources))
	for _, s := range sources {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
