package gates

import "github.com/nself-org/plugins/free/ci/internal/model"

func init() {
	model.Register(model.Code{ID: "E614", Class: "infra", Summary: "required check tool is unavailable", Fix: "Install the named tool and retry"})
	model.Register(model.Code{ID: "E615", Class: "code", Summary: "check configuration or threshold is invalid", Fix: "Correct the check configuration and retry"})
}
