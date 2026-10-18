package services

// Purpose: what the engine knows about each nSelf component it can start as a
// CI service dependency: the `.env` keys it sets through `nself config set`,
// the ports, and the protocol probe. A component that is in the CLI's list but
// has no row here is refused with E639 instead of guessed at.
// Inputs:  a component name from `.nself/ci.yaml` or `--service`.
// Outputs: its row.
// Constraints: keys are the CLI's own; the engine never writes a compose file.

import "sort"

// component describes one startable nSelf component.
type component struct {
	Name          string
	Required      bool // part of every stack: there is no `config services enable`
	ComposeName   string
	ContainerPort int
	PortKey       string
	VersionKey    string
	PasswordKey   string
	UserKey       string
	DBKey         string
	User, DB      string
	Scheme        string
	Probe         probeKind
}

// components is the table of supported dependencies.
var components = map[string]component{
	"postgres": {
		Name: "postgres", Required: true, ComposeName: "postgres", ContainerPort: 5432,
		PortKey: "POSTGRES_PORT", VersionKey: "POSTGRES_VERSION", PasswordKey: "POSTGRES_PASSWORD",
		UserKey: "POSTGRES_USER", DBKey: "POSTGRES_DB", User: "nselfci", DB: "nselfci",
		Scheme: "postgres", Probe: probePostgres,
	},
	"redis": {
		Name: "redis", ComposeName: "redis", ContainerPort: 6379,
		PortKey: "REDIS_PORT", VersionKey: "REDIS_VERSION", PasswordKey: "REDIS_PASSWORD",
		Scheme: "redis", Probe: probeRedis,
	},
}

// supported lists the component names the engine can start, sorted.
func supported() []string {
	out := make([]string, 0, len(components))
	for n := range components {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
