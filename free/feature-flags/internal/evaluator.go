package internal

import (
)

// EvaluationResult is returned by Evaluate and EvaluateBatch.
type EvaluationResult struct {
	FlagKey string      `json:"flag_key"`
	Enabled bool        `json:"enabled"`
	Value   interface{} `json:"value"`
	Variant string      `json:"variant,omitempty"`
	Reason  string      `json:"reason"`
	RuleIdx *int        `json:"rule_index,omitempty"`
}

// EvaluateRequest is the JSON body for POST /v1/evaluate.
type EvaluateRequest struct {
	FlagKey string                 `json:"flag_key"`
	UserID  string                 `json:"user_id,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// BatchEvaluateRequest is the JSON body for POST /v1/evaluate/batch.
type BatchEvaluateRequest struct {
	FlagKeys []string               `json:"flag_keys"`
	UserID   string                 `json:"user_id,omitempty"`
	Context  map[string]interface{} `json:"context,omitempty"`
}

// Rule represents a single targeting rule stored in the flag's rules JSONB array.
type Rule struct {
	Type           string      `json:"type"`
	Enabled        *bool       `json:"enabled,omitempty"`
	Percentage     *float64    `json:"percentage,omitempty"`
	Users          []string    `json:"users,omitempty"`
	SegmentID      *string     `json:"segment_id,omitempty"`
	Attribute      *string     `json:"attribute,omitempty"`
	Operator       *string     `json:"operator,omitempty"`
	AttributeValue interface{} `json:"attribute_value,omitempty"`
	Value          interface{} `json:"value,omitempty"`
	Variant        string      `json:"variant,omitempty"`
	StartAt        *string     `json:"start_at,omitempty"`
	EndAt          *string     `json:"end_at,omitempty"`
}

// Evaluator runs flag evaluation logic against user context.
type Evaluator struct {
	db *DB
}

// NewEvaluator creates a new Evaluator.
