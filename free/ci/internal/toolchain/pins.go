package toolchain

// Purpose: the pin sources of the toolchain image. Every external artifact the
// image (and `artifacts tools install`) uses has one file in toolchain/pins:
// name, version, purpose, owning package, version probe, upstream release feed
// and, per os/arch, the download URL with its sha256.
// Inputs:  an fs.FS holding the pins directory.
// Outputs: validated Pin values sorted by name.
// Constraints: strict decoding (unknown keys fail); a tool source needs an
// https URL and a lower-case 64-hex sha256 (plus an optional binary_sha256 of the extracted executable); a base image is pinned by digest;
// the image row is `pending` (no digest) or `pinned` (digest).

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Pin kinds.
const (
	KindTool      = "tool"
	KindBaseImage = "base-image"
	KindImage     = "image"
)

// Image row states.
const (
	StatusPending = "pending"
	StatusPinned  = "pinned"
)

// Source is one downloadable artifact of a pin.
type Source struct {
	OS     string `yaml:"os"`
	Arch   string `yaml:"arch"`
	URL    string `yaml:"url"`
	SHA256 string `yaml:"sha256"`
	// BinarySHA256 is the optional sha256 of the extracted executable (D-1024),
	// so an installed tool can be authenticated by its bytes, not only by the archive.
	BinarySHA256 string `yaml:"binary_sha256,omitempty"`
}

// Upstream says where the drill finds the newest release of a pin.
type Upstream struct {
	Type string `yaml:"type"`
	Ref  string `yaml:"ref"`
}

// Pin is one row of the toolchain: a tool, a base image or the image itself.
type Pin struct {
	Name     string   `yaml:"name"`
	Kind     string   `yaml:"kind"`
	Version  string   `yaml:"version,omitempty"`
	Purpose  string   `yaml:"purpose"`
	Owner    string   `yaml:"owner"`
	Probe    []string `yaml:"probe,omitempty"`
	Upstream Upstream `yaml:"upstream"`
	Sources  []Source `yaml:"sources,omitempty"`
	Image    string   `yaml:"image,omitempty"`
	Digest   string   `yaml:"digest,omitempty"`
	Status   string   `yaml:"status,omitempty"`
}

var (
	nameRe   = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)
	digestRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

	upstreamTypes = map[string]bool{"github": true, "go": true, "node": true, "rust": true, "gomodule": true, "oci": true, "none": true}
	osNames       = map[string]bool{"linux": true, "darwin": true, "any": true}
	archNames     = map[string]bool{"amd64": true, "arm64": true, "any": true}
)

// LoadPins reads and validates every *.yaml file of dir.
func LoadPins(fsys fs.FS, dir string) ([]Pin, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fail("cannot read the pins directory %s: %v", dir, err)
	}
	var pins []Pin
	seen := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".yaml" {
			continue
		}
		raw, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, fail("cannot read pin %s: %v", e.Name(), err)
		}
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		dec.KnownFields(true)
		var p Pin
		if err := dec.Decode(&p); err != nil {
			return nil, fail("pin %s: %v", e.Name(), err)
		}
		if err := p.Validate(); err != nil {
			return nil, fail("pin %s: %v", e.Name(), err)
		}
		if want := strings.TrimSuffix(e.Name(), ".yaml"); p.Name != want && p.Kind != KindImage {
			return nil, fail("pin %s: name %q must match the file name", e.Name(), p.Name)
		}
		if prev, dup := seen[p.Name]; dup {
			return nil, fail("pin %s: name %q already used by %s", e.Name(), p.Name, prev)
		}
		seen[p.Name] = e.Name()
		pins = append(pins, p)
	}
	if len(pins) == 0 {
		return nil, fail("no pins found in %s", dir)
	}
	sort.Slice(pins, func(i, j int) bool { return pins[i].Name < pins[j].Name })
	return pins, nil
}

// Validate checks one pin against the rules of its kind.
func (p Pin) Validate() error {
	if !nameRe.MatchString(p.Name) {
		return fmt.Errorf("name %q is not lower-case words", p.Name)
	}
	if p.Purpose == "" || p.Owner == "" {
		return fmt.Errorf("%s needs purpose and owner", p.Name)
	}
	if !upstreamTypes[p.Upstream.Type] {
		return fmt.Errorf("%s: unknown upstream type %q", p.Name, p.Upstream.Type)
	}
	switch p.Kind {
	case KindTool:
		return p.validateTool()
	case KindBaseImage:
		if p.Image == "" || p.Version == "" || !digestRe.MatchString(p.Digest) {
			return fmt.Errorf("%s: a base image needs image, version (tag) and a sha256 digest", p.Name)
		}
	case KindImage:
		return p.validateImage()
	default:
		return fmt.Errorf("%s: unknown kind %q", p.Name, p.Kind)
	}
	return nil
}

func (p Pin) validateTool() error {
	if p.Version == "" || len(p.Probe) == 0 || len(p.Sources) == 0 {
		return fmt.Errorf("%s: a tool needs version, probe and at least one source", p.Name)
	}
	seen := map[string]bool{}
	for _, s := range p.Sources {
		key := s.OS + "/" + s.Arch
		switch {
		case !osNames[s.OS] || !archNames[s.Arch]:
			return fmt.Errorf("%s: unknown os/arch %s", p.Name, key)
		case seen[key]:
			return fmt.Errorf("%s: duplicate source for %s", p.Name, key)
		case !strings.HasPrefix(s.URL, "https://"):
			return fmt.Errorf("%s: source %s is not an https URL", p.Name, key)
		case !sha256Re.MatchString(s.SHA256):
			return fmt.Errorf("%s: source %s has no valid sha256", p.Name, key)
		case s.BinarySHA256 != "" && !sha256Re.MatchString(s.BinarySHA256):
			return fmt.Errorf("%s: source %s has an invalid binary_sha256", p.Name, key)
		}
		seen[key] = true
	}
	return nil
}

func (p Pin) validateImage() error {
	if p.Image == "" {
		return fmt.Errorf("%s: the image row needs an image", p.Name)
	}
	switch p.Status {
	case StatusPending:
		if p.Digest != "" {
			return fmt.Errorf("%s: a pending image has no digest", p.Name)
		}
	case StatusPinned:
		if !digestRe.MatchString(p.Digest) {
			return fmt.Errorf("%s: a pinned image needs a sha256 digest", p.Name)
		}
	default:
		return fmt.Errorf("%s: image status must be pending or pinned, got %q", p.Name, p.Status)
	}
	return nil
}
