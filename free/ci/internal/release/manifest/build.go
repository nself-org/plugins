package manifest

// Purpose: Build, the one producer of release-manifest payload bytes.
// Inputs:  a Manifest with its schema field set.
// Outputs: the canonical payload: UTF-8 JSON, object keys sorted, no
// insignificant whitespace, artifacts sorted by name, image manifests sorted by
// platform, arrays never null. The same Manifest always gives the same bytes.
// Constraints: Build validates first, so a producer cannot sign a payload the
// verifiers refuse; it never mutates its argument.

import (
	"bytes"
	"encoding/json"
	"sort"
)

// Build returns the canonical payload bytes of m.
func Build(m Manifest) ([]byte, error) {
	m = normalize(m)
	if err := validate(m); err != nil {
		return nil, err
	}
	b, err := canonical(m)
	if err != nil {
		return nil, err
	}
	if len(b) > maxPayload {
		return nil, invalid("payload is %d bytes, more than the %d a verifier reads", len(b), maxPayload)
	}
	return b, nil
}

// normalize copies m with its lists sorted and non-nil.
func normalize(m Manifest) Manifest {
	arts := make([]Artifact, len(m.Artifacts))
	for i, a := range m.Artifacts {
		if a.Image != nil {
			im := *a.Image
			im.Manifests = append([]ImageManifest{}, im.Manifests...)
			sort.Slice(im.Manifests, func(x, y int) bool { return im.Manifests[x].Platform < im.Manifests[y].Platform })
			a.Image = &im
		}
		arts[i] = a
	}
	sort.Slice(arts, func(i, j int) bool { return arts[i].Name < arts[j].Name })
	m.Artifacts = arts
	return m
}

// canonical encodes v with sorted object keys and no insignificant whitespace.
func canonical(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // a size above 2^53 must survive the round trip
	var tree any
	if err = dec.Decode(&tree); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err = enc.Encode(tree); err != nil { // map keys are written in sorted order
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
