package coordclient

// Purpose: where a `nself ci` invocation goes (P7-NODE-19, Epic D14): the coordinator client's target
// resolution. Order: --coordinator flag > NSELF_CI_COORDINATOR > `ci.coordinator` in the user config > a
// live local `ci serve` on the same NSELF_CI_HOME (lock held) > in-process.
// Inputs:  the flag value, the environment, the CI home, the repository root (only to warn about a
//          committed value).
// Outputs: a Target (kind, URL or socket path, credential file paths) and warning lines.
// Constraints: the revision under test never chooses the coordinator or supplies a credential: a
//          `ci.coordinator*` key in the committed .nself/ci.yaml is ignored and costs one warning line;
//          a coordinator URL must be https (E664 plain_http), have a host and carry no user info, query
//          or fragment; resolution reads files and probes the lock, it opens no store and no network.

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
)

// Env names the variables the client reads.
const (
	EnvCoordinator = "NSELF_CI_COORDINATOR"
	EnvCA          = "NSELF_CI_COORDINATOR_CA"
	EnvCert        = "NSELF_CI_COORDINATOR_CERT"
	EnvKey         = "NSELF_CI_COORDINATOR_KEY"
	EnvTokenFile   = "NSELF_CI_COORDINATOR_TOKEN_FILE"
	EnvUserConfig  = "NSELF_CI_USER_CONFIG"
)

// Kind says how a run reaches the fabric.
type Kind int

// The three kinds, in resolution order after the explicit sources.
const (
	InProcess  Kind = iota // PERSONAL: the engine of this process
	LocalServe             // a live `ci serve` on this CI home, over serve.sock
	Remote                 // a TEAM coordinator over TLS
)

func (k Kind) String() string { return [...]string{"in-process", "local-serve", "remote"}[k] }

// Creds are the credential files of a remote target; none is read until a request is made.
type Creds struct{ CA, Cert, Key, TokenFile string }

// Target is a resolved destination.
type Target struct {
	Kind   Kind
	URL    string // Remote: the https base URL
	Socket string // LocalServe: the unix socket path
	Creds  Creds
}

// Active reports whether runs and reads leave this process.
func (t Target) Active() bool { return t.Kind != InProcess }

// Inputs are the sources of one resolution.
type Inputs struct {
	Flag     string              // --coordinator
	Getenv   func(string) string // default os.Getenv
	Home     string              // CI home; "" = NSELF_CI_HOME or ~/.nself/ci
	RepoRoot string              // for the committed-value warning; "" skips it
}

// Home is the CI home for env.
func Home(getenv func(string) string) string {
	if v := getenv("NSELF_CI_HOME"); v != "" {
		return v
	}
	u, _ := os.UserHomeDir()
	return filepath.Join(u, ".nself", "ci")
}

// Resolve picks the target and returns the warning lines the caller prints once each.
func Resolve(in Inputs) (Target, []string, error) {
	if in.Getenv == nil {
		in.Getenv = os.Getenv
	}
	home := in.Home
	if home == "" {
		home = Home(in.Getenv)
	}
	warns := committedWarnings(in.RepoRoot)
	uc, err := loadUserConfig(in.Getenv, home)
	if err != nil {
		return Target{}, warns, err
	}
	creds := Creds{CA: firstNonEmpty(in.Getenv(EnvCA), uc.CA), Cert: firstNonEmpty(in.Getenv(EnvCert), uc.Cert),
		Key: firstNonEmpty(in.Getenv(EnvKey), uc.Key), TokenFile: firstNonEmpty(in.Getenv(EnvTokenFile), uc.TokenFile)}
	if raw := firstNonEmpty(in.Flag, in.Getenv(EnvCoordinator), uc.Coordinator); raw != "" {
		u, err := ParseURL(raw)
		if err != nil {
			return Target{}, warns, err
		}
		return Target{Kind: Remote, URL: u, Creds: creds}, warns, nil
	}
	if ServeHoldsLock(home) {
		return Target{Kind: LocalServe, Socket: SocketFor(home)}, warns, nil
	}
	return Target{Kind: InProcess}, warns, nil
}

// ParseURL validates a coordinator address and returns it without a trailing slash.
func ParseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	switch {
	case err != nil || u.Host == "":
		return "", clientapi.Failf(clientapi.ReasonBadRequest, "the coordinator address is not a URL")
	case u.Scheme != "https":
		return "", clientapi.Failf(clientapi.ReasonPlainHTTP, "the coordinator address must be https; credentials never travel in clear text")
	case u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/"):
		return "", clientapi.Failf(clientapi.ReasonBadRequest, "the coordinator address is a bare https://host[:port]")
	}
	return "https://" + u.Host, nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// committedWarnings looks for coordinator or credential keys in the committed config. They are read only
// to say they are ignored: the revision under test may not choose where its own run goes.
func committedWarnings(root string) []string {
	if root == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(root, ".nself", "ci.yaml"))
	if err != nil {
		return nil
	}
	var doc map[string]any
	if yaml.Unmarshal(raw, &doc) != nil {
		return nil
	}
	ci, _ := doc["ci"].(map[string]any)
	var keys []string
	for k := range ci {
		if strings.HasPrefix(k, "coordinator") {
			keys = append(keys, "ci."+k)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys) // one line, the same text on every run
	return []string{"warning: ignoring " + strings.Join(keys, ", ") + " in .nself/ci.yaml; the coordinator comes from --coordinator, " + EnvCoordinator + " or your user config"}
}
