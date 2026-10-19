package provenance

// Purpose: Parse, the one reader of a provenance statement payload (the bytes
// inside the DSSE envelope, never the envelope).
// Inputs:  payload bytes. Outputs: a validated Statement.
// Constraints: bounded (size, depth); a repeated key is refused (CheckStrict);
// the in-toto type and the SLSA predicate type must be the v1 values; every
// subject carries a unique name and a lowercase sha256; the build definition
// and the run details carry what a verifier compares; unknown additive fields
// are accepted and dropped. Pure.

import (
	"bytes"
	"encoding/json"
)

const maxStatement = 4 << 20

// Parse decodes and validates a statement payload.
func Parse(payload []byte) (Statement, error) {
	var st Statement
	if len(payload) > maxStatement {
		return st, invalid("statement is larger than %d bytes", maxStatement)
	}
	if err := CheckStrict(payload); err != nil {
		return st, err
	}
	if err := json.NewDecoder(bytes.NewReader(payload)).Decode(&st); err != nil {
		return Statement{}, invalid("%s", err)
	}
	if err := st.validate(); err != nil {
		return Statement{}, err
	}
	return st, nil
}

func (s Statement) validate() error {
	if s.Type != StatementType || s.PredicateType != PredicateType {
		return invalid("_type or predicateType is not the v1 value")
	}
	if len(s.Subject) == 0 {
		return invalid("subject is empty")
	}
	seen := make(map[string]bool, len(s.Subject))
	for _, sub := range s.Subject {
		if err := text("subject name", sub.Name); err != nil {
			return err
		}
		if seen[sub.Name] || !hexRe.MatchString(sub.Digest["sha256"]) {
			return invalid("subject %q repeats or has no sha256", sub.Name)
		}
		seen[sub.Name] = true
	}
	bd, rd := s.Predicate.BuildDefinition, s.Predicate.RunDetails
	if bd.BuildType == "" || bd.ExternalParameters == nil || rd.Builder.ID == "" || rd.Metadata.InvocationID == "" {
		return invalid("the predicate lacks a build type, external parameters, builder id or invocation id")
	}
	for _, d := range append(append([]ResourceDescriptor{}, bd.ResolvedDependencies...), rd.Byproducts...) {
		if len(d.Digest) == 0 {
			return invalid("a dependency or byproduct has no digest")
		}
	}
	return nil
}
