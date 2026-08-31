-- 001_np_payments.down.sql
-- Reverse of 001_np_payments.up.sql

-- Drop in FK-safe order: usage_records and payment_events reference subscriptions.
DROP TABLE IF EXISTS np_usage_records;
DROP TABLE IF EXISTS np_payment_events;
DROP TABLE IF EXISTS np_subscriptions;
