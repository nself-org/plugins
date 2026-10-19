package sbom

// Purpose: produce, collect and validate the SBOMs of a release's artifacts
// (Epic CACHE D13; capability cap:ci.sbom).
// Inputs:  a Runner, the resolved syft path, the Pin, and the Targets.
// Outputs: one Doc per target holding both formats and their digests.
// Constraints: one syft run per target, argv from Argv; a non-zero exit, a
// missing output file or a failed structural check is E718 invalid; nothing is
// written by this package itself (syft writes into OutDir through the Runner),
// the documents are returned so the artifact collector commits them.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Doc is the SBOM pair of one artifact.
type Doc struct {
	Name       string
	CDXPath    string // workdir-relative
	SPDXPath   string
	CDX, SPDX  []byte
	CDXSHA256  string
	SPDXSHA256 string
}

// Generate runs syft for each target and validates the output.
func Generate(ctx context.Context, r Runner, syft string, pin Pin, targets []Target) ([]Doc, error) {
	if len(targets) == 0 {
		return nil, Failf(ReasonInvalid, "no artifacts to inventory")
	}
	seen := map[string]bool{}
	for _, t := range targets {
		if err := t.Check(); err != nil {
			return nil, err
		}
		if seen[t.Name] {
			return nil, Failf(ReasonInvalid, "artifact name %q is used twice", t.Name)
		}
		seen[t.Name] = true
	}
	docs := make([]Doc, 0, len(targets))
	for _, t := range targets {
		d, err := one(ctx, r, syft, pin, t)
		if err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, nil
}

func one(ctx context.Context, r Runner, syft string, pin Pin, t Target) (Doc, error) {
	cmd, err := Argv(syft, t)
	if err != nil {
		return Doc{}, err
	}
	res, err := r.Run(ctx, cmd)
	if err != nil {
		return Doc{}, Failf(ReasonInvalid, "%s: syft did not complete: %v", t.Name, err)
	}
	if res.Exit != 0 {
		return Doc{}, Failf(ReasonInvalid, "%s: syft exited %d: %s", t.Name, res.Exit, excerpt(res.Output))
	}
	cdxPath, spdxPath := Outputs(t.Name)
	cdx, spdx := res.Files[cdxPath], res.Files[spdxPath]
	if len(cdx) == 0 || len(spdx) == 0 {
		return Doc{}, Failf(ReasonInvalid, "%s: syft wrote no %s or %s", t.Name, cdxPath, spdxPath)
	}
	if err := ValidateCycloneDX(cdx, t.Name, pin.Version); err != nil {
		return Doc{}, prefixed(t.Name, err)
	}
	if err := ValidateSPDX(spdx); err != nil {
		return Doc{}, prefixed(t.Name, err)
	}
	return Doc{Name: t.Name, CDXPath: cdxPath, SPDXPath: spdxPath, CDX: cdx, SPDX: spdx, CDXSHA256: digest(cdx), SPDXSHA256: digest(spdx)}, nil
}

func prefixed(name string, err error) error {
	if e, ok := err.(*Error); ok {
		return Failf(e.Reason, "%s: %s", name, e.Msg)
	}
	return err
}

func digest(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func excerpt(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}
