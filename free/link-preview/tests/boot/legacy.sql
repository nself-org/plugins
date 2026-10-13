-- Boot-kit legacy fixture (tests/boot/tables-upgrade.sh LEGACY pass): the TS-era link-preview schema (lp_* tables) an old
-- install holds, one row each with fixed primary keys. Columns reduced to the key ones. Test data only.
CREATE TABLE IF NOT EXISTS lp_link_previews (
  id UUID PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  url TEXT NOT NULL,
  title TEXT,
  description TEXT,
  fetch_status VARCHAR(32) DEFAULT 'ok',
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, url)
);
CREATE INDEX IF NOT EXISTS idx_lp_link_previews_account ON lp_link_previews(source_account_id);
CREATE INDEX IF NOT EXISTS idx_lp_link_previews_url ON lp_link_previews(url);

CREATE TABLE IF NOT EXISTS lp_link_preview_usage (
  id UUID PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  preview_id UUID NOT NULL,
  message_id VARCHAR(255),
  clicked BOOLEAN DEFAULT false,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_lp_link_preview_usage_account ON lp_link_preview_usage(source_account_id);

CREATE TABLE IF NOT EXISTS lp_preview_templates (
  id UUID PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  url_pattern TEXT NOT NULL,
  template TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_lp_preview_templates_account ON lp_preview_templates(source_account_id);

CREATE TABLE IF NOT EXISTS lp_oembed_providers (
  id UUID PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  endpoint_url TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_lp_oembed_providers_account ON lp_oembed_providers(source_account_id);

CREATE TABLE IF NOT EXISTS lp_url_blocklist (
  id UUID PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  pattern TEXT NOT NULL,
  pattern_type VARCHAR(16) DEFAULT 'exact',
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_lp_url_blocklist_account ON lp_url_blocklist(source_account_id);

CREATE TABLE IF NOT EXISTS lp_preview_settings (
  id UUID PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  scope VARCHAR(64) NOT NULL DEFAULT 'global',
  settings JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_lp_preview_settings_account ON lp_preview_settings(source_account_id);

CREATE TABLE IF NOT EXISTS lp_preview_analytics (
  id UUID PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  preview_id UUID NOT NULL,
  day DATE NOT NULL DEFAULT CURRENT_DATE,
  views INTEGER DEFAULT 0,
  clicks INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_lp_preview_analytics_account ON lp_preview_analytics(source_account_id);

INSERT INTO lp_link_previews (id, url, title) VALUES ('00000000-0000-4000-8000-0000000000b1', 'https://example.invalid/legacy', 'Legacy');
INSERT INTO lp_link_preview_usage (id, preview_id, message_id) VALUES ('00000000-0000-4000-8000-0000000000b2', '00000000-0000-4000-8000-0000000000b1', 'legacy-message');
INSERT INTO lp_preview_templates (id, name, url_pattern, template) VALUES ('00000000-0000-4000-8000-0000000000b3', 'legacy-template', '.*', '<div/>');
INSERT INTO lp_oembed_providers (id, name, endpoint_url) VALUES ('00000000-0000-4000-8000-0000000000b4', 'legacy-provider', 'https://example.invalid/oembed');
INSERT INTO lp_url_blocklist (id, pattern) VALUES ('00000000-0000-4000-8000-0000000000b5', 'blocked.example.invalid');
INSERT INTO lp_preview_settings (id, scope) VALUES ('00000000-0000-4000-8000-0000000000b6', 'global');
INSERT INTO lp_preview_analytics (id, preview_id) VALUES ('00000000-0000-4000-8000-0000000000b7', '00000000-0000-4000-8000-0000000000b1');
