-- Boot-kit legacy fixture (tests/boot/tables-upgrade.sh LEGACY pass): the TS-era invitations schema (inv_* tables) an old
-- install holds, one row each with fixed primary keys. Columns reduced to the key ones. Test data only.
CREATE TABLE IF NOT EXISTS inv_invitations (
  id UUID PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  type VARCHAR(64) NOT NULL DEFAULT 'app_signup',
  inviter_id VARCHAR(255) NOT NULL,
  invitee_email VARCHAR(255),
  code VARCHAR(64) NOT NULL UNIQUE,
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  expires_at TIMESTAMPTZ,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_inv_invitations_account ON inv_invitations(source_account_id);
CREATE INDEX IF NOT EXISTS idx_inv_invitations_status ON inv_invitations(status);

CREATE TABLE IF NOT EXISTS inv_templates (
  id UUID PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  body TEXT NOT NULL,
  enabled BOOLEAN DEFAULT true,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, name)
);
CREATE INDEX IF NOT EXISTS idx_inv_templates_account ON inv_templates(source_account_id);

CREATE TABLE IF NOT EXISTS inv_bulk_sends (
  id UUID PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  inviter_id VARCHAR(255) NOT NULL,
  total_count INTEGER DEFAULT 0,
  status VARCHAR(32) DEFAULT 'pending',
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_inv_bulk_sends_account ON inv_bulk_sends(source_account_id);

CREATE TABLE IF NOT EXISTS inv_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  event_type VARCHAR(128) NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}',
  processed BOOLEAN DEFAULT false,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_inv_webhook_events_account ON inv_webhook_events(source_account_id);

INSERT INTO inv_invitations (id, inviter_id, invitee_email, code) VALUES ('00000000-0000-4000-8000-0000000000a1', 'legacy-user', 'legacy@example.invalid', 'legacy-code-not-real');
INSERT INTO inv_templates (id, name, body) VALUES ('00000000-0000-4000-8000-0000000000a2', 'legacy-template', 'Hello');
INSERT INTO inv_bulk_sends (id, inviter_id) VALUES ('00000000-0000-4000-8000-0000000000a3', 'legacy-user');
INSERT INTO inv_webhook_events (id, event_type) VALUES ('legacy-invitation-event', 'invitation.created');
