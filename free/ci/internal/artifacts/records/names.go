package records

// Purpose: list the files declarations select and the record name each will
// get, before anything is stored (the release SBOM step names its documents
// after the records, which the publication looks up by name).
// Inputs:  the workdir root and normalized declarations.
// Outputs: one Named per selected file.
// Constraints: read only; uses the same expansion and naming as the collector,
// so a name here is the name the collector commits; a declaration that selects
// nothing is a refusal, as it is for the collector.

import (
	"context"
	"os"
)

// Named is a file a declaration selects.
type Named struct {
	Rel  string // workdir-relative path
	Name string // the record name
	Decl Decl
}

// Names expands decls inside root.
func Names(ctx context.Context, root *os.Root, decls []Decl) ([]Named, error) {
	var out []Named
	for _, d := range decls {
		ents, err := expand(ctx, root.FS(), d.Path, DefaultMaxRecords-len(out))
		if err != nil {
			return nil, err
		}
		if len(ents) == 0 {
			return nil, failf(ReasonMissing, "declared artifact %q matched no file in the workdir", d.Path)
		}
		literal := !hasMeta(d.Path) && len(ents) == 1 && ents[0].rel == d.Path
		for _, f := range ents {
			out = append(out, Named{Rel: f.rel, Name: recordName(d, f.rel, literal), Decl: d})
		}
	}
	return out, nil
}
