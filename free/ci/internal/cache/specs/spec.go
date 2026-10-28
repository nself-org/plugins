package specs

// Purpose: the per-ecosystem cache specs (Epic CACHE D5). A Spec says which
// directories a package manager or compiler fills, which environment
// variables point it at them, which environment values and tool versions
// belong in the key, and which detector facts switch it on.
// Inputs:  detect.Fact values (stack, package_manager, lockfile, container).
// Outputs: Register, For, Get, Spec.
// Constraints: pure; no I/O. A spec id matches the entries spec id pattern
// ([a-z0-9][a-z0-9._-]{0,63}) and a Dir name is one clean path element, so a
// blob path can never leave its entry. Cargo's target/ and any other build
// output directory is never a Dir (R-SEC-18: a cache holds downloads and
// content-addressed compiler caches, not build products).

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/detect"
)

// Dir is one cache path of a spec: a blob of the entry, a directory under the
// attempt's cache root, and the variable (if any) that points the tool at it.
type Dir struct{ Name, Env string }

// Probe is one toolchain probe of a process-level job: Argv runs through the
// exec funnel and its first output line joins the key as Name=<line>.
type Probe struct {
	Name string
	Argv []string
}

// Spec is one ecosystem cache. Version changes whenever Dirs, Env bindings or
// the packing rules change, so an older entry misses instead of mis-restoring.
type Spec struct {
	ID, Version, Ecosystem string
	Dirs                   []Dir
	// EnvAllow names the variables whose values are hashed into the key; a
	// trailing * is a prefix. Values are never stored or logged.
	EnvAllow []string
	// Lockfiles are the detector lockfile managers whose digests key the spec.
	Lockfiles []string
	Probes    []Probe
	// InflateScan is retained for spec compatibility. Every save now scans
	// gzip and ZIP payloads, regardless of this legacy flag.
	InflateScan bool
	// ContainerOnly specs apply to container-level jobs only.
	ContainerOnly bool
	// RootEnv, when set, is bound to the spec's own directory (cargo's CARGO_HOME,
	// whose registry and git subdirectories are the Dirs).
	RootEnv string
	// Seed names files of the user's tool home copied into the attempt dir
	// before the job (cargo's config.toml), because the bound variable hides
	// the user's own directory.
	Seed    SeedFile
	applies func(facts []detect.Fact) bool
}

// SeedFile describes a tool-home file copied into a spec's first dir parent.
type SeedFile struct {
	HomeEnv string   // variable naming the tool home, e.g. CARGO_HOME
	HomeDir string   // default under the user's home, e.g. .cargo
	Names   []string // files copied when present, e.g. config.toml, config
	// Blockers are files of the tool home whose presence disables the spec,
	// because rebinding the home would hide them (cargo's credentials).
	Blockers []string
}

var (
	idPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	dirPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	mu         sync.RWMutex
	registry   = map[string]Spec{}
)

// Register adds s; an invalid or repeated spec is an error.
func Register(s Spec) error {
	switch {
	case !idPattern.MatchString(s.ID) || s.Version == "" || s.Ecosystem == "" || len(s.Dirs) == 0:
		return fmt.Errorf("specs: spec %q needs a valid id, version, ecosystem and dirs", s.ID)
	case s.applies == nil:
		return fmt.Errorf("specs: spec %q has no detector rule", s.ID)
	}
	seen := map[string]bool{}
	for _, d := range s.Dirs {
		if !dirPattern.MatchString(d.Name) || seen[d.Name] || strings.EqualFold(d.Name, "target") {
			return fmt.Errorf("specs: spec %q has an invalid, repeated or build-output dir %q", s.ID, d.Name)
		}
		seen[d.Name] = true
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[s.ID]; dup {
		return errors.New("specs: duplicate spec " + s.ID)
	}
	registry[s.ID] = s
	return nil
}

func mustRegister(s Spec) {
	if err := Register(s); err != nil {
		panic(err)
	}
}

// Get returns the spec with the id.
func Get(id string) (Spec, bool) {
	mu.RLock()
	defer mu.RUnlock()
	s, ok := registry[id]
	return s, ok
}

// For returns, in id order, the specs the detector facts switch on. Container
// specs apply only when container is true.
func For(facts []detect.Fact, container bool) []Spec {
	mu.RLock()
	defer mu.RUnlock()
	var out []Spec
	for _, s := range registry {
		if s.ContainerOnly && !container {
			continue
		}
		if s.applies(facts) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// has reports a certain fact of the kind and name.
func has(facts []detect.Fact, kind, name string) bool {
	for _, f := range facts {
		if f.Kind == kind && f.Name == name && f.Confidence == detect.Certain {
			return true
		}
	}
	return false
}
