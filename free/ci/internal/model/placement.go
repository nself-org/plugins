package model

// PlacementPlan is the experimental ci.placement-plan/v1 document.
type PlacementPlan struct {
	Schema   string               `json:"schema"`
	Pipeline PlacementPipelineRef `json:"pipeline"`
	FastPath FastPath             `json:"fast_path"`
	Jobs     []JobPlacement       `json:"jobs"`
	Totals   Totals               `json:"totals"`
}
type PlacementPipelineRef struct {
	ID           *string `json:"id"`
	Name         string  `json:"name"`
	Revision     string  `json:"revision"`
	Trigger      string  `json:"trigger"`
	PolicyDigest string  `json:"policy_digest"`
	Preset       string  `json:"preset"`
	WorldDigest  string  `json:"world_digest"`
}
type FastPath struct {
	Taken  bool   `json:"taken"`
	Reason string `json:"reason"`
}
type PlacementReasonDetail struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}
type PlacementRequirements struct {
	Platform     string       `json:"platform"`
	IsolationMin Isolation    `json:"isolation_min"`
	Network      NetworkScope `json:"network"`
	Labels       []string     `json:"labels"`
	CPU          int64        `json:"cpu"`
	MemMB        int64        `json:"mem_mb"`
	Tools        []string     `json:"tools"`
}
type JobPlacement struct {
	ID             string                  `json:"id"`
	Name           string                  `json:"name"`
	MatrixKey      string                  `json:"matrix_key"`
	Deps           []string                `json:"deps"`
	Band           string                  `json:"band"`
	Requirements   PlacementRequirements   `json:"requirements"`
	SecretClasses  []SecretClass           `json:"secret_classes"`
	Status         string                  `json:"status"`
	Reasons        []PlacementReasonDetail `json:"reasons"`
	Placement      *Placement              `json:"placement"`
	Alternatives   []Alternative           `json:"alternatives"`
	ExpectedWaitMs *int64                  `json:"expected_wait_ms"`
}
type Placement struct {
	RunnerID   string               `json:"runner_id"`
	Provider   string               `json:"provider"`
	Location   string               `json:"location"`
	CostClass  string               `json:"cost_class"`
	Score      int64                `json:"score"`
	Components map[string]Component `json:"components"`
	Estimate   Estimate             `json:"estimate"`
}
type Component struct {
	ValueMs        int64  `json:"value_ms"`
	WeightPermille int64  `json:"weight_permille"`
	Confidence     string `json:"confidence"`
}
type Estimate struct {
	WaitMs         *int64 `json:"wait_ms"`
	RuntimeMs      *int64 `json:"runtime_ms"`
	CostMicro      *int64 `json:"cost_micro"`
	CostConfidence string `json:"cost_confidence"`
}
type Alternative struct {
	RunnerID   string                  `json:"runner_id"`
	Provider   string                  `json:"provider"`
	Eligible   bool                    `json:"eligible"`
	Reasons    []PlacementReasonDetail `json:"reasons"`
	Score      *int64                  `json:"score"`
	Components map[string]Component    `json:"components"`
}
type Totals struct {
	EstimatedCostMicro int64  `json:"estimated_cost_micro"`
	CostConfidence     string `json:"cost_confidence"`
	Placed             int    `json:"placed"`
	Waiting            int    `json:"waiting"`
	AwaitingApproval   int    `json:"awaiting_approval"`
	Unplaceable        int    `json:"unplaceable"`
}
