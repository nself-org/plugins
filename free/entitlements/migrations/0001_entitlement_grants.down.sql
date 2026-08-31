-- Migration 0001 (down): entitlement_grants
DROP POLICY IF EXISTS entitlement_grants_isolation ON entitlement_grants;
DROP TABLE IF EXISTS entitlement_grants;
