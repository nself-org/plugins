package inventory

// Purpose: the inventory/v1 contract (P7-AUTO-04): one document of runner entries from a file or from the
//          output of a discovery command, decoded strictly and checked entry by entry.
// Inputs:  raw bytes (YAML or JSON; JSON is YAML).
// Outputs: Document, or a whole-input refusal (unknown field, wrong schema, too many entries); per-entry
//          problems come back from Entry.Check so one bad entry never aborts the others.
// Constraints: unknown fields anywhere refuse the whole document; there is no slots field (slots are
//          agent-side); a label may not use the keys the dedicated fields own (rank, pool.*).

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/plugins/free/ci/internal/nodes/labels"
	"github.com/nself-org/plugins/free/ci/internal/pools"
)

// SchemaID is the contract id carried by every document.
const SchemaID = "inventory/v1"

// Bounds of one document and one entry.
const (
	MaxEntries   = 1000
	MaxRank      = 1000
	MaxSkipRepos = 64
	maxRepoLen   = 256
)

// Entry is one runner of the inventory. Pointer fields tell absent from zero.
type Entry struct {
	ID         string   `yaml:"id" json:"id"`
	SSH        string   `yaml:"ssh" json:"ssh"`
	OS         string   `yaml:"os" json:"os"`
	Arch       string   `yaml:"arch" json:"arch"`
	Rank       *int     `yaml:"rank" json:"rank"`
	Local      bool     `yaml:"local" json:"local"`
	Labels     []string `yaml:"labels" json:"labels"`
	Pools      []string `yaml:"pools" json:"pools"`
	SkipRepos  []string `yaml:"skip_repos" json:"skip_repos"`
	PublicOnly bool     `yaml:"public_only" json:"public_only"`
}

// Document is the inventory/v1 document.
type Document struct {
	Schema  string  `yaml:"schema" json:"schema"`
	Entries []Entry `yaml:"entries" json:"entries"`
}

var (
	idRE   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	sshRE  = regexp.MustCompile(`^([A-Za-z0-9._-]{1,64}@)?[A-Za-z0-9][A-Za-z0-9._-]{0,252}$`)
	osRE   = regexp.MustCompile(`^(linux|darwin|windows)$`)
	archRE = regexp.MustCompile(`^(amd64|arm64)$`)
)

// Decode reads one document strictly: an unknown field, a second document or a wrong schema id refuses it all.
func Decode(raw []byte) (Document, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var d Document
	if err := dec.Decode(&d); err != nil {
		return Document{}, newErr(ReasonDecode, "the inventory is not a valid inventory/v1 document: %s", firstLine(err))
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return Document{}, newErr(ReasonDecode, "the inventory holds more than one document")
	}
	if d.Schema != SchemaID {
		return Document{}, newErr(ReasonSchema, "schema must be %s", SchemaID)
	}
	if len(d.Entries) > MaxEntries {
		return Document{}, newErr(ReasonSchema, "%d entries, at most %d allowed", len(d.Entries), MaxEntries)
	}
	return d, nil
}

func firstLine(err error) string {
	s, _, _ := strings.Cut(err.Error(), "\n")
	return s
}

// Check validates one entry; the error text is the Reject reason.
func (e Entry) Check() error {
	switch {
	case !idRE.MatchString(e.ID):
		return fmt.Errorf("id is not a lower-case node id")
	case e.SSH != "" && !sshRE.MatchString(e.SSH):
		return fmt.Errorf("ssh must be host or user@host")
	case e.OS != "" && !osRE.MatchString(e.OS):
		return fmt.Errorf("os must be linux, darwin or windows")
	case e.Arch != "" && !archRE.MatchString(e.Arch):
		return fmt.Errorf("arch must be amd64 or arm64")
	case e.Rank != nil && (*e.Rank < 0 || *e.Rank > MaxRank):
		return fmt.Errorf("rank must be 0 to %d", MaxRank)
	case len(e.SkipRepos) > MaxSkipRepos:
		return fmt.Errorf("more than %d skip_repos", MaxSkipRepos)
	}
	for _, p := range e.Pools {
		if !pools.ValidID(p) || pools.Reserved(p) {
			return fmt.Errorf("pool %q is not a valid explicit pool id", p)
		}
	}
	for _, r := range e.SkipRepos {
		if r == "" || len(r) > maxRepoLen || strings.ContainsFunc(r, func(c rune) bool { return c < ' ' || c == 0x7f }) {
			return fmt.Errorf("a skip_repos entry is empty, too long or has control characters")
		}
	}
	for _, l := range e.Labels {
		if k, _, _ := strings.Cut(l, "="); k == "rank" || strings.HasPrefix(k, "pool.") {
			return fmt.Errorf("label %s is owned by the rank and pools fields", k)
		}
	}
	want, err := e.Want()
	if err != nil {
		return err
	}
	return labels.Validate(want)
}
