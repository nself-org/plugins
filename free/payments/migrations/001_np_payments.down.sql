-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- 001_np_payments.down.sql
-- Reverse of 001_np_payments.up.sql

-- Drop in FK-safe order: usage_records and payment_events reference subscriptions.
DROP TABLE IF EXISTS public.np_usage_records;
DROP TABLE IF EXISTS public.np_payment_events;
DROP TABLE IF EXISTS public.np_subscriptions;
