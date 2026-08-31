-- Migration 005: Seed autonomy-tier threshold rows
-- nself-eval-gate plugin — eval harness foundation
-- Idempotent: INSERT ... ON CONFLICT DO NOTHING
-- Values aligned with autonomy-tiers.md (P4-E8-W1-S01-T01) tier definitions:
--   supervised  = no gate (enforced=false, thresholds 0.00)
--   semi-auto   = recall-quality-v1 must score >= 0.75 pass-rate 0.80
--   full-auto   = recall-quality-v1 + generation-v1 must score >= 0.88 pass-rate 0.92

INSERT INTO np_eval_thresholds (autonomy_tier, min_pass_rate, min_suite_score, applies_to, enforced)
VALUES
    ('supervised',  0.00, 0.00, '{}',                                            false),
    ('semi-auto',   0.80, 0.75, ARRAY['recall-quality-v1'],                      true),
    ('full-auto',   0.92, 0.88, ARRAY['recall-quality-v1', 'generation-v1'],     true)
ON CONFLICT (autonomy_tier) DO NOTHING;
