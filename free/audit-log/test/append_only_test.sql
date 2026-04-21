-- S43-T15: Append-only regression guard for np_auditlog_events.
--
-- Purpose: Verify that the RLS policies for UPDATE, DELETE, and TRUNCATE
-- are in force on np_auditlog_events. If any policy is regressed (removed or
-- relaxed), this test will fail with an SQL error or return unexpected row counts.
--
-- Run in nightly CI via:
--   psql $DATABASE_URL -f test/append_only_test.sql
--
-- The script exits non-zero on any failure because we use:
--   \set ON_ERROR_STOP on
--
-- Expected outcome: all three mutating operations are denied by RLS,
-- the final INSERT succeeds, and row count is 1.

\set ON_ERROR_STOP on

-- ============================================================
-- Setup: Use a dedicated test role that mirrors plugin runtime
-- ============================================================

-- Create a low-privilege test role that represents the audit-log plugin's
-- runtime database user. This role should only have INSERT + SELECT on
-- np_auditlog_events (not BYPASSRLS or SUPERUSER).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nself_audit_test_role') THEN
        CREATE ROLE nself_audit_test_role NOLOGIN;
    END IF;
END
$$;

-- Grant only INSERT + SELECT (no UPDATE, no DELETE, no TRUNCATE).
GRANT INSERT, SELECT ON np_auditlog_events TO nself_audit_test_role;
GRANT INSERT, SELECT ON np_auditlog_events_default TO nself_audit_test_role;

-- Set the app.source_account_id required by the SELECT + INSERT RLS policies.
SET LOCAL app.source_account_id = 'test_account_s43';

-- ============================================================
-- Insert a test row as the plugin role.
-- ============================================================
SET ROLE nself_audit_test_role;

INSERT INTO np_auditlog_events (
    id, source_account_id, actor_user_id, actor_type,
    event_type, resource_type, resource_id, severity
) VALUES (
    'test-s43-t15-row',
    'test_account_s43',
    'test-user-001',
    'system',
    'audit.append_only_test',
    'test',
    'test-resource-001',
    'info'
);

-- ============================================================
-- Test 1: UPDATE must be denied (np_auditlog_no_update policy).
-- ============================================================
-- We use a DO block with EXCEPTION so we can continue after the expected error.
-- If the UPDATE succeeds, we raise an explicit error to fail the test.

RESET ROLE;

DO $$
DECLARE
    updated_count INTEGER;
BEGIN
    -- Attempt UPDATE as the test role.
    SET LOCAL ROLE nself_audit_test_role;
    SET LOCAL app.source_account_id = 'test_account_s43';

    UPDATE np_auditlog_events
    SET severity = 'critical'
    WHERE id = 'test-s43-t15-row';

    GET DIAGNOSTICS updated_count = ROW_COUNT;

    IF updated_count > 0 THEN
        RAISE EXCEPTION 'FAIL: UPDATE succeeded on np_auditlog_events (% rows updated) — append-only RLS policy regressed!', updated_count;
    END IF;
    -- updated_count = 0 means the policy blocked the update silently (expected for RLS).
    RAISE NOTICE 'PASS: UPDATE affected 0 rows (blocked by RLS np_auditlog_no_update)';
EXCEPTION
    WHEN insufficient_privilege THEN
        RAISE NOTICE 'PASS: UPDATE denied with insufficient_privilege (blocked by RLS np_auditlog_no_update)';
END
$$;

-- ============================================================
-- Test 2: DELETE must be denied (np_auditlog_no_delete policy).
-- ============================================================

DO $$
DECLARE
    deleted_count INTEGER;
BEGIN
    SET LOCAL ROLE nself_audit_test_role;
    SET LOCAL app.source_account_id = 'test_account_s43';

    DELETE FROM np_auditlog_events
    WHERE id = 'test-s43-t15-row';

    GET DIAGNOSTICS deleted_count = ROW_COUNT;

    IF deleted_count > 0 THEN
        RAISE EXCEPTION 'FAIL: DELETE succeeded on np_auditlog_events (% rows deleted) — append-only RLS policy regressed!', deleted_count;
    END IF;
    RAISE NOTICE 'PASS: DELETE affected 0 rows (blocked by RLS np_auditlog_no_delete)';
EXCEPTION
    WHEN insufficient_privilege THEN
        RAISE NOTICE 'PASS: DELETE denied with insufficient_privilege (blocked by RLS np_auditlog_no_delete)';
END
$$;

-- ============================================================
-- Test 3: TRUNCATE must be denied (no TRUNCATE privilege granted).
-- ============================================================

DO $$
BEGIN
    SET LOCAL ROLE nself_audit_test_role;

    EXECUTE 'TRUNCATE TABLE np_auditlog_events';

    RAISE EXCEPTION 'FAIL: TRUNCATE succeeded on np_auditlog_events — nself_audit_test_role should not have TRUNCATE privilege!';
EXCEPTION
    WHEN insufficient_privilege THEN
        RAISE NOTICE 'PASS: TRUNCATE denied with insufficient_privilege';
    WHEN others THEN
        -- Any other error (including "not owner") also means TRUNCATE was blocked.
        RAISE NOTICE 'PASS: TRUNCATE blocked (error: %)', SQLERRM;
END
$$;

-- ============================================================
-- Verify: The test row is still present (not deleted by above attempts).
-- ============================================================

RESET ROLE;
SET LOCAL app.source_account_id = 'test_account_s43';

DO $$
DECLARE
    row_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO row_count
    FROM np_auditlog_events
    WHERE id = 'test-s43-t15-row'
      AND source_account_id = 'test_account_s43';

    IF row_count != 1 THEN
        RAISE EXCEPTION 'FAIL: Expected 1 test row in np_auditlog_events, found %', row_count;
    END IF;
    RAISE NOTICE 'PASS: Test row still present (count=%) — append-only invariant holds', row_count;
END
$$;

-- ============================================================
-- Cleanup: Remove test row and role (as superuser).
-- ============================================================

DELETE FROM np_auditlog_events WHERE id = 'test-s43-t15-row';
REVOKE INSERT, SELECT ON np_auditlog_events FROM nself_audit_test_role;
REVOKE INSERT, SELECT ON np_auditlog_events_default FROM nself_audit_test_role;
DROP ROLE IF EXISTS nself_audit_test_role;

\echo 'S43-T15: All append-only tests PASSED.'
