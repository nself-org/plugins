package notify

// Purpose: the Notifier interface and its registry (P7-AUTO-18), the same plug-in shape
// as the other registries of the CI plugin. A sink type registers a Factory; an operator
// names sinks in `notify.sinks`; the factory turns one spec into a Notifier.
// Inputs: SinkSpec values from policy, Deps from the run hook (and tests).
// Outputs: Notifier values; Build refuses a bad spec without echoing its URL or argv.
// Constraints: no vendor name appears here or in any sink: an external tool (a chat bot)
// attaches through the command sink. Registrations happen at init.

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"sort"
	"sync"
)

// Notifier delivers one event to one destination. It must honour ctx: the engine cancels it
// when its time budget ends.
type Notifier interface {
	Name() string
	Notify(ctx context.Context, ev Event) error
}

// SinkSpec is one entry of notify.sinks. Unknown fields are refused by the policy decoder.
type SinkSpec struct {
	Type   string   `json:"type"`
	Name   string   `json:"name,omitempty"`
	URL    string   `json:"url,omitempty"`    // webhook: https only, no credentials, query or fragment
	Secret string   `json:"secret,omitempty"` // webhook: HMAC-SHA256 key; the signature goes in Authorization
	Argv   []string `json:"argv,omitempty"`   // command: never run through a shell
}

// Deps are the seams a sink needs; the zero value is production.
type Deps struct {
	Out          io.Writer     // stdout sink target (default os.Stdout)
	Home         string        // CI home of the command funnel ("" = NSELF_CI_HOME or ~/.nself/ci)
	AllowPrivate bool          // notify.allow_private: webhook may dial a private address
	Runner       CommandRunner // command sink process runner (default: the exec funnel)
	TLSConfig    *tls.Config   // webhook TLS roots (tests only)
}

// Factory builds a Notifier from a spec.
type Factory func(spec SinkSpec, d Deps) (Notifier, error)

var registry = struct {
	sync.RWMutex
	byType map[string]Factory
}{byType: map[string]Factory{}}

// Register adds a sink type. A duplicate or empty type is refused.
func Register(typ string, f Factory) error {
	if typ == "" || f == nil {
		return fmt.Errorf("notify: sink type and factory required")
	}
	registry.Lock()
	defer registry.Unlock()
	if _, ok := registry.byType[typ]; ok {
		return fmt.Errorf("notify: sink type already registered: %s", typ)
	}
	registry.byType[typ] = f
	return nil
}

// Types lists the registered sink types, sorted.
func Types() []string {
	registry.RLock()
	defer registry.RUnlock()
	out := make([]string, 0, len(registry.byType))
	for t := range registry.byType {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// New builds one sink. Its errors name the sink by its name or type, never by URL or argv.
func New(spec SinkSpec, d Deps) (Notifier, error) {
	registry.RLock()
	f, ok := registry.byType[spec.Type]
	registry.RUnlock()
	if !ok {
		return nil, coded(fmt.Sprintf("unknown sink type (have %v)", Types()))
	}
	return f(spec, d)
}

// Build builds every sink of a config, in order. A sink that cannot be built is skipped and
// reported; the others still work.
func Build(specs []SinkSpec, d Deps) (sinks []Notifier, errs []error) {
	for _, s := range specs {
		n, err := New(s, d)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		sinks = append(sinks, n)
	}
	return sinks, errs
}

func init() {
	for typ, f := range map[string]Factory{"stdout": newStdout, "webhook": newWebhook, "command": newCommand} {
		if err := Register(typ, f); err != nil {
			panic(err)
		}
	}
}

// nameOf is the sink's identity in counters and errors.
func nameOf(s SinkSpec) string {
	if s.Name != "" {
		return s.Name
	}
	return s.Type
}
