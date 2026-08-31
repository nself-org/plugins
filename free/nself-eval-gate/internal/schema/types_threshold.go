package schema

import "time"

// EvalThreshold represents a row in np_eval_thresholds.
// Purpose: Global autonomy-tier gate configuration; NOT per-tenant (no source_account_id).
// Inputs: read by gate.go to determine if a tier is cleared.
// Outputs: returned by GET /eval/thresholds and used in IsTierCleared checks.
// Constraints: AppliesTo lists suite slugs that must all pass for the tier to be cleared.
type EvalThreshold struct {
	ID             string    `json:"id" db:"id"`
	AutononyTier   string    `json:"autonomy_tier" db:"autonomy_tier"`
	MinPassRate    float64   `json:"min_pass_rate" db:"min_pass_rate"`
	MinSuiteScore  float64   `json:"min_suite_score" db:"min_suite_score"`
	AppliesTo      []string  `json:"applies_to" db:"applies_to"`
	Enforced       bool      `json:"enforced" db:"enforced"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}
