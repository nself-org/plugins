package config

import "github.com/nself-org/plugins/free/ci/internal/model"

// Purpose: the error codes of P7-CI-36 (Epic D14 range E620-E623).
// E620 an invalid `.nself/ci.yaml` (also the warning for a required job whose
// `when.env_present` can exclude it, Epic D28); E621 a leftover `.ci.yaml`
// under V15; E622 a legacy field that cannot become a job; E623 (warning) a
// git-ignored `.nself/ci.yaml`.

// The codes this package raises.
const (
	CodeInvalid        = "E620"
	CodeLeftover       = "E621"
	CodeUntranslatable = "E622"
	CodeIgnored        = "E623"
)

func init() {
	for _, c := range []model.Code{
		{ID: CodeInvalid, Class: "usage", Summary: ".nself/ci.yaml is invalid or risky", Fix: "Fix the named key at the reported file:line and retry"},
		{ID: CodeLeftover, Class: "usage", Summary: "a legacy .ci.yaml is present under V15", Fix: "Run nself ci config migrate --write, then delete the old file"},
		{ID: CodeUntranslatable, Class: "usage", Summary: "a legacy .ci.yaml field cannot become a job", Fix: "Remove or hand-convert the named field"},
		{ID: CodeIgnored, Class: "usage", Summary: ".nself/ci.yaml is git-ignored", Fix: "Add .nself/* and !.nself/ci.yaml to .gitignore"},
	} {
		model.Register(c)
	}
}
