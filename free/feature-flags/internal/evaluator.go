package internal

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"regexp"
	"time"
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
func NewEvaluator(db *DB) *Evaluator {
	return &Evaluator{db: db}
}

// Evaluate evaluates a single flag for the given user context.
func (e *Evaluator) Evaluate(flagKey, userID string, ctx map[string]interface{}) EvaluationResult {
	flag, err := e.db.GetFlag(flagKey)
	if err != nil {
		log.Printf("feature-flags: evaluate error for %s: %v", flagKey, err)
		return EvaluationResult{
			FlagKey: flagKey,
			Enabled: false,
			Value:   false,
			Reason:  "error",
		}
	}
	if flag == nil {
		return EvaluationResult{
			FlagKey: flagKey,
			Enabled: false,
			Value:   false,
			Reason:  "not_found",
		}
	}

	if !flag.Enabled {
		val := parseJSONValue(flag.DefaultValue)
		return EvaluationResult{
			FlagKey: flagKey,
			Enabled: false,
			Value:   val,
			Reason:  "disabled",
		}
	}

	// Parse rules from JSONB
	var rules []Rule
	if err := json.Unmarshal(flag.Rules, &rules); err != nil {
		log.Printf("feature-flags: failed to parse rules for %s: %v", flagKey, err)
		val := parseJSONValue(flag.DefaultValue)
		return EvaluationResult{
			FlagKey: flagKey,
			Enabled: true,
			Value:   val,
			Reason:  "default",
		}
	}

	// Evaluate rules in order (priority is array order)
	for i, rule := range rules {
		if rule.Enabled != nil && !*rule.Enabled {
			continue
		}
		if e.matchRule(rule, flagKey, userID, ctx) {
			val := rule.Value
			if val == nil {
				val = true
			}
			idx := i
			return EvaluationResult{
				FlagKey: flagKey,
				Enabled: true,
				Value:   val,
				Variant: rule.Variant,
				Reason:  "rule_match",
				RuleIdx: &idx,
			}
		}
	}

	// No rules matched: return default value
	val := parseJSONValue(flag.DefaultValue)
	return EvaluationResult{
		FlagKey: flagKey,
		Enabled: true,
		Value:   val,
		Reason:  "default",
	}
}

// EvaluateBatch evaluates multiple flags at once.
func (e *Evaluator) EvaluateBatch(flagKeys []string, userID string, ctx map[string]interface{}) []EvaluationResult {
	results := make([]EvaluationResult, 0, len(flagKeys))
	for _, key := range flagKeys {
		results = append(results, e.Evaluate(key, userID, ctx))
	}
	return results
}

// matchRule checks if a single rule matches the given context.
func (e *Evaluator) matchRule(rule Rule, flagKey, userID string, ctx map[string]interface{}) bool {
	switch rule.Type {
	case "percentage":
		return e.matchPercentage(rule, flagKey, userID)
	case "user_list":
		return e.matchUserList(rule, userID)
	case "segment":
		return e.matchSegment(rule, ctx)
	case "attribute":
		return e.matchAttribute(rule, ctx)
	case "schedule":
		return e.matchSchedule(rule)
	default:
		log.Printf("feature-flags: unknown rule type %q", rule.Type)
		return false
	}
}

// matchPercentage uses a consistent hash to determine if user falls within rollout percentage.
func (e *Evaluator) matchPercentage(rule Rule, flagKey, userID string) bool {
	if userID == "" || rule.Percentage == nil {
		return false
	}
	h := hashPercentage(flagKey, userID)
	return float64(h) < *rule.Percentage
}

// matchUserList checks if the user ID is in the targeting list.
func (e *Evaluator) matchUserList(rule Rule, userID string) bool {
	if userID == "" || len(rule.Users) == 0 {
		return false
	}
	for _, u := range rule.Users {
		if u == userID {
			return true
		}
	}
	return false
}

// matchSegment loads the segment from DB and evaluates its rules against the context.
func (e *Evaluator) matchSegment(rule Rule, ctx map[string]interface{}) bool {
	if rule.SegmentID == nil {
		return false
	}
	seg, err := e.db.GetSegment(*rule.SegmentID)
	if err != nil || seg == nil {
		return false
	}

	// Parse segment rules
	var segRules []SegmentRule
	if err := json.Unmarshal(seg.Rules, &segRules); err != nil {
		log.Printf("feature-flags: failed to parse segment rules: %v", err)
		return false
	}
	if len(segRules) == 0 {
		return false
	}

	// All segment rules must match (AND logic)
	for _, sr := range segRules {
		if !evaluateAttributeOp(sr.Attribute, sr.Operator, sr.Value, ctx) {
			return false
		}
	}
	return true
}

// matchAttribute evaluates a single attribute condition.
func (e *Evaluator) matchAttribute(rule Rule, ctx map[string]interface{}) bool {
	if rule.Attribute == nil || rule.Operator == nil {
		return false
	}
	return evaluateAttributeOp(*rule.Attribute, *rule.Operator, rule.AttributeValue, ctx)
}

// matchSchedule checks if the current time falls within start_at/end_at window.
func (e *Evaluator) matchSchedule(rule Rule) bool {
	if rule.StartAt == nil && rule.EndAt == nil {
		return false
	}
	now := time.Now()
	if rule.StartAt != nil {
		t, err := time.Parse(time.RFC3339, *rule.StartAt)
		if err != nil {
			return false
		}
		if now.Before(t) {
			return false
		}
	}
	if rule.EndAt != nil {
		t, err := time.Parse(time.RFC3339, *rule.EndAt)
		if err != nil {
			return false
		}
		if now.After(t) {
			return false
		}
	}
	return true
}

// SegmentRule is a single condition within a segment's rules JSONB.
type SegmentRule struct {
	Attribute string      `json:"attribute"`
	Operator  string      `json:"operator"`
	Value     interface{} `json:"value"`
}

// evaluateAttributeOp compares a context attribute against a value using the given operator.
func evaluateAttributeOp(attr, op string, target interface{}, ctx map[string]interface{}) bool {
	val, ok := ctx[attr]
	if !ok {
		return false
	}

	switch op {
	case "eq":
		return fmt.Sprintf("%v", val) == fmt.Sprintf("%v", target)
	case "neq":
		return fmt.Sprintf("%v", val) != fmt.Sprintf("%v", target)
	case "gt":
		return toFloat(val) > toFloat(target)
	case "lt":
		return toFloat(val) < toFloat(target)
	case "gte":
		return toFloat(val) >= toFloat(target)
	case "lte":
		return toFloat(val) <= toFloat(target)
	case "contains":
		sv, okS := val.(string)
		tv, okT := target.(string)
		return okS && okT && containsStr(sv, tv)
	case "regex":
		sv, okS := val.(string)
		tv, okT := target.(string)
		if !okS || !okT {
			return false
		}
		re, err := regexp.Compile(tv)
		if err != nil {
			log.Printf("feature-flags: invalid regex %q: %v", tv, err)
			return false
		}
		return re.MatchString(sv)
	default:
		return false
	}
}

// hashPercentage produces a consistent 0-99 value for percentage rollouts.
func hashPercentage(flagKey, userID string) int {
	s := flagKey + ":" + userID
	var h int32
	for _, c := range s {
		h = ((h << 5) - h) + int32(c)
	}
	if h < 0 {
		h = -h
	}
	return int(h) % 100
}

// toFloat attempts to convert an interface{} to float64.
func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return math.NaN()
	}
}

// containsStr checks if s contains substr.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// parseJSONValue decodes a json.RawMessage into a native Go value.
func parseJSONValue(raw json.RawMessage) interface{} {
	if len(raw) == 0 {
		return false
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}

