package sched

import "github.com/nself-org/plugins/free/ci/internal/model"

func init() {
	model.Register(model.Code{ID: "E690", Class: "usage", Summary: "matrix entry is outside pipeline support", Fix: "Declare the platform in support or remove the matrix entry"})
}
