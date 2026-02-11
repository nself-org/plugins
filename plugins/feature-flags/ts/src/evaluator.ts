/**
 * Feature Flag Evaluation Engine
 * Evaluates flags against rules and contexts
 */

import { createLogger } from '@nself/plugin-utils';
import type { FeatureFlagsDatabase } from './database.js';
import type {
  RuleRecord,
  SegmentRecord,
  EvaluationContext,
  EvaluationResult,
  RuleConditions,
  SegmentRule,
} from './types.js';

const logger = createLogger('feature-flags:evaluator');

/**
 * Consistent hash function for percentage-based rollouts
 * Returns a value between 0-99
 */
function hashPercentage(flagKey: string, userId: string): number {
  const str = `${flagKey}:${userId}`;
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    const char = str.charCodeAt(i);
    hash = ((hash << 5) - hash) + char;
    hash = hash & hash; // Convert to 32-bit int
  }
  return Math.abs(hash) % 100;
}

/**
 * Evaluate a segment rule against context
 */
function evaluateSegmentRule(rule: SegmentRule, context: EvaluationContext): boolean {
  const value = context[rule.attribute];

  switch (rule.operator) {
    case 'eq':
      return value === rule.value;
    case 'neq':
      return value !== rule.value;
    case 'gt':
      return typeof value === 'number' && typeof rule.value === 'number' && value > rule.value;
    case 'lt':
      return typeof value === 'number' && typeof rule.value === 'number' && value < rule.value;
    case 'gte':
      return typeof value === 'number' && typeof rule.value === 'number' && value >= rule.value;
    case 'lte':
      return typeof value === 'number' && typeof rule.value === 'number' && value <= rule.value;
    case 'contains':
      return typeof value === 'string' && typeof rule.value === 'string' && value.includes(rule.value);
    case 'regex':
      if (typeof value === 'string' && typeof rule.value === 'string') {
        try {
          const regex = new RegExp(rule.value);
          return regex.test(value);
        } catch (error) {
          logger.warn('Invalid regex in segment rule', { regex: rule.value, error });
          return false;
        }
      }
      return false;
    default:
      logger.warn('Unknown segment rule operator', { operator: rule.operator });
      return false;
  }
}

/**
 * Evaluate a segment against context
 */
function evaluateSegment(segment: SegmentRecord, context: EvaluationContext): boolean {
  if (segment.rules.length === 0) {
    return false;
  }

  const results = segment.rules.map(rule => evaluateSegmentRule(rule, context));

  if (segment.match_type === 'all') {
    return results.every(r => r);
  } else {
    return results.some(r => r);
  }
}

/**
 * Evaluate a rule against context
 */
async function evaluateRule(
  rule: RuleRecord,
  userId: string | undefined,
  context: EvaluationContext,
  db: FeatureFlagsDatabase,
  flagKey: string
): Promise<boolean> {
  if (!rule.enabled) {
    return false;
  }

  const conditions = rule.conditions as RuleConditions;

  switch (rule.rule_type) {
    case 'percentage': {
      if (!userId || typeof conditions.percentage !== 'number') {
        return false;
      }
      const hash = hashPercentage(flagKey, userId);
      return hash < conditions.percentage;
    }

    case 'user_list': {
      if (!userId || !Array.isArray(conditions.users)) {
        return false;
      }
      return conditions.users.includes(userId);
    }

    case 'segment': {
      if (!conditions.segment_id) {
        return false;
      }
      const segment = await db.getSegment(conditions.segment_id);
      if (!segment) {
        logger.warn('Segment not found', { segmentId: conditions.segment_id });
        return false;
      }
      return evaluateSegment(segment, context);
    }

    case 'attribute': {
      if (!conditions.attribute || !conditions.operator) {
        return false;
      }
      const value = context[conditions.attribute];
      const targetValue = conditions.attribute_value;

      switch (conditions.operator) {
        case 'eq':
          return value === targetValue;
        case 'neq':
          return value !== targetValue;
        case 'gt':
          return typeof value === 'number' && typeof targetValue === 'number' && value > targetValue;
        case 'lt':
          return typeof value === 'number' && typeof targetValue === 'number' && value < targetValue;
        case 'gte':
          return typeof value === 'number' && typeof targetValue === 'number' && value >= targetValue;
        case 'lte':
          return typeof value === 'number' && typeof targetValue === 'number' && value <= targetValue;
        case 'contains':
          return typeof value === 'string' && typeof targetValue === 'string' && value.includes(targetValue);
        case 'regex':
          if (typeof value === 'string' && typeof targetValue === 'string') {
            try {
              const regex = new RegExp(targetValue);
              return regex.test(value);
            } catch (error) {
              logger.warn('Invalid regex in attribute rule', { regex: targetValue, error });
              return false;
            }
          }
          return false;
        default:
          return false;
      }
    }

    case 'schedule': {
      if (!conditions.start_at && !conditions.end_at) {
        return false;
      }

      const now = new Date();

      if (conditions.start_at) {
        const startAt = new Date(conditions.start_at);
        if (now < startAt) {
          return false;
        }
      }

      if (conditions.end_at) {
        const endAt = new Date(conditions.end_at);
        if (now > endAt) {
          return false;
        }
      }

      return true;
    }

    default:
      logger.warn('Unknown rule type', { ruleType: rule.rule_type });
      return false;
  }
}

/**
 * Evaluate a flag for the given context
 */
export async function evaluateFlag(
  flagKey: string,
  userId: string | undefined,
  context: EvaluationContext,
  db: FeatureFlagsDatabase
): Promise<EvaluationResult> {
  try {
    // Get flag
    const flag = await db.getFlag(flagKey);
    if (!flag) {
      return {
        flag_key: flagKey,
        value: false,
        reason: 'not_found',
      };
    }

    // Check if enabled
    if (!flag.enabled) {
      return {
        flag_key: flagKey,
        value: parseJsonValue(flag.default_value),
        reason: 'disabled',
      };
    }

    // Get rules sorted by priority
    const rules = await db.getRulesByFlagId(flag.id);

    // Evaluate rules in order
    for (const rule of rules) {
      const matches = await evaluateRule(rule, userId, context, db, flagKey);
      if (matches) {
        return {
          flag_key: flagKey,
          value: parseJsonValue(rule.value),
          reason: 'rule_match',
          rule_id: rule.id,
        };
      }
    }

    // No rules matched - return default
    return {
      flag_key: flagKey,
      value: parseJsonValue(flag.default_value),
      reason: 'default',
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Flag evaluation failed', { flagKey, error: message });
    return {
      flag_key: flagKey,
      value: false,
      reason: 'error',
      error: message,
    };
  }
}

/**
 * Parse JSON value stored in database
 */
function parseJsonValue(value: unknown): unknown {
  if (typeof value === 'string') {
    try {
      return JSON.parse(value);
    } catch {
      return value;
    }
  }
  return value;
}

/**
 * Evaluate multiple flags in batch
 */
export async function evaluateFlags(
  flagKeys: string[],
  userId: string | undefined,
  context: EvaluationContext,
  db: FeatureFlagsDatabase
): Promise<EvaluationResult[]> {
  const results: EvaluationResult[] = [];

  for (const flagKey of flagKeys) {
    const result = await evaluateFlag(flagKey, userId, context, db);
    results.push(result);
  }

  return results;
}
