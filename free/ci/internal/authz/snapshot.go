package authz

// Purpose: which committed cache entries may serve as a read-only trusted
// snapshot for lower-trust jobs (R-SEC-18). Only entries written by jobs that
// received no secret, scanned clean, in a project without private
// dependencies.
// Inputs:  EntryFacts (recorded at commit), ProjectFacts.
// Outputs: SnapshotEligible.
// Constraints: pure. Every fact is a positive attestation, so the zero value
// (unknown, missing, never recorded) is not eligible.

// EntryFacts are the facts the store recorded when it committed the entry.
type EntryFacts struct {
	Namespace Namespace
	// NoSecretsReceived: the writer's job spec declared no `secrets:` ref (the
	// conservative P7-CACHE rule: declared counts, resolved or not).
	NoSecretsReceived bool
	// NoPrivateDeps: the entry recorded no authenticated registry or private
	// module setting (private_deps false).
	NoPrivateDeps bool
	// ScannedClean: the ValueScanner found no registered value before commit.
	ScannedClean bool
}

// ProjectFacts are the project-level switches.
type ProjectFacts struct {
	// NoPrivateDeps is false for a project flagged with private dependencies, which
	// switches snapshots off for every entry.
	NoPrivateDeps bool
}

// SnapshotEligible reports whether e may be served in ro-snapshot mode.
func SnapshotEligible(e EntryFacts, p ProjectFacts) bool {
	return IsSnapshotSource(e.Namespace) &&
		e.NoSecretsReceived && e.NoPrivateDeps && e.ScannedClean &&
		p.NoPrivateDeps
}
