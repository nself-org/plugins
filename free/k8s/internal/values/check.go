// Purpose: the parity check behind `nself k8s values --check`: compare an
// existing values.yaml with the services of the current compose model.
//
// Inputs: the bytes of an existing values.yaml, the resolved Model.
//
// Outputs: []Diff, empty when the file matches the model.
//
// Constraints: reports a service that is missing from the values file, extra
// in it, or whose image or tag differs. A service listed under unsupported
// counts as present (it is accounted for). Diffs are sorted by service then
// kind and never include env values.
package values

import (
	"bytes"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// Diff kinds.
const (
	DiffMissing = "missing"
	DiffExtra   = "extra"
	DiffDiffers = "differs"
)

// Diff is one parity difference.
type Diff struct {
	Service string
	Kind    string
	Detail  string
}

func (d Diff) String() string {
	return fmt.Sprintf("%s: %s (%s)", d.Service, d.Kind, d.Detail)
}

// Check compares existing with the values the model would produce today.
func Check(existing []byte, m *Model) ([]Diff, error) {
	var have Values
	dec := yaml.NewDecoder(bytes.NewReader(existing))
	if err := dec.Decode(&have); err != nil {
		return nil, fmt.Errorf("existing values file is not valid: %w", err)
	}
	want, _, err := Map(m, nil)
	if err != nil {
		return nil, err
	}
	var diffs []Diff
	for _, name := range sortedKeys(m.Services) {
		_, mapped := have.Services[name]
		if !mapped && !hasUnsupported(have.Unsupported, name) {
			diffs = append(diffs, Diff{name, DiffMissing, "in compose, not in values"})
			continue
		}
		w, ok := want.Services[name]
		if !ok || !mapped {
			continue
		}
		if h := have.Services[name]; h.Image != w.Image || h.Tag != w.Tag || h.Digest != w.Digest {
			diffs = append(diffs, Diff{name, DiffDiffers,
				fmt.Sprintf("values %s, compose %s", ref(h), ref(w))})
		}
	}
	for _, name := range sortedKeys(have.Services) {
		if _, ok := m.Services[name]; !ok {
			diffs = append(diffs, Diff{name, DiffExtra, "in values, not in compose"})
		}
	}
	for _, u := range have.Unsupported {
		if _, ok := m.Services[u.Name]; !ok {
			diffs = append(diffs, Diff{u.Name, DiffExtra, "listed unsupported in values, not in compose"})
		}
	}
	sort.SliceStable(diffs, func(i, j int) bool { return diffs[i].Service < diffs[j].Service })
	return diffs, nil
}

func ref(s Service) string {
	r := s.Image + ":" + s.Tag
	if s.Digest != "" {
		r += "@" + s.Digest
	}
	return r
}

func hasUnsupported(list []Unsupported, name string) bool {
	for _, u := range list {
		if u.Name == name {
			return true
		}
	}
	return false
}
