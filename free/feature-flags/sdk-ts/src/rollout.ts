/**
 * rollout.ts — Staged rollout and dark-launch helper for the nSelf
 * feature-flags TypeScript SDK.
 *
 * Cross-language parity: this module implements the SAME algorithm as
 * plugins/free/feature-flags/internal/rollout/rollout.go. Same inputs
 * MUST produce the same outputs.
 *
 * Algorithm: FNV-1a 32-bit hash of "<flag_name>:<user_id>", result mod 100.
 *
 * @see S88d.T02 — staged rollout wiring
 * @see S64 — feature flag operator surface
 */

/** Audience type for internal canary rollouts. */
export const AUDIENCE_INTERNAL = 'internal'

/** Email domain suffix that qualifies for internal canary. */
const INTERNAL_DOMAIN = '@nself.org'

/** FNV-1a 32-bit constants. Must match Go implementation. */
const FNV1A_OFFSET_BASIS = 2166136261
const FNV1A_PRIME = 16777619
const UINT32_MOD = 0x100000000 // 2^32 — used to keep arithmetic in uint32 range

/** Configuration for a rollout evaluation. */
export interface RolloutConfig {
  /** Name of the feature flag being evaluated. */
  flagName: string

  /**
   * Stable user identifier for consistent hash routing.
   * Required for percentage-based evaluation.
   * Never sent to the flag service in plaintext — only the hash result is used.
   */
  userId: string

  /** User's email address. Used for @nself.org internal canary gating. */
  userEmail?: string

  /**
   * True when NSELF_INTERNAL=true is present in the request context.
   * Overrides email check for internal audience qualification.
   */
  isInternal?: boolean

  /**
   * Percentage of users who should see the feature (0–100).
   * 0 = dark launch (nobody), 100 = full rollout (everyone).
   */
  rolloutPercentage: number

  /**
   * Restricts evaluation to a named audience.
   * Use AUDIENCE_INTERNAL ("internal") for canary gating.
   * Leave undefined for percentage-only evaluation.
   */
  audience?: string
}

/** Result of a rollout evaluation. */
export interface RolloutResult {
  /** Whether the user should see the feature. */
  enabled: boolean

  /** Human-readable explanation of the decision. */
  reason: string
}

/**
 * Returns true if the user qualifies for the internal canary audience.
 * Qualifications:
 * - Email ends with @nself.org (case-insensitive)
 * - isInternal flag is explicitly true (NSELF_INTERNAL=true context)
 */
export function isInternalUser(email: string | undefined, isInternal?: boolean): boolean {
  if (isInternal === true) return true
  if (!email) return false
  return email.toLowerCase().endsWith(INTERNAL_DOMAIN)
}

/**
 * Computes a consistent hash bucket in [0, 100) for the given flag name
 * and user ID. Uses FNV-1a 32-bit algorithm.
 *
 * This is IDENTICAL to Go's rollout.ConsistentHash() — same flag name
 * and user ID produce the same result in both languages.
 *
 * The user ID is combined with the flag name before hashing so that the
 * same user gets DIFFERENT buckets for DIFFERENT flags (preventing correlated
 * rollout across features for the same users).
 */
export function consistentHash(flagName: string, userId: string): number {
  const input = `${flagName}:${userId}`

  // FNV-1a 32-bit, matching Go's hash/fnv.New32a()
  let hash = FNV1A_OFFSET_BASIS
  for (let i = 0; i < input.length; i++) {
    // XOR with byte value
    hash ^= input.charCodeAt(i)
    // Multiply by FNV prime, keep in uint32 range
    hash = Math.imul(hash, FNV1A_PRIME)
    // Force unsigned 32-bit to match Go uint32 behavior
    hash = (hash >>> 0)
  }

  // Result in [0, 100)
  return hash % 100
}

/**
 * Evaluates whether a user should see a feature based on the rollout config.
 *
 * Evaluation order:
 * 1. If audience == "internal": only @nself.org emails or isInternal=true qualify.
 * 2. If userId == "": always disabled (cannot hash without stable ID).
 * 3. Consistent hash of "flagName:userId" determines percentage bucket.
 *
 * This function runs entirely in-memory (<1ms). It does NOT make network calls.
 */
export function evaluate(cfg: RolloutConfig): RolloutResult {
  // Audience gate: internal canary check first.
  if (cfg.audience === AUDIENCE_INTERNAL) {
    if (!isInternalUser(cfg.userEmail, cfg.isInternal)) {
      return {
        enabled: false,
        reason: 'audience:internal — user not in internal audience',
      }
    }
    // Internal users bypass percentage: they always see the feature.
    return {
      enabled: true,
      reason: 'audience:internal — user qualifies as internal',
    }
  }

  // Percentage gate: requires stable user ID.
  if (!cfg.userId) {
    return {
      enabled: false,
      reason: 'percentage — no user_id provided',
    }
  }

  const bucket = consistentHash(cfg.flagName, cfg.userId)
  if (bucket < cfg.rolloutPercentage) {
    return {
      enabled: true,
      reason: 'percentage — user in rollout bucket',
    }
  }

  return {
    enabled: false,
    reason: 'percentage — user not in rollout bucket',
  }
}

/**
 * Convenience wrapper that returns only the boolean result.
 * Use when you don't need the reason string.
 */
export function isEnabled(cfg: RolloutConfig): boolean {
  return evaluate(cfg).enabled
}

/**
 * All seven UI states for a flag-gated feature.
 * Components should handle all states to avoid flicker or blank screens.
 */
export type FlagUIState =
  | 'loading'          // Flag resolution in flight (show skeleton)
  | 'empty'            // Feature disabled (flag evaluates false)
  | 'error'            // Flag service unreachable → fail-open (use cached)
  | 'populated'        // Feature enabled (flag evaluates true)
  | 'offline'          // No flag service → use cached value
  | 'permission-denied' // User not in audience (not internal)
  | 'rate-limited'     // Flag service throttled → use cached value

/**
 * Resolves the UI state for a flag-gated component based on the
 * evaluation context.
 */
export function resolveFlagUIState(params: {
  isLoading: boolean
  isError: boolean
  isOffline: boolean
  isRateLimited: boolean
  result: RolloutResult | null
}): FlagUIState {
  if (params.isLoading) return 'loading'
  if (params.isRateLimited) return 'rate-limited'
  if (params.isOffline) return 'offline'
  if (params.isError) return 'error'
  if (params.result === null) return 'loading'

  if (params.result.reason.includes('not in audience')) return 'permission-denied'
  if (!params.result.enabled) return 'empty'

  return 'populated'
}
