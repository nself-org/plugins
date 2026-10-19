package provenance

// Purpose: Build, the producer of the SLSA provenance v1 statement of a
// release (Epic CACHE D14, §Contracts). It writes the statement bytes the
// signer wraps in DSSE and the verifier compares with the manifest.
// Inputs:  ReleaseFacts, taken from the evidence and the release inputs.
// Outputs: canonical statement bytes (UTF-8 JSON, object keys sorted, no
// insignificant whitespace; the same facts always give the same bytes).
// Constraints: pure. For a repository that is not known to be private, the
// builder id is pseudonymous through attest.ProfileFor and Build refuses
// facts that carry a private address, host name, user name or home path
// (R-SEC-27); it never rewrites a name, because a subject must stay equal to
// its manifest artifact. Subjects, dependencies and byproducts are sorted.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/nself-org/plugins/free/ci/internal/attest"
)

// FileDigest names a file and its sha256 (lowercase hex).
type FileDigest struct{ Name, SHA256 string }

// ImageDigest is the toolchain image a containerized job ran in.
type ImageDigest struct{ Ref, SHA256 string }

// ReleaseFacts are the facts of one release (Epic §Contracts).
type ReleaseFacts struct {
	Visibility attest.Visibility
	Hostnames  []string // extra literals the publication profile must keep out
	Usernames  []string

	RepoID, Revision, Pipeline, Job, ConfigDigest string
	Isolation, NselfCIVersion                     string
	Lockfiles                                     []FileDigest // path and digest at Revision
	Image                                         *ImageDigest // nil unless containerized

	Coordinator, Runner   string // raw ids; the profile pseudonymizes them
	RunID, AttemptID      string
	StartedOn, FinishedOn time.Time

	Subjects []FileDigest // every release artifact: name and sha256
	Evidence string       // sha256 of evidence.json
	SBOMs    []FileDigest // "sbom/<name>.cdx.json" and ".spdx.json" with digests
}

var (
	hexRe      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	revisionRe = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
)

// text checks a free-text fact: present, bounded, valid UTF-8, no control
// character.
func text(field, v string) error {
	switch {
	case v == "":
		return invalid("%s is empty", field)
	case utf8.RuneCountInString(v) > 512 || !utf8.ValidString(v):
		return invalid("%s is too long or not valid UTF-8", field)
	case strings.ContainsFunc(v, unicode.IsControl):
		return invalid("%s holds a control character", field)
	}
	return nil
}

func files(field string, list []FileDigest, min int) ([]FileDigest, error) {
	if len(list) < min {
		return nil, invalid("%s list is empty", field)
	}
	out := append([]FileDigest(nil), list...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	for i, f := range out {
		if err := text(field+" name", f.Name); err != nil {
			return nil, err
		}
		if !hexRe.MatchString(f.SHA256) {
			return nil, invalid("%s %q has no sha256", field, f.Name)
		}
		if i > 0 && out[i-1].Name == f.Name {
			return nil, invalid("%s %q repeats", field, f.Name)
		}
	}
	return out, nil
}

func rfc3339(field string, t time.Time) (string, error) {
	if t.IsZero() {
		return "", invalid("%s is not set", field)
	}
	return t.UTC().Truncate(time.Second).Format("2006-01-02T15:04:05Z"), nil
}

func descriptors(list []FileDigest) []ResourceDescriptor {
	out := make([]ResourceDescriptor, 0, len(list))
	for _, f := range list {
		out = append(out, ResourceDescriptor{Name: f.Name, Digest: map[string]string{"sha256": f.SHA256}})
	}
	return out
}

// Build returns the canonical statement bytes of a release.
func Build(f ReleaseFacts) ([]byte, error) {
	for _, c := range []struct{ field, v string }{{"repo id", f.RepoID}, {"pipeline", f.Pipeline}, {"job", f.Job},
		{"isolation", f.Isolation}, {"nself-ci version", f.NselfCIVersion}, {"coordinator", f.Coordinator}, {"runner", f.Runner},
		{"run id", f.RunID}, {"attempt id", f.AttemptID}} {
		if err := text(c.field, c.v); err != nil {
			return nil, err
		}
	}
	if !revisionRe.MatchString(f.Revision) || !hexRe.MatchString(f.ConfigDigest) || !hexRe.MatchString(f.Evidence) {
		return nil, invalid("revision, config digest or evidence digest is not lowercase hex")
	}
	subjects, err := files("subject", f.Subjects, 1)
	if err != nil {
		return nil, err
	}
	locks, err := files("lockfile", f.Lockfiles, 0)
	if err != nil {
		return nil, err
	}
	sboms, err := files("sbom", f.SBOMs, 0)
	if err != nil {
		return nil, err
	}
	started, err := rfc3339("startedOn", f.StartedOn)
	if err != nil {
		return nil, err
	}
	finished, err := rfc3339("finishedOn", f.FinishedOn)
	if err != nil {
		return nil, err
	}
	if f.FinishedOn.Before(f.StartedOn) {
		return nil, invalid("finishedOn is before startedOn")
	}
	prof := attest.ProfileFor(attest.RepoFacts{Visibility: f.Visibility, RepoID: f.RepoID, Hostnames: f.Hostnames, Usernames: f.Usernames})
	if err = f.guard(prof, locks, subjects, sboms); err != nil {
		return nil, err
	}
	deps := []ResourceDescriptor{{URI: "git+" + f.RepoID + "@" + f.Revision, Digest: map[string]string{"gitCommit": f.Revision}}}
	deps = append(deps, descriptors(locks)...)
	if f.Image != nil {
		if !hexRe.MatchString(f.Image.SHA256) {
			return nil, invalid("toolchain image has no sha256")
		}
		deps = append(deps, ResourceDescriptor{URI: f.Image.Ref, Digest: map[string]string{"sha256": f.Image.SHA256}})
	}
	by := append([]FileDigest{{Name: "evidence.json", SHA256: f.Evidence}}, sboms...)
	sort.Slice(by, func(i, j int) bool { return by[i].Name < by[j].Name })
	subs := make([]Subject, 0, len(subjects))
	for _, s := range subjects {
		subs = append(subs, Subject{Name: s.Name, Digest: map[string]string{"sha256": s.SHA256}})
	}
	return encode(Statement{Type: StatementType, Subject: subs, PredicateType: PredicateType, Predicate: Predicate{
		BuildDefinition: BuildDefinition{BuildType: BuildTypeRelease,
			ExternalParameters: map[string]string{"repository": f.RepoID, "revision": f.Revision, "pipeline": f.Pipeline, "job": f.Job, "config_digest": f.ConfigDigest},
			InternalParameters: map[string]string{"isolation": f.Isolation, "nself_ci_version": f.NselfCIVersion}, ResolvedDependencies: deps},
		RunDetails: RunDetails{
			Builder:    Builder{ID: builderPrefix + prof.CoordinatorID(f.Coordinator) + "/" + prof.RunnerID(f.Runner)},
			Metadata:   Metadata{InvocationID: f.RunID + "/" + f.AttemptID, StartedOn: started, FinishedOn: finished},
			Byproducts: descriptors(by)},
	}})
}

// guard refuses facts the publication profile would redact: for a public
// repository nothing private may reach the statement.
func (f ReleaseFacts) guard(prof attest.Profile, locks, subjects, sboms []FileDigest) error {
	values := []string{f.RepoID, f.Pipeline, f.Job, f.Isolation, f.NselfCIVersion, f.RunID, f.AttemptID}
	for _, list := range [][]FileDigest{locks, subjects, sboms} {
		values = append(values, names(list)...)
	}
	if f.Image != nil {
		values = append(values, f.Image.Ref)
	}
	return keepPublic(prof, values...)
}

func names(list []FileDigest) []string {
	out := make([]string, 0, len(list))
	for _, f := range list {
		out = append(out, f.Name)
	}
	return out
}

// keepPublic fails when a public profile would redact any of values.
func keepPublic(prof attest.Profile, values ...string) error {
	if !prof.Public() {
		return nil
	}
	for _, v := range values {
		if prof.Text(v) != v {
			return invalid("a fact for a public repository names a private host, address, user or path")
		}
	}
	return nil
}

// encode writes v with sorted object keys and no insignificant whitespace.
func encode(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var tree any
	if err = dec.Decode(&tree); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err = enc.Encode(tree); err != nil {
		return nil, fmt.Errorf("provenance: encode: %w", err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
