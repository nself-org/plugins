package internal

import (
	"encoding/json"
	"log"
)

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
