package lease

import "github.com/nself-org/plugins/free/ci/internal/model"

func init() {
	model.Register(model.Code{ID: "E662", Class: "infra", Summary: "stale lease epoch", Fix: "Stop the job and clean its workspace before retrying"})
}
