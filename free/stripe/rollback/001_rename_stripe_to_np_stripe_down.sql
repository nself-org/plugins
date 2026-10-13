-- Migration: 001_rename_stripe_to_np_stripe_down.sql
-- Plugin: free/stripe
-- Purpose: Revert np_stripe_* tables back to stripe_* prefix
-- Idempotent: uses DO $$ blocks with IF EXISTS guards
-- WARNING: Apply only in dev/staging — rolling back renames in production
--          requires application downtime and Hasura metadata revert.

DO $$
BEGIN
  -- np_stripe_customers → stripe_customers
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_customers')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_customers') THEN
    ALTER TABLE np_stripe_customers RENAME TO stripe_customers;
  END IF;

  -- np_stripe_products → stripe_products
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_products')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_products') THEN
    ALTER TABLE np_stripe_products RENAME TO stripe_products;
  END IF;

  -- np_stripe_prices → stripe_prices
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_prices')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_prices') THEN
    ALTER TABLE np_stripe_prices RENAME TO stripe_prices;
  END IF;

  -- np_stripe_coupons → stripe_coupons
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_coupons')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_coupons') THEN
    ALTER TABLE np_stripe_coupons RENAME TO stripe_coupons;
  END IF;

  -- np_stripe_promotion_codes → stripe_promotion_codes
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_promotion_codes')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_promotion_codes') THEN
    ALTER TABLE np_stripe_promotion_codes RENAME TO stripe_promotion_codes;
  END IF;

  -- np_stripe_subscriptions → stripe_subscriptions
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_subscriptions')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_subscriptions') THEN
    ALTER TABLE np_stripe_subscriptions RENAME TO stripe_subscriptions;
  END IF;

  -- np_stripe_subscription_items → stripe_subscription_items
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_subscription_items')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_subscription_items') THEN
    ALTER TABLE np_stripe_subscription_items RENAME TO stripe_subscription_items;
  END IF;

  -- np_stripe_subscription_schedules → stripe_subscription_schedules
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_subscription_schedules')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_subscription_schedules') THEN
    ALTER TABLE np_stripe_subscription_schedules RENAME TO stripe_subscription_schedules;
  END IF;

  -- np_stripe_invoices → stripe_invoices
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_invoices')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_invoices') THEN
    ALTER TABLE np_stripe_invoices RENAME TO stripe_invoices;
  END IF;

  -- np_stripe_invoice_items → stripe_invoice_items
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_invoice_items')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_invoice_items') THEN
    ALTER TABLE np_stripe_invoice_items RENAME TO stripe_invoice_items;
  END IF;

  -- np_stripe_credit_notes → stripe_credit_notes
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_credit_notes')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_credit_notes') THEN
    ALTER TABLE np_stripe_credit_notes RENAME TO stripe_credit_notes;
  END IF;

  -- np_stripe_charges → stripe_charges
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_charges')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_charges') THEN
    ALTER TABLE np_stripe_charges RENAME TO stripe_charges;
  END IF;

  -- np_stripe_refunds → stripe_refunds
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_refunds')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_refunds') THEN
    ALTER TABLE np_stripe_refunds RENAME TO stripe_refunds;
  END IF;

  -- np_stripe_disputes → stripe_disputes
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_disputes')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_disputes') THEN
    ALTER TABLE np_stripe_disputes RENAME TO stripe_disputes;
  END IF;

  -- np_stripe_payment_intents → stripe_payment_intents
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_payment_intents')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_payment_intents') THEN
    ALTER TABLE np_stripe_payment_intents RENAME TO stripe_payment_intents;
  END IF;

  -- np_stripe_setup_intents → stripe_setup_intents
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_setup_intents')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_setup_intents') THEN
    ALTER TABLE np_stripe_setup_intents RENAME TO stripe_setup_intents;
  END IF;

  -- np_stripe_payment_methods → stripe_payment_methods
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_payment_methods')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_payment_methods') THEN
    ALTER TABLE np_stripe_payment_methods RENAME TO stripe_payment_methods;
  END IF;

  -- np_stripe_balance_transactions → stripe_balance_transactions
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_balance_transactions')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_balance_transactions') THEN
    ALTER TABLE np_stripe_balance_transactions RENAME TO stripe_balance_transactions;
  END IF;

  -- np_stripe_checkout_sessions → stripe_checkout_sessions
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_checkout_sessions')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_checkout_sessions') THEN
    ALTER TABLE np_stripe_checkout_sessions RENAME TO stripe_checkout_sessions;
  END IF;

  -- np_stripe_checkout_session_line_items → stripe_checkout_session_line_items
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_checkout_session_line_items')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_checkout_session_line_items') THEN
    ALTER TABLE np_stripe_checkout_session_line_items RENAME TO stripe_checkout_session_line_items;
  END IF;

  -- np_stripe_tax_ids → stripe_tax_ids
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_tax_ids')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_tax_ids') THEN
    ALTER TABLE np_stripe_tax_ids RENAME TO stripe_tax_ids;
  END IF;

  -- np_stripe_tax_rates → stripe_tax_rates
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_tax_rates')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_tax_rates') THEN
    ALTER TABLE np_stripe_tax_rates RENAME TO stripe_tax_rates;
  END IF;

  -- np_stripe_webhook_events → stripe_webhook_events
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_webhook_events')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_webhook_events') THEN
    ALTER TABLE np_stripe_webhook_events RENAME TO stripe_webhook_events;
  END IF;

  -- np_stripe_events → stripe_events
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_events')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_events') THEN
    ALTER TABLE np_stripe_events RENAME TO stripe_events;
  END IF;
END $$;

-- Revert index renames (idempotent)
DO $$
DECLARE
  idx RECORD;
BEGIN
  FOR idx IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND indexname LIKE 'idx_np_stripe_%'
      AND NOT EXISTS (
        SELECT 1 FROM pg_indexes i2
        WHERE i2.schemaname = 'public'
          AND i2.indexname = 'idx_stripe_' || substring(pg_indexes.indexname FROM 13)
      )
  LOOP
    EXECUTE format('ALTER INDEX %I RENAME TO %I',
      idx.indexname,
      'idx_stripe_' || substring(idx.indexname FROM 12)
    );
  END LOOP;
END $$;
