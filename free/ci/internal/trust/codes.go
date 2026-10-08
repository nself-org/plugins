package trust

import "github.com/nself-org/plugins/free/ci/internal/model"

func init() {
	for _, code := range []model.Code{
		{ID: "E670", Class: "usage", Summary: "policy scope loosens a parent", Fix: "Tighten the lower scope or edit the operator policy"},
		{ID: "E671", Class: "usage", Summary: "revision or branch policy field ignored", Fix: "Move spend authorization to operator policy or tighten the revision"},
		{ID: "E672", Class: "usage", Summary: "invalid policy key or value", Fix: "Use a registered key with a valid value"},
	} {
		model.Register(code)
	}
}
