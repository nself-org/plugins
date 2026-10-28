-- Boot-kit seed (tests/boot/tables-upgrade.sh): one row in each of the 22 tables the basis InitSchema creates, written
-- through the basis schema (np_ names) before the upgrade. Parents first, matching the Stripe object graph; the basis
-- schema has no foreign keys, so the order is for readers, not the database. Test data only.
INSERT INTO np_stripe_customers (id, email, name) VALUES ('cus_seed1', 'seed@example.invalid', 'Seed Customer');
INSERT INTO np_stripe_products (id, name) VALUES ('prod_seed1', 'Seed Product');
INSERT INTO np_stripe_prices (id, product_id, currency, unit_amount, type) VALUES ('price_seed1', 'prod_seed1', 'usd', 1000, 'recurring');
INSERT INTO np_stripe_coupons (id, name, percent_off, duration) VALUES ('coupon_seed1', 'Seed Coupon', 10, 'once');
INSERT INTO np_stripe_promotion_codes (id, coupon_id, code) VALUES ('promo_seed1', 'coupon_seed1', 'SEED10');
INSERT INTO np_stripe_subscriptions (id, customer_id, status) VALUES ('sub_seed1', 'cus_seed1', 'active');
INSERT INTO np_stripe_subscription_items (id, subscription_id, price_id) VALUES ('si_seed1', 'sub_seed1', 'price_seed1');
INSERT INTO np_stripe_invoices (id, customer_id, subscription_id, currency, amount_due, subtotal, total) VALUES ('in_seed1', 'cus_seed1', 'sub_seed1', 'usd', 1000, 1000, 1000);
INSERT INTO np_stripe_invoice_items (id, customer_id, invoice_id, subscription_id, price_id, amount, currency) VALUES ('ii_seed1', 'cus_seed1', 'in_seed1', 'sub_seed1', 'price_seed1', 1000, 'usd');
INSERT INTO np_stripe_discounts (id, coupon_id, customer_id, subscription_id, promotion_code_id) VALUES ('di_seed1', 'coupon_seed1', 'cus_seed1', 'sub_seed1', 'promo_seed1');
INSERT INTO np_stripe_payment_methods (id, customer_id, type) VALUES ('pm_seed1', 'cus_seed1', 'card');
INSERT INTO np_stripe_payment_intents (id, customer_id, invoice_id, amount, currency, status) VALUES ('pi_seed1', 'cus_seed1', 'in_seed1', 1000, 'usd', 'succeeded');
INSERT INTO np_stripe_setup_intents (id, status) VALUES ('seti_seed1', 'succeeded');
INSERT INTO np_stripe_charges (id, customer_id, payment_intent_id, invoice_id, amount, currency, status) VALUES ('ch_seed1', 'cus_seed1', 'pi_seed1', 'in_seed1', 1000, 'usd', 'succeeded');
INSERT INTO np_stripe_refunds (id, amount, currency, status) VALUES ('re_seed1', 500, 'usd', 'succeeded');
INSERT INTO np_stripe_disputes (id, amount, currency, status, reason) VALUES ('dp_seed1', 1000, 'usd', 'needs_response', 'general');
INSERT INTO np_stripe_balance_transactions (id, amount, currency, net, type, status) VALUES ('txn_seed1', 1000, 'usd', 970, 'charge', 'available');
INSERT INTO np_stripe_payouts (id, amount, currency, status, type) VALUES ('po_seed1', 970, 'usd', 'paid', 'bank_account');
INSERT INTO np_stripe_checkout_sessions (id, mode) VALUES ('cs_seed1', 'subscription');
INSERT INTO np_stripe_tax_ids (id, type, value) VALUES ('txi_seed1', 'eu_vat', 'DE000000000');
INSERT INTO np_stripe_tax_rates (id, display_name, percentage) VALUES ('txr_seed1', 'Seed VAT', 1.9000);
INSERT INTO np_stripe_webhook_events (id, type, data) VALUES ('evt_seed1', 'customer.created', '{}');
