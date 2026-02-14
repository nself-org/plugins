-- =============================================================================
-- Notifications Plugin Schema
-- Multi-channel notification system with templates, preferences, and tracking
-- =============================================================================

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- =============================================================================
-- Notification Templates
-- Reusable templates with Handlebars support
-- =============================================================================

CREATE TABLE IF NOT EXISTS np_notifications_templates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL UNIQUE,          -- Template identifier (welcome_email, password_reset, etc.)
    category VARCHAR(50) NOT NULL,              -- transactional, marketing, system, alert
    channels JSONB NOT NULL DEFAULT '["email"]', -- Supported channels: email, push, sms
    subject VARCHAR(255),                       -- Email subject (Handlebars supported)
    body_text TEXT,                             -- Plain text body (Handlebars supported)
    body_html TEXT,                             -- HTML body (Handlebars supported)
    push_title VARCHAR(255),                    -- Push notification title
    push_body TEXT,                             -- Push notification body
    sms_body TEXT,                              -- SMS body (160 chars recommended)
    metadata JSONB DEFAULT '{}',                -- Template variables, defaults, etc.
    variables JSONB DEFAULT '[]',               -- Required/optional variables
    active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_by UUID,                            -- User who created template
    updated_by UUID
);

CREATE INDEX IF NOT EXISTS idx_np_notifications_templates_name ON np_notifications_templates(name);
CREATE INDEX IF NOT EXISTS idx_np_notifications_templates_category ON np_notifications_templates(category);
CREATE INDEX IF NOT EXISTS idx_np_notifications_templates_active ON np_notifications_templates(active);

-- =============================================================================
-- Notification Preferences
-- User opt-in/out per channel and category
-- =============================================================================

CREATE TABLE IF NOT EXISTS np_notifications_preferences (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    channel VARCHAR(20) NOT NULL,               -- email, push, sms
    category VARCHAR(50) NOT NULL,              -- transactional, marketing, system, alert
    enabled BOOLEAN DEFAULT TRUE,
    frequency VARCHAR(20) DEFAULT 'immediate',  -- immediate, hourly, daily, weekly, disabled
    quiet_hours JSONB,                          -- { start: "22:00", end: "08:00", timezone: "UTC" }
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(user_id, channel, category)
);

CREATE INDEX IF NOT EXISTS idx_np_notifications_preferences_user ON np_notifications_preferences(user_id);
CREATE INDEX IF NOT EXISTS idx_np_notifications_preferences_channel ON np_notifications_preferences(channel);
CREATE INDEX IF NOT EXISTS idx_np_notifications_preferences_enabled ON np_notifications_preferences(enabled);

-- =============================================================================
-- Notification Messages (Sent Log)
-- Record of all sent notifications
-- =============================================================================

CREATE TABLE IF NOT EXISTS np_notifications_messages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    template_id UUID REFERENCES np_notifications_templates(id),
    template_name VARCHAR(255),                 -- Denormalized for history
    channel VARCHAR(20) NOT NULL,               -- email, push, sms
    category VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, queued, sent, delivered, failed, bounced
    priority INTEGER DEFAULT 5,                 -- 1 (highest) to 10 (lowest)

    -- Recipients
    recipient_email VARCHAR(255),
    recipient_phone VARCHAR(50),
    recipient_push_token TEXT,
    recipient_user_id UUID,

    -- Content (rendered from template)
    subject VARCHAR(255),
    body_text TEXT,
    body_html TEXT,

    -- Delivery tracking
    provider VARCHAR(50),                       -- resend, sendgrid, twilio, fcm, etc.
    provider_message_id VARCHAR(255),           -- External message ID
    provider_response JSONB,                    -- Raw provider response

    -- Timing
    scheduled_at TIMESTAMP WITH TIME ZONE,      -- When to send
    sent_at TIMESTAMP WITH TIME ZONE,           -- When actually sent
    delivered_at TIMESTAMP WITH TIME ZONE,      -- When delivered
    failed_at TIMESTAMP WITH TIME ZONE,

    -- Engagement tracking
    opened_at TIMESTAMP WITH TIME ZONE,         -- Email opened
    clicked_at TIMESTAMP WITH TIME ZONE,        -- Link clicked
    unsubscribed_at TIMESTAMP WITH TIME ZONE,

    -- Retries
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    next_retry_at TIMESTAMP WITH TIME ZONE,

    -- Error handling
    error_message TEXT,
    error_code VARCHAR(50),
    error_details JSONB,

    -- Metadata
    metadata JSONB DEFAULT '{}',                -- Custom data, tracking params
    tags JSONB DEFAULT '[]',                    -- Categorization
    batch_id UUID,                              -- For batch/digest sends

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_notifications_messages_user ON np_notifications_messages(user_id);
CREATE INDEX IF NOT EXISTS idx_np_notifications_messages_template ON np_notifications_messages(template_id);
CREATE INDEX IF NOT EXISTS idx_np_notifications_messages_status ON np_notifications_messages(status);
CREATE INDEX IF NOT EXISTS idx_np_notifications_messages_channel ON np_notifications_messages(channel);
CREATE INDEX IF NOT EXISTS idx_np_notifications_messages_category ON np_notifications_messages(category);
CREATE INDEX IF NOT EXISTS idx_np_notifications_messages_scheduled ON np_notifications_messages(scheduled_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_np_notifications_messages_retry ON np_notifications_messages(next_retry_at) WHERE status = 'failed' AND retry_count < max_retries;
CREATE INDEX IF NOT EXISTS idx_np_notifications_messages_batch ON np_notifications_messages(batch_id) WHERE batch_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_np_notifications_messages_created ON np_notifications_messages(created_at);
CREATE INDEX IF NOT EXISTS idx_np_notifications_messages_provider_id ON np_notifications_messages(provider_message_id);

-- =============================================================================
-- Notification Queue
-- Processing queue for async delivery
-- =============================================================================

CREATE TABLE IF NOT EXISTS np_notifications_queue (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    notification_id UUID NOT NULL REFERENCES np_notifications_messages(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, processing, completed, failed
    priority INTEGER DEFAULT 5,
    attempts INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 3,
    next_attempt_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_error TEXT,
    processing_started_at TIMESTAMP WITH TIME ZONE,
    processing_completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(notification_id)
);

CREATE INDEX IF NOT EXISTS idx_np_notifications_queue_status ON np_notifications_queue(status);
CREATE INDEX IF NOT EXISTS idx_np_notifications_queue_next_attempt ON np_notifications_queue(next_attempt_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_np_notifications_queue_priority ON np_notifications_queue(priority);

-- =============================================================================
-- Notification Providers
-- Provider configuration and priority
-- =============================================================================

CREATE TABLE IF NOT EXISTS np_notifications_providers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(50) NOT NULL UNIQUE,           -- resend, sendgrid, mailgun, ses, twilio, fcm, etc.
    type VARCHAR(20) NOT NULL,                  -- email, push, sms
    priority INTEGER DEFAULT 5,                 -- Lower = higher priority
    enabled BOOLEAN DEFAULT TRUE,

    -- Configuration
    config JSONB NOT NULL DEFAULT '{}',         -- API keys, endpoints, etc. (encrypted in production)
    rate_limit_per_second INTEGER,
    rate_limit_per_hour INTEGER,
    rate_limit_per_day INTEGER,

    -- Health tracking
    success_count BIGINT DEFAULT 0,
    failure_count BIGINT DEFAULT 0,
    last_success_at TIMESTAMP WITH TIME ZONE,
    last_failure_at TIMESTAMP WITH TIME ZONE,
    health_status VARCHAR(20) DEFAULT 'healthy', -- healthy, degraded, unhealthy

    -- Metadata
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_notifications_providers_type ON np_notifications_providers(type);
CREATE INDEX IF NOT EXISTS idx_np_notifications_providers_enabled ON np_notifications_providers(enabled);
CREATE INDEX IF NOT EXISTS idx_np_notifications_providers_priority ON np_notifications_providers(priority);
CREATE INDEX IF NOT EXISTS idx_np_notifications_providers_health ON np_notifications_providers(health_status);

-- =============================================================================
-- Notification Batches
-- For digest/batch sends
-- =============================================================================

CREATE TABLE IF NOT EXISTS np_notifications_batches (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255),
    category VARCHAR(50),
    batch_type VARCHAR(50) DEFAULT 'digest',    -- digest, bulk, scheduled
    status VARCHAR(20) DEFAULT 'pending',       -- pending, processing, completed, failed

    -- Timing
    interval_seconds INTEGER DEFAULT 86400,     -- How often to send (daily default)
    last_sent_at TIMESTAMP WITH TIME ZONE,
    next_send_at TIMESTAMP WITH TIME ZONE,

    -- Stats
    total_notifications INTEGER DEFAULT 0,
    sent_count INTEGER DEFAULT 0,
    failed_count INTEGER DEFAULT 0,

    -- Configuration
    config JSONB DEFAULT '{}',                  -- Grouping rules, filters, etc.
    metadata JSONB DEFAULT '{}',

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_notifications_batches_status ON np_notifications_batches(status);
CREATE INDEX IF NOT EXISTS idx_np_notifications_batches_next_send ON np_notifications_batches(next_send_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_np_notifications_batches_category ON np_notifications_batches(category);

-- =============================================================================
-- Views for Analytics
-- =============================================================================

-- Delivery rate by channel
CREATE OR REPLACE VIEW notification_delivery_rates AS
SELECT
    channel,
    category,
    DATE_TRUNC('day', created_at) AS date,
    COUNT(*) AS total,
    COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
    COUNT(*) FILTER (WHERE status = 'failed') AS failed,
    COUNT(*) FILTER (WHERE status = 'bounced') AS bounced,
    ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'delivered') / NULLIF(COUNT(*), 0), 2) AS delivery_rate
FROM np_notifications_messages
WHERE created_at >= NOW() - INTERVAL '30 days'
GROUP BY channel, category, DATE_TRUNC('day', created_at)
ORDER BY date DESC, channel;

-- Engagement metrics
CREATE OR REPLACE VIEW notification_engagement AS
SELECT
    channel,
    category,
    DATE_TRUNC('day', created_at) AS date,
    COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
    COUNT(*) FILTER (WHERE opened_at IS NOT NULL) AS opened,
    COUNT(*) FILTER (WHERE clicked_at IS NOT NULL) AS clicked,
    COUNT(*) FILTER (WHERE unsubscribed_at IS NOT NULL) AS unsubscribed,
    ROUND(100.0 * COUNT(*) FILTER (WHERE opened_at IS NOT NULL) / NULLIF(COUNT(*) FILTER (WHERE status = 'delivered'), 0), 2) AS open_rate,
    ROUND(100.0 * COUNT(*) FILTER (WHERE clicked_at IS NOT NULL) / NULLIF(COUNT(*) FILTER (WHERE status = 'delivered'), 0), 2) AS click_rate
FROM np_notifications_messages
WHERE channel = 'email'
  AND created_at >= NOW() - INTERVAL '30 days'
GROUP BY channel, category, DATE_TRUNC('day', created_at)
ORDER BY date DESC;

-- Provider health summary
CREATE OR REPLACE VIEW notification_provider_health AS
SELECT
    name,
    type,
    enabled,
    health_status,
    success_count,
    failure_count,
    ROUND(100.0 * success_count / NULLIF(success_count + failure_count, 0), 2) AS success_rate,
    last_success_at,
    last_failure_at
FROM np_notifications_providers
ORDER BY type, priority;

-- User notification summary
CREATE OR REPLACE VIEW notification_user_summary AS
SELECT
    user_id,
    COUNT(*) AS total_notifications,
    COUNT(*) FILTER (WHERE channel = 'email') AS email_count,
    COUNT(*) FILTER (WHERE channel = 'push') AS push_count,
    COUNT(*) FILTER (WHERE channel = 'sms') AS sms_count,
    COUNT(*) FILTER (WHERE status = 'delivered') AS delivered_count,
    COUNT(*) FILTER (WHERE status = 'failed') AS failed_count,
    MAX(created_at) AS last_notification_at
FROM np_notifications_messages
WHERE created_at >= NOW() - INTERVAL '30 days'
GROUP BY user_id;

-- Queue backlog
CREATE OR REPLACE VIEW np_notifications_queue_backlog AS
SELECT
    nq.status,
    n.channel,
    n.priority,
    COUNT(*) AS count,
    MIN(nq.next_attempt_at) AS oldest_scheduled,
    AVG(nq.attempts) AS avg_attempts
FROM np_notifications_queue nq
JOIN np_notifications_messages n ON nq.notification_id = n.id
WHERE nq.status IN ('pending', 'processing')
GROUP BY nq.status, n.channel, n.priority
ORDER BY n.priority, nq.status;

-- =============================================================================
-- Functions
-- =============================================================================

-- Update timestamp trigger
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply updated_at trigger to all tables
CREATE TRIGGER update_np_notifications_templates_updated_at BEFORE UPDATE ON np_notifications_templates
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_np_notifications_preferences_updated_at BEFORE UPDATE ON np_notifications_preferences
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_np_notifications_messages_updated_at BEFORE UPDATE ON np_notifications_messages
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_np_notifications_queue_updated_at BEFORE UPDATE ON np_notifications_queue
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_np_notifications_providers_updated_at BEFORE UPDATE ON np_notifications_providers
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_np_notifications_batches_updated_at BEFORE UPDATE ON np_notifications_batches
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to get user preferences with fallbacks
CREATE OR REPLACE FUNCTION get_user_notification_preference(
    p_user_id UUID,
    p_channel VARCHAR,
    p_category VARCHAR
)
RETURNS BOOLEAN AS $$
DECLARE
    v_enabled BOOLEAN;
BEGIN
    -- Check for specific preference
    SELECT enabled INTO v_enabled
    FROM np_notifications_preferences
    WHERE user_id = p_user_id
      AND channel = p_channel
      AND category = p_category;

    -- Default to enabled if no preference found
    RETURN COALESCE(v_enabled, TRUE);
END;
$$ LANGUAGE plpgsql;

-- Function to check rate limits
CREATE OR REPLACE FUNCTION check_notification_rate_limit(
    p_user_id UUID,
    p_channel VARCHAR,
    p_window_seconds INTEGER,
    p_max_count INTEGER
)
RETURNS BOOLEAN AS $$
DECLARE
    v_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_count
    FROM np_notifications_messages
    WHERE user_id = p_user_id
      AND channel = p_channel
      AND created_at >= NOW() - (p_window_seconds || ' seconds')::INTERVAL;

    RETURN v_count < p_max_count;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- Seed Default Templates
-- =============================================================================

INSERT INTO np_notifications_templates (name, category, channels, subject, body_html, body_text) VALUES
(
    'welcome_email',
    'transactional',
    '["email"]',
    'Welcome to {{app_name}}!',
    '<h1>Welcome, {{user_name}}!</h1><p>We''re excited to have you on board.</p>',
    'Welcome, {{user_name}}! We''re excited to have you on board.'
),
(
    'password_reset',
    'transactional',
    '["email"]',
    'Reset your password',
    '<h1>Password Reset</h1><p>Click <a href="{{reset_url}}">here</a> to reset your password. This link expires in 1 hour.</p>',
    'Reset your password by visiting: {{reset_url}}. This link expires in 1 hour.'
),
(
    'email_verification',
    'transactional',
    '["email"]',
    'Verify your email address',
    '<h1>Email Verification</h1><p>Click <a href="{{verify_url}}">here</a> to verify your email address.</p>',
    'Verify your email by visiting: {{verify_url}}'
),
(
    'password_changed',
    'transactional',
    '["email"]',
    'Your password has been changed',
    '<h1>Password Changed</h1><p>Your password was successfully changed. If you did not make this change, please contact support immediately.</p>',
    'Your password was successfully changed. If you did not make this change, please contact support immediately.'
)
ON CONFLICT (name) DO NOTHING;

-- =============================================================================
-- Seed Default Providers
-- =============================================================================

INSERT INTO np_notifications_providers (name, type, priority, enabled, config) VALUES
('resend', 'email', 1, false, '{"api_key": "", "from": "noreply@example.com"}'),
('sendgrid', 'email', 2, false, '{"api_key": "", "from": "noreply@example.com"}'),
('mailgun', 'email', 3, false, '{"api_key": "", "domain": "", "from": "noreply@example.com"}'),
('ses', 'email', 4, false, '{"region": "us-east-1", "from": "noreply@example.com"}'),
('smtp', 'email', 5, false, '{"host": "smtp.gmail.com", "port": 587, "secure": true, "user": "", "pass": "", "from": "noreply@example.com"}'),
('fcm', 'push', 1, false, '{"server_key": ""}'),
('onesignal', 'push', 2, false, '{"app_id": "", "api_key": ""}'),
('webpush', 'push', 3, false, '{"vapid_public_key": "", "vapid_private_key": ""}'),
('twilio', 'sms', 1, false, '{"account_sid": "", "auth_token": "", "from": ""}'),
('plivo', 'sms', 2, false, '{"auth_id": "", "auth_token": "", "from": ""}'),
('sns', 'sms', 3, false, '{"region": "us-east-1"}')
ON CONFLICT (name) DO NOTHING;
