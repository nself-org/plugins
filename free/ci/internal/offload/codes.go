package offload

import "github.com/nself-org/plugins/free/ci/internal/model"

func init() {
	model.Register(model.Code{ID: CodeTracked, Class: "usage", Summary: "a tracked file is secret-shaped or over the size cap", Fix: "Untrack the file, or run the gate on a clean checkout"})
}
