// Package rollout implements staged rollout and dark-launch capability
// for the nSelf feature flag system. It provides percentage-based consistent
// hash evaluation and internal-canary audience gating.
//
// Cross-ref: S88d.T02 (staged rollout wiring), S64 (feature flag operator surface).
package rollout

import (
	"hash/fnv"
	"strings"
)

// AudienceInternal is the audience type for internal canary rollouts.
// Users with @nself.org email or NSELF_INTERNAL=true in context see the feature.
const AudienceInternal = "internal"

// InternalDomain is the email domain that qualifies for internal canary audience.
const InternalDomain = "@nself.org"

// EvalConfig holds the configuration for a rollout evaluation.
type EvalConfig struct {
	// FlagName is the name of the feature flag being evaluated.
	FlagName string

	// UserID is the stable identifier for consistent hash routing.
	// Must be non-empty for percentage evaluation.
	UserID string

	// UserEmail is used for internal canary audience gating.
	// Users with @nself.org email qualify as internal.
	UserEmail string

	// IsInternal overrides email check. Set when NSELF_INTERNAL=true
	// is present in the request context.
	IsInternal bool

	// RolloutPercentage is the percentage (0-100) of users who should
	// see the feature. 0 = dark launch, 100 = full rollout.
	RolloutPercentage float64

	// Audience restricts evaluation to a specific audience group.
	// "internal" = @nself.org email or NSELF_INTERNAL env var only.
	// "" = all users (subject to RolloutPercentage).
	Audience string
}

// Result is the outcome of a rollout evaluation.
type Result struct {
	// Enabled indicates whether the user should see the feature.
	Enabled bool

	// Reason explains why the decision was made.
	Reason string
}

// Evaluate determines whether a user should see a feature based on
// the rollout configuration. It applies audience gating first, then
// percentage evaluation.
//
// Evaluation order:
//  1. If Audience == "internal": only @nself.org emails or IsInternal=true qualify.
//  2. If UserID == "": always disabled (cannot hash without stable ID).
//  3. Consistent hash of FlagName:UserID determines percentage bucket.
func Evaluate(cfg EvalConfig) Result {
	// Audience gate: internal canary check first.
	if cfg.Audience == AudienceInternal {
		if !isInternalUser(cfg.UserEmail, cfg.IsInternal) {
			return Result{Enabled: false, Reason: "audience:internal — user not in internal audience"}
		}
		// Internal users bypass percentage: they always see the feature
		// when audience is set to internal.
		return Result{Enabled: true, Reason: "audience:internal — user qualifies as internal"}
	}

	// Percentage gate: requires stable user ID.
	if cfg.UserID == "" {
		return Result{Enabled: false, Reason: "percentage — no user_id provided"}
	}

	bucket := consistentHash(cfg.FlagName, cfg.UserID)
	if float64(bucket) < cfg.RolloutPercentage {
		return Result{
			Enabled: true,
			Reason:  "percentage — user in rollout bucket",
		}
	}

	return Result{
		Enabled: false,
		Reason:  "percentage — user not in rollout bucket",
	}
}

// isInternalUser returns true if the user qualifies for the internal
// canary audience. Qualifications:
//   - Email ends with @nself.org (case-insensitive)
//   - isInternal flag is explicitly true (NSELF_INTERNAL=true context)
func isInternalUser(email string, isInternal bool) bool {
	if isInternal {
		return true
	}
	return strings.HasSuffix(strings.ToLower(email), InternalDomain)
}

// consistentHash returns a stable bucket in [0, 100) for the given
// flag name and user ID. Uses FNV-1a 32-bit hash for speed and good
// distribution. The same input always produces the same bucket across
// restarts, deployments, and language implementations.
//
// Algorithm (matches sdk-ts/rollout.ts for cross-language parity):
//  1. Concatenate: "<flag_name>:<user_id>"
//  2. FNV-1a 32-bit hash of UTF-8 bytes
//  3. Result mod 100
func consistentHash(flagName, userID string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(flagName + ":" + userID))
	return int(h.Sum32() % 100)
}

// IsInternalUser is an exported helper for testing and SDK usage.
// See isInternalUser for qualification rules.
func IsInternalUser(email string, isInternal bool) bool {
	return isInternalUser(email, isInternal)
}

// ConsistentHash is an exported helper for testing and cross-language
// parity verification. Returns [0, 100).
func ConsistentHash(flagName, userID string) int {
	return consistentHash(flagName, userID)
}
