package main

// Purpose: freeze parsed serve flags before listeners start.
// Inputs: a parsed flag set. Outputs: immutable values in the serve context.
// Constraints: listeners never read a flag set another serve may still be parsing.
import (
	"context"
	"flag"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
)

type serveSettings struct {
	values   map[string]string
	explicit map[string]bool
}
type serveSettingsKey struct{}

var userAPIFlags struct {
	sync.Mutex
	settings serveSettings
}

func withServeSettings(ctx context.Context, fs *flag.FlagSet) context.Context {
	s := serveSettings{values: map[string]string{}, explicit: map[string]bool{}}
	fs.VisitAll(func(f *flag.Flag) { s.values[f.Name] = f.Value.String() })
	fs.Visit(func(f *flag.Flag) { s.explicit[f.Name] = true })
	userAPIFlags.Lock()
	userAPIFlags.settings = s
	userAPIFlags.Unlock()
	return context.WithValue(ctx, serveSettingsKey{}, s)
}
func settingsFrom(ctx context.Context) serveSettings {
	if s, ok := ctx.Value(serveSettingsKey{}).(serveSettings); ok {
		return s
	}
	userAPIFlags.Lock()
	defer userAPIFlags.Unlock()
	return userAPIFlags.settings
}
func serveFlagContext(ctx context.Context, name string) string { return settingsFrom(ctx).values[name] }
func serveHomeContext(ctx context.Context) string {
	return records.StoreDir(serveFlagContext(ctx, "data-dir"))
}
func serveHome() string { return serveHomeContext(context.Background()) }

// optional logs a failure of a default listener; an explicit listener must bind.
func optional(ctx context.Context, c *coordinator.Coordinator, name string, err error) (func(context.Context) error, error) {
	if settingsFrom(ctx).explicit[name] {
		return nil, err
	}
	c.Logf("--%s listener is off: %v (pass --%s to choose another or to make this an error)", name, err, name)
	return noop, nil
}
