-- Migration 002: Add source_account_id for multi-app isolation
-- Required by the Multi-Tenant Convention Wall (Hard Rule).
-- Additive only — safe to apply on running instances.

ALTER TABLE np_invitations_invitations
    ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_invitations_source_account
    ON np_invitations_invitations(source_account_id);
