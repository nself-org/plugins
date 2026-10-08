package detect

import (
	"errors"
	"io/fs"
	"sort"
)

// Detector describes an ecosystem and its check binding. Empty check IDs are
// rejected so future ecosystems cannot silently claim runnable checks.
type Detector struct {
	ID        string
	Ecosystem string
	CheckIDs  []string
	Detect    func(Snapshot) []Fact
}

// Registry stores independently registered detectors.
type Registry struct{ detectors map[string]Detector }

func NewRegistry() *Registry { return &Registry{detectors: map[string]Detector{}} }

// Register rejects duplicate IDs and unbound detectors.
func (r *Registry) Register(d Detector) error {
	if d.ID == "" || d.Ecosystem == "" || d.Detect == nil || len(d.CheckIDs) == 0 {
		return errors.New("detector requires id, ecosystem, check IDs and function")
	}
	if r.detectors == nil {
		r.detectors = map[string]Detector{}
	}
	if _, ok := r.detectors[d.ID]; ok {
		return errors.New("duplicate detector: " + d.ID)
	}
	r.detectors[d.ID] = d
	return nil
}

// Detect runs only detectors whose check IDs are in the supplied catalogue.
// The catalogue hook is filled by the check package in P7-CI-34.
func (r *Registry) Detect(source fs.FS, checkRegistered func(string) bool) ([]Fact, error) {
	if checkRegistered == nil {
		return nil, errors.New("check registry hook required")
	}
	s, err := NewSnapshot(source)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(r.detectors))
	for id := range r.detectors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var facts []Fact
	for _, id := range ids {
		d := r.detectors[id]
		bound := false
		for _, check := range d.CheckIDs {
			if checkRegistered(check) {
				bound = true
				break
			}
		}
		if !bound {
			continue
		}
		facts = append(facts, d.Detect(s)...)
	}
	stack, uncertainStack := false, false
	for _, f := range facts {
		if f.Kind == "stack" && f.Confidence == Certain {
			stack = true
		}
		if f.Kind == "manifest" && f.Confidence == Unknown {
			uncertainStack = true
		}
	}
	if !stack && !uncertainStack {
		facts = append(facts, Fact{Kind: "stack", Name: "no_stack", Path: ".", Confidence: Certain})
	}
	sort.Slice(facts, func(i, j int) bool {
		if facts[i].Kind != facts[j].Kind {
			return facts[i].Kind < facts[j].Kind
		}
		if facts[i].Path != facts[j].Path {
			return facts[i].Path < facts[j].Path
		}
		return facts[i].Name < facts[j].Name
	})
	unique := facts[:0]
	for _, f := range facts {
		if len(unique) > 0 {
			prev := unique[len(unique)-1]
			if prev.Kind == f.Kind && prev.Path == f.Path && prev.Name == f.Name && prev.Confidence == f.Confidence {
				continue
			}
		}
		unique = append(unique, f)
	}
	return unique, nil
}

// DefaultRegistry binds the built-in ecosystems to check IDs owned by gates.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	for _, d := range []Detector{
		{ID: "compose", Ecosystem: "container", CheckIDs: []string{"container.compose"}, Detect: detectCompose},
		{ID: "docker", Ecosystem: "container", CheckIDs: []string{"container.docker"}, Detect: detectDocker},
		{ID: "flutter", Ecosystem: "flutter", CheckIDs: []string{"flutter.test"}, Detect: detectFlutter},
		{ID: "go", Ecosystem: "go", CheckIDs: []string{"go.test"}, Detect: detectGo},
		{ID: "lockfiles", Ecosystem: "inputs", CheckIDs: []string{"inputs.lockfiles"}, Detect: detectLockfiles},
		{ID: "node", Ecosystem: "node", CheckIDs: []string{"node.test"}, Detect: detectNode},
		{ID: "rust", Ecosystem: "rust", CheckIDs: []string{"rust.test"}, Detect: detectRust},
	} {
		_ = r.Register(d)
	}
	return r
}
