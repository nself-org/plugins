// Package sdkgo exposes the rollout helper for use by nSelf Go applications
// and plugins. It wraps the internal rollout package with a public API.
//
// Usage:
//
//	import sdkgo "github.com/nself-org/nself-feature-flags/sdk-go"
//
//	enabled := sdkgo.IsEnabled(sdkgo.RolloutConfig{
//	    FlagName:          "my-feature",
//	    UserID:            userID,
//	    UserEmail:         userEmail,
//	    RolloutPercentage: 10, // 10%
//	})
package sdkgo

import (
	"github.com/nself-org/nself-feature-flags/internal/rollout"
)

// RolloutConfig is the public API for rollout evaluation.
type RolloutConfig struct {
	// FlagName is the name of the feature flag.
	FlagName string

	// UserID is the stable user identifier for consistent hash routing.
	// Required for percentage-based evaluation.
	UserID string

	// UserEmail is used for internal canary gating (@nself.org domain).
	UserEmail string

	// IsInternal is true when NSELF_INTERNAL=true is present in the request
	// context. Overrides email check for internal audience gating.
	IsInternal bool

	// RolloutPercentage is 0-100. 0 = dark launch, 100 = full rollout.
	RolloutPercentage float64

	// Audience restricts evaluation. Use "internal" for canary audience.
	// Leave empty for percentage-only evaluation.
	Audience string
}

// RolloutResult is the result of a rollout evaluation.
type RolloutResult struct {
	// Enabled is true if the user should see the feature.
	Enabled bool

	// Reason is a human-readable explanation of the decision.
	Reason string
}

// Evaluate performs a staged rollout evaluation for the given configuration.
// It applies audience gating first, then percentage-based evaluation.
//
// This is safe to call on every request. Evaluation is purely in-memory
// (no DB call, no network call). Latency target: <1ms.
func Evaluate(cfg RolloutConfig) RolloutResult {
	r := rollout.Evaluate(rollout.EvalConfig{
		FlagName:          cfg.FlagName,
		UserID:            cfg.UserID,
		UserEmail:         cfg.UserEmail,
		IsInternal:        cfg.IsInternal,
		RolloutPercentage: cfg.RolloutPercentage,
		Audience:          cfg.Audience,
	})
	return RolloutResult{
		Enabled: r.Enabled,
		Reason:  r.Reason,
	}
}

// IsEnabled is a convenience wrapper that returns only the boolean result.
// Use when you don't need the reason string.
func IsEnabled(cfg RolloutConfig) bool {
	return Evaluate(cfg).Enabled
}

// IsInternalUser returns true if the email or context qualifies for the
// internal canary audience (@nself.org or NSELF_INTERNAL=true).
func IsInternalUser(email string, isInternal bool) bool {
	return rollout.IsInternalUser(email, isInternal)
}

// ConsistentHash returns the stable bucket [0, 100) for a given flag name
// and user ID. This is the same algorithm used by sdk-ts/rollout.ts — same
// input always produces the same output across languages and restarts.
func ConsistentHash(flagName, userID string) int {
	return rollout.ConsistentHash(flagName, userID)
}
