// Purpose: the --json rendering of `nself-k8s values`: the data member and the
// parity-failure envelope.
//
// Inputs: a *values.Result and the error from values.Run.
//
// Outputs: the data map; an error envelope (E723 for a parity failure, with
// the differences in cause) via the invocation.
//
// Constraints: Result carries no secret value; differences name services and
// image refs only (values.Check never includes env values).
package main

import (
	"errors"
	"strings"

	"github.com/nself-org/nself-k8s/internal/values"
)

// valuesData is the data member for a successful run.
func valuesData(res *values.Result) map[string]any {
	if res.Mode == values.ModeCheck {
		return map[string]any{
			"mode":        res.Mode,
			"values_file": res.ValuesFile,
			"services":    res.Services,
			"parity":      "ok",
		}
	}
	return map[string]any{
		"mode":            res.Mode,
		"values_file":     res.ValuesFile,
		"secrets_file":    res.SecretsFile,
		"mapped":          res.Mapped,
		"unsupported":     values.Items(res.Unsupported),
		"unmapped_routes": values.Items(res.UnmappedRoutes),
	}
}

// valuesFail writes the error envelope for a failed values run.
func (r *invocation) valuesFail(res *values.Result, err error) error {
	d := classify(err)
	var ve *values.Error
	if res != nil && errors.As(err, &ve) && ve.Kind == values.KindParity {
		lines := make([]string, len(res.Diffs))
		for i, df := range res.Diffs {
			lines[i] = df.String()
		}
		d.Cause = strings.Join(lines, "; ")
	}
	return r.emit(d)
}
