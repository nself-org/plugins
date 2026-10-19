package provenance

// Purpose: the in-toto Statement v1 carrying a SLSA provenance v1 predicate
// (Epic CACHE D14, §Contracts "Provenance statement"). This file owns the
// types and the fixed strings; Build and BuildAgent write them, Parse reads
// them, internal/release/verify and internal/agent/verify check them.
// Inputs:  none (types and constants).
// Outputs: Statement and its parts.
// Constraints: the claimed level is SLSA Build L1 and nothing else (D14): the
// one level string lives in the manifest package and is re-exported as Level;
// no code path here can name another one. Unknown additive fields of a parsed
// document are accepted and dropped, as the in-toto and SLSA specs allow.

import "github.com/nself-org/plugins/free/ci/internal/release/manifest"

// Fixed strings of the statement (in-toto Statement v1, SLSA provenance v1).
const (
	StatementType    = "https://in-toto.io/Statement/v1"
	PredicateType    = manifest.ProvenancePredicate
	PayloadType      = "application/vnd.in-toto+json" // the DSSE payloadType of a statement
	BuildTypeRelease = "https://nself.org/ci/buildtypes/release/v1"
	BuildTypeAgent   = "https://nself.org/ci/buildtypes/agent/v1"
	builderPrefix    = "https://nself.org/ci/builder/v1#"
	// Level is the only SLSA build level this fabric claims.
	Level = manifest.SLSALevel
)

// Statement is an in-toto Statement v1 whose predicate is SLSA provenance v1.
type Statement struct {
	Type          string    `json:"_type"`
	Subject       []Subject `json:"subject"`
	PredicateType string    `json:"predicateType"`
	Predicate     Predicate `json:"predicate"`
}

// Subject is one released artifact: its name and sha256.
type Subject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

// Predicate is the SLSA provenance v1 predicate.
type Predicate struct {
	BuildDefinition BuildDefinition `json:"buildDefinition"`
	RunDetails      RunDetails      `json:"runDetails"`
}

// BuildDefinition says what was built from what.
type BuildDefinition struct {
	BuildType            string               `json:"buildType"`
	ExternalParameters   map[string]string    `json:"externalParameters"`
	InternalParameters   map[string]string    `json:"internalParameters"`
	ResolvedDependencies []ResourceDescriptor `json:"resolvedDependencies"`
}

// ResourceDescriptor is the in-toto resource descriptor, reduced to the fields
// this fabric writes.
type ResourceDescriptor struct {
	URI    string            `json:"uri,omitempty"`
	Name   string            `json:"name,omitempty"`
	Digest map[string]string `json:"digest"`
}

// RunDetails says who built it and what else came out.
type RunDetails struct {
	Builder    Builder              `json:"builder"`
	Metadata   Metadata             `json:"metadata"`
	Byproducts []ResourceDescriptor `json:"byproducts"`
}

// Builder identifies the builder; for a public repository the id is
// pseudonymous (P7-TRUST-11 publication profile).
type Builder struct {
	ID string `json:"id"`
}

// Metadata is the invocation record. The times are whole seconds in UTC and
// may be empty for a build that does not record them.
type Metadata struct {
	InvocationID string `json:"invocationId"`
	StartedOn    string `json:"startedOn,omitempty"`
	FinishedOn   string `json:"finishedOn,omitempty"`
}

// SubjectDigests maps each subject name to its sha256.
func (s Statement) SubjectDigests() map[string]string {
	out := make(map[string]string, len(s.Subject))
	for _, sub := range s.Subject {
		out[sub.Name] = sub.Digest["sha256"]
	}
	return out
}

// Byproduct returns the sha256 of the named byproduct.
func (s Statement) Byproduct(name string) (string, bool) {
	for _, b := range s.Predicate.RunDetails.Byproducts {
		if b.Name == name {
			d, ok := b.Digest["sha256"]
			return d, ok
		}
	}
	return "", false
}

// Dependency returns the sha256 of the named resolved dependency (a lockfile).
func (s Statement) Dependency(name string) (string, bool) {
	for _, d := range s.Predicate.BuildDefinition.ResolvedDependencies {
		if d.Name == name {
			v, ok := d.Digest["sha256"]
			return v, ok
		}
	}
	return "", false
}

// GitCommit returns the gitCommit digest of the source dependency.
func (s Statement) GitCommit() string {
	for _, d := range s.Predicate.BuildDefinition.ResolvedDependencies {
		if c, ok := d.Digest["gitCommit"]; ok {
			return c
		}
	}
	return ""
}
