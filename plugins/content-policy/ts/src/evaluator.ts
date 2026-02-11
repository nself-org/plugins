/**
 * Content Policy Evaluator
 * Core evaluation engine for content policy rules
 */

import { createLogger } from '@nself/plugin-utils';
import type {
  RuleRecord,
  RuleEvaluationResult,
  EvaluationContext,
  EvaluateRequest,
  EvaluateResponse,
  MatchedRule,
  EvaluationResult,
  RuleConfig,
  KeywordRuleConfig,
  RegexRuleConfig,
  LengthRuleConfig,
  MediaTypeRuleConfig,
  LinkCheckRuleConfig,
} from './types.js';
import { ContentPolicyDatabase } from './database.js';
import { PROFANITY_WORDS } from './profanity.js';

const logger = createLogger('content-policy:evaluator');

export class ContentPolicyEvaluator {
  constructor(private db: ContentPolicyDatabase) {}

  /**
   * Evaluate content against all applicable policies
   */
  async evaluate(request: EvaluateRequest): Promise<EvaluateResponse> {
    const startTime = Date.now();

    // Fetch applicable policies
    const policies = await this.db.getPoliciesForContentType(request.content_type);

    // Filter by specific policy IDs if provided
    const applicablePolicies = request.policy_ids
      ? policies.filter(p => request.policy_ids!.includes(p.id))
      : policies;

    if (applicablePolicies.length === 0) {
      // No policies apply - allow by default
      const evaluation = await this.db.createEvaluation({
        content_type: request.content_type,
        content_id: request.content_id,
        content_text: request.content_text,
        submitter_id: request.submitter_id,
        result: 'allowed',
        matched_rules: [],
        score: 0,
        processing_time_ms: Date.now() - startTime,
      });

      return {
        result: 'allowed',
        matched_rules: [],
        score: 0,
        processing_time_ms: Date.now() - startTime,
        evaluation_id: evaluation.id,
        message: 'No policies apply to this content type',
      };
    }

    // Load word lists
    const wordLists = await this.db.getAllWordLists();
    const wordListMap = new Map(wordLists.map(wl => [wl.id, wl]));

    // Build evaluation context
    const context: EvaluationContext = {
      content_type: request.content_type,
      content_text: request.content_text,
      content_id: request.content_id,
      submitter_id: request.submitter_id,
      policies: applicablePolicies,
      word_lists: wordListMap,
    };

    // Evaluate all rules across all policies
    const matchedRules: MatchedRule[] = [];

    for (const policy of applicablePolicies) {
      if (policy.mode === 'disabled') {
        continue;
      }

      const rules = await this.db.listRules(policy.id);
      const enabledRules = rules.filter(r => r.enabled);

      for (const rule of enabledRules) {
        const ruleResult = await this.evaluateRule(rule, context);

        if (ruleResult.matched) {
          matchedRules.push({
            rule_id: rule.id,
            rule_name: rule.name,
            rule_type: rule.rule_type,
            action: rule.action,
            severity: rule.severity,
            message: ruleResult.message ?? rule.message ?? undefined,
            matched_text: ruleResult.matched_text,
          });
        }
      }
    }

    // Determine final result based on matched rules
    const finalResult = this.determineFinalResult(matchedRules);

    // Calculate score (0.0 = clean, 1.0 = definitely violates)
    const score = this.calculateScore(matchedRules);

    const processingTime = Date.now() - startTime;

    // Store evaluation
    const evaluation = await this.db.createEvaluation({
      content_type: request.content_type,
      content_id: request.content_id,
      content_text: request.content_text,
      submitter_id: request.submitter_id,
      policy_id: applicablePolicies[0]?.id,
      rule_id: matchedRules[0]?.rule_id,
      result: finalResult,
      matched_rules: matchedRules,
      score,
      processing_time_ms: processingTime,
    });

    return {
      result: finalResult,
      matched_rules: matchedRules,
      score,
      processing_time_ms: processingTime,
      evaluation_id: evaluation.id,
      message: this.buildResultMessage(finalResult, matchedRules),
    };
  }

  /**
   * Evaluate a single rule against content
   */
  private async evaluateRule(rule: RuleRecord, context: EvaluationContext): Promise<RuleEvaluationResult> {
    try {
      switch (rule.rule_type) {
        case 'keyword':
          return this.evaluateKeywordRule(rule, context);
        case 'regex':
          return this.evaluateRegexRule(rule, context);
        case 'length':
          return this.evaluateLengthRule(rule, context);
        case 'profanity':
          return this.evaluateProfanityRule(rule, context);
        case 'media_type':
          return this.evaluateMediaTypeRule(rule, context);
        case 'link_check':
          return this.evaluateLinkCheckRule(rule, context);
        default:
          logger.warn('Unknown rule type', { type: rule.rule_type });
          return { matched: false, rule };
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Rule evaluation failed', { rule: rule.name, error: message });
      return { matched: false, rule };
    }
  }

  /**
   * Keyword rule: check if any word from word list appears in content
   */
  private evaluateKeywordRule(rule: RuleRecord, context: EvaluationContext): RuleEvaluationResult {
    const config = rule.config as KeywordRuleConfig;
    let words: string[] = [];

    if (config.word_list_id) {
      const wordList = context.word_lists.get(config.word_list_id);
      if (wordList) {
        words = wordList.words;
      }
    } else if (config.words) {
      words = config.words;
    }

    if (words.length === 0) {
      return { matched: false, rule };
    }

    const caseSensitive = config.case_sensitive ?? false;
    const content = caseSensitive ? context.content_text : context.content_text.toLowerCase();

    for (const word of words) {
      const searchWord = caseSensitive ? word : word.toLowerCase();
      if (content.includes(searchWord)) {
        return {
          matched: true,
          rule,
          message: `Keyword matched: "${word}"`,
          matched_text: word,
        };
      }
    }

    return { matched: false, rule };
  }

  /**
   * Regex rule: test content against regex pattern
   */
  private evaluateRegexRule(rule: RuleRecord, context: EvaluationContext): RuleEvaluationResult {
    const config = rule.config as RegexRuleConfig;

    try {
      const regex = new RegExp(config.pattern, config.flags ?? 'i');
      const match = context.content_text.match(regex);

      if (match) {
        return {
          matched: true,
          rule,
          message: `Regex pattern matched`,
          matched_text: match[0],
        };
      }

      return { matched: false, rule };
    } catch (error) {
      logger.error('Invalid regex pattern', { pattern: config.pattern });
      return { matched: false, rule };
    }
  }

  /**
   * Length rule: check content length
   */
  private evaluateLengthRule(rule: RuleRecord, context: EvaluationContext): RuleEvaluationResult {
    const config = rule.config as LengthRuleConfig;
    const length = context.content_text.length;

    if (config.min_length !== undefined && length < config.min_length) {
      return {
        matched: true,
        rule,
        message: `Content too short (${length} < ${config.min_length})`,
      };
    }

    if (config.max_length !== undefined && length > config.max_length) {
      return {
        matched: true,
        rule,
        message: `Content too long (${length} > ${config.max_length})`,
      };
    }

    return { matched: false, rule };
  }

  /**
   * Profanity rule: check against built-in profanity word list
   */
  private evaluateProfanityRule(rule: RuleRecord, context: EvaluationContext): RuleEvaluationResult {
    const content = context.content_text.toLowerCase();
    const words = content.split(/\s+/);

    for (const word of words) {
      // Remove punctuation
      const cleanWord = word.replace(/[^\w]/g, '');

      if (PROFANITY_WORDS.has(cleanWord)) {
        return {
          matched: true,
          rule,
          message: `Profanity detected`,
          matched_text: cleanWord,
        };
      }
    }

    return { matched: false, rule };
  }

  /**
   * Media type rule: check if content type matches allowed/blocked types
   */
  private evaluateMediaTypeRule(rule: RuleRecord, context: EvaluationContext): RuleEvaluationResult {
    const config = rule.config as MediaTypeRuleConfig;

    if (config.allowed_types && config.allowed_types.length > 0) {
      if (!config.allowed_types.includes(context.content_type)) {
        return {
          matched: true,
          rule,
          message: `Content type "${context.content_type}" not allowed`,
        };
      }
    }

    if (config.blocked_types && config.blocked_types.length > 0) {
      if (config.blocked_types.includes(context.content_type)) {
        return {
          matched: true,
          rule,
          message: `Content type "${context.content_type}" is blocked`,
        };
      }
    }

    return { matched: false, rule };
  }

  /**
   * Link check rule: check if URLs in content match blocked/allowed domains
   */
  private evaluateLinkCheckRule(rule: RuleRecord, context: EvaluationContext): RuleEvaluationResult {
    const config = rule.config as LinkCheckRuleConfig;

    // Extract URLs from content
    const urlRegex = /https?:\/\/[^\s]+/g;
    const urls = context.content_text.match(urlRegex) ?? [];

    if (urls.length === 0) {
      return { matched: false, rule };
    }

    for (const url of urls) {
      try {
        const urlObj = new URL(url);
        const domain = urlObj.hostname;

        if (config.blocked_domains && config.blocked_domains.length > 0) {
          for (const blockedDomain of config.blocked_domains) {
            if (domain === blockedDomain || domain.endsWith(`.${blockedDomain}`)) {
              return {
                matched: true,
                rule,
                message: `Blocked domain detected: ${domain}`,
                matched_text: url,
              };
            }
          }
        }

        if (config.allowed_domains && config.allowed_domains.length > 0) {
          let allowed = false;
          for (const allowedDomain of config.allowed_domains) {
            if (domain === allowedDomain || domain.endsWith(`.${allowedDomain}`)) {
              allowed = true;
              break;
            }
          }
          if (!allowed) {
            return {
              matched: true,
              rule,
              message: `Domain not in allowlist: ${domain}`,
              matched_text: url,
            };
          }
        }
      } catch (error) {
        // Invalid URL, skip
        continue;
      }
    }

    return { matched: false, rule };
  }

  /**
   * Test a rule configuration without recording
   */
  async testRule(contentText: string, config: RuleConfig): Promise<{ matched: boolean; message?: string; matched_text?: string }> {
    // Create a temporary rule record for testing
    const tempRule: RuleRecord = {
      id: 'test',
      source_account_id: 'test',
      policy_id: 'test',
      name: 'Test Rule',
      rule_type: config.type,
      config,
      action: 'flag',
      severity: 'medium',
      message: null,
      enabled: true,
      created_at: new Date(),
      updated_at: new Date(),
    };

    const context: EvaluationContext = {
      content_type: 'test',
      content_text: contentText,
      policies: [],
      word_lists: new Map(),
    };

    const result = await this.evaluateRule(tempRule, context);

    return {
      matched: result.matched,
      message: result.message,
      matched_text: result.matched_text,
    };
  }

  /**
   * Determine final result based on matched rules
   * Priority: deny > quarantine > flag > allow
   */
  private determineFinalResult(matchedRules: MatchedRule[]): EvaluationResult {
    if (matchedRules.length === 0) {
      return 'allowed';
    }

    // Check for deny actions
    if (matchedRules.some(r => r.action === 'deny')) {
      return 'denied';
    }

    // Check for quarantine actions
    if (matchedRules.some(r => r.action === 'quarantine')) {
      return 'quarantined';
    }

    // Check for flag actions
    if (matchedRules.some(r => r.action === 'flag')) {
      return 'flagged';
    }

    return 'allowed';
  }

  /**
   * Calculate violation score (0.0 = clean, 1.0 = severe violations)
   */
  private calculateScore(matchedRules: MatchedRule[]): number {
    if (matchedRules.length === 0) {
      return 0.0;
    }

    const severityWeights = {
      low: 0.25,
      medium: 0.5,
      high: 0.75,
      critical: 1.0,
    };

    let totalWeight = 0;
    for (const rule of matchedRules) {
      totalWeight += severityWeights[rule.severity];
    }

    // Average severity, capped at 1.0
    return Math.min(totalWeight / matchedRules.length, 1.0);
  }

  /**
   * Build human-readable result message
   */
  private buildResultMessage(result: EvaluationResult, matchedRules: MatchedRule[]): string {
    if (result === 'allowed') {
      return 'Content allowed - no policy violations detected';
    }

    const ruleCount = matchedRules.length;
    const ruleWord = ruleCount === 1 ? 'rule' : 'rules';

    switch (result) {
      case 'denied':
        return `Content denied - ${ruleCount} ${ruleWord} violated`;
      case 'flagged':
        return `Content flagged for review - ${ruleCount} ${ruleWord} matched`;
      case 'quarantined':
        return `Content quarantined - ${ruleCount} ${ruleWord} violated`;
      default:
        return 'Content evaluated';
    }
  }
}
