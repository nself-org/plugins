-- Migration 006: Add is_golden column to nclaw_memory_facts (conditional)
-- nself-eval-gate plugin — eval harness foundation
-- Idempotent: ADD COLUMN IF NOT EXISTS
-- Depends on: nclaw_memory_facts table existing (nclaw plugin)
-- Purpose: Mark specific memory facts as golden (hand-curated eval ground truth).
--   is_golden=true facts are used by RecallQualityEval and are NEVER auto-generated.
--   Partial index accelerates golden-set lookups during eval runs.

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_name = 'nclaw_memory_facts'
    ) THEN
        ALTER TABLE nclaw_memory_facts
            ADD COLUMN IF NOT EXISTS is_golden BOOLEAN NOT NULL DEFAULT false;

        -- Partial index for efficient golden-set lookups during eval
        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes
            WHERE tablename = 'nclaw_memory_facts'
              AND indexname = 'nclaw_memory_facts_golden_idx'
        ) THEN
            CREATE INDEX nclaw_memory_facts_golden_idx
                ON nclaw_memory_facts (is_golden)
                WHERE is_golden = true;
        END IF;
    END IF;
END $$;
