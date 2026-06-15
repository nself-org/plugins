-- Rollback: Revert tables from stripe_* back to np_stripe_*
-- Reverse of 001_rename_stripe_to_np_stripe_down.sql

DO $$
BEGIN
  -- stripe_customers → np_stripe_customers
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_customers')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_customers') THEN
    ALTER TABLE stripe_customers RENAME TO np_stripe_customers;
  END IF;

  -- stripe_products → np_stripe_products
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_products')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_products') THEN
    ALTER TABLE stripe_products RENAME TO np_stripe_products;
  END IF;

  -- stripe_prices → np_stripe_prices
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_prices')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_prices') THEN
    ALTER TABLE stripe_prices RENAME TO np_stripe_prices;
  END IF;

  -- stripe_coupons → np_stripe_coupons
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_coupons')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_coupons') THEN
    ALTER TABLE stripe_coupons RENAME TO np_stripe_coupons;
  END IF;

  -- stripe_promotion_codes → np_stripe_promotion_codes
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_promotion_codes')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_promotion_codes') THEN
    ALTER TABLE stripe_promotion_codes RENAME TO np_stripe_promotion_codes;
  END IF;

  -- stripe_subscriptions → np_stripe_subscriptions
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_subscriptions')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_subscriptions') THEN
    ALTER TABLE stripe_subscriptions RENAME TO np_stripe_subscriptions;
  END IF;

  -- stripe_subscription_items → np_stripe_subscription_items
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_subscription_items')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_subscription_items') THEN
    ALTER TABLE stripe_subscription_items RENAME TO np_stripe_subscription_items;
  END IF;

  -- stripe_subscription_schedules → np_stripe_subscription_schedules
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_subscription_schedules')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_subscription_schedules') THEN
    ALTER TABLE stripe_subscription_schedules RENAME TO np_stripe_subscription_schedules;
  END IF;

  -- stripe_invoices → np_stripe_invoices
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_invoices')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_invoices') THEN
    ALTER TABLE stripe_invoices RENAME TO np_stripe_invoices;
  END IF;

  -- stripe_invoice_items → np_stripe_invoice_items
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_invoice_items')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_invoice_items') THEN
    ALTER TABLE stripe_invoice_items RENAME TO np_stripe_invoice_items;
  END IF;

  -- stripe_credit_notes → np_stripe_credit_notes
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_credit_notes')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_credit_notes') THEN
    ALTER TABLE stripe_credit_notes RENAME TO np_stripe_credit_notes;
  END IF;

  -- stripe_charges → np_stripe_charges
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_charges')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_charges') THEN
    ALTER TABLE stripe_charges RENAME TO np_stripe_charges;
  END IF;

  -- stripe_refunds → np_stripe_refunds
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_refunds')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_refunds') THEN
    ALTER TABLE stripe_refunds RENAME TO np_stripe_refunds;
  END IF;

  -- stripe_disputes → np_stripe_disputes
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_disputes')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_disputes') THEN
    ALTER TABLE stripe_disputes RENAME TO np_stripe_disputes;
  END IF;

  -- stripe_payment_intents → np_stripe_payment_intents
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_payment_intents')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_payment_intents') THEN
    ALTER TABLE stripe_payment_intents RENAME TO np_stripe_payment_intents;
  END IF;

  -- stripe_setup_intents → np_stripe_setup_intents
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_setup_intents')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_setup_intents') THEN
    ALTER TABLE stripe_setup_intents RENAME TO np_stripe_setup_intents;
  END IF;

  -- stripe_payment_methods → np_stripe_payment_methods
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_payment_methods')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_payment_methods') THEN
    ALTER TABLE stripe_payment_methods RENAME TO np_stripe_payment_methods;
  END IF;

  -- stripe_balance_transactions → np_stripe_balance_transactions
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_balance_transactions')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_balance_transactions') THEN
    ALTER TABLE stripe_balance_transactions RENAME TO np_stripe_balance_transactions;
  END IF;

  -- stripe_checkout_sessions → np_stripe_checkout_sessions
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_checkout_sessions')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_checkout_sessions') THEN
    ALTER TABLE stripe_checkout_sessions RENAME TO np_stripe_checkout_sessions;
  END IF;

  -- stripe_checkout_session_line_items → np_stripe_checkout_session_line_items
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_checkout_session_line_items')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_checkout_session_line_items') THEN
    ALTER TABLE stripe_checkout_session_line_items RENAME TO np_stripe_checkout_session_line_items;
  END IF;

  -- stripe_tax_ids → np_stripe_tax_ids
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_tax_ids')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_tax_ids') THEN
    ALTER TABLE stripe_tax_ids RENAME TO np_stripe_tax_ids;
  END IF;

  -- stripe_tax_rates → np_stripe_tax_rates
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_tax_rates')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_tax_rates') THEN
    ALTER TABLE stripe_tax_rates RENAME TO np_stripe_tax_rates;
  END IF;

  -- stripe_webhook_events → np_stripe_webhook_events
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_webhook_events')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_webhook_events') THEN
    ALTER TABLE stripe_webhook_events RENAME TO np_stripe_webhook_events;
  END IF;

  -- stripe_events → np_stripe_events
  IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'stripe_events')
     AND NOT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'np_stripe_events') THEN
    ALTER TABLE stripe_events RENAME TO np_stripe_events;
  END IF;
END $$;

-- Restore index renames (idempotent)
DO $$
DECLARE
  idx RECORD;
BEGIN
  FOR idx IN
    SELECT indexname FROM pg_indexes
    WHERE schemaname = 'public'
      AND indexname LIKE 'idx_stripe_%'
      AND NOT EXISTS (
        SELECT 1 FROM pg_indexes i2
        WHERE i2.schemaname = 'public'
          AND i2.indexname = 'idx_np_stripe_' || substring(pg_indexes.indexname FROM 11)
      )
  LOOP
    EXECUTE format('ALTER INDEX %I RENAME TO %I',
      idx.indexname,
      'idx_np_stripe_' || substring(idx.indexname FROM 11)
    );
  END LOOP;
END $$;
