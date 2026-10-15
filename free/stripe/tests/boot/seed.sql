-- Upgrade-pass rows for the pre-conversion basis (793fe4b8^), whose InitSchema
-- already creates np_stripe_* tables. legacy.sql independently seeds the old
-- stripe_* names, exercising both-names-present warnings during the upgrade.
-- One synthetic row per manifest table, parents before children; no live data.

BEGIN;

INSERT INTO np_stripe_customers (id, source_account_id, email)
VALUES ('cus_boot', 'primary', 'boot@example.invalid');
INSERT INTO np_stripe_products (id, source_account_id, name)
VALUES ('prod_boot', 'primary', 'Boot fixture');
INSERT INTO np_stripe_prices (id, source_account_id, product_id, currency, type)
VALUES ('price_boot', 'primary', 'prod_boot', 'usd', 'recurring');
INSERT INTO np_stripe_coupons (id, source_account_id, duration)
VALUES ('coupon_boot', 'primary', 'once');
INSERT INTO np_stripe_promotion_codes (id, source_account_id, coupon_id, code)
VALUES ('promo_boot', 'primary', 'coupon_boot', 'BOOT');
INSERT INTO np_stripe_payment_methods (id, source_account_id, customer_id, type)
VALUES ('pm_boot', 'primary', 'cus_boot', 'card');
INSERT INTO np_stripe_subscriptions (id, source_account_id, customer_id, status)
VALUES ('sub_boot', 'primary', 'cus_boot', 'active');
INSERT INTO np_stripe_subscription_items (id, source_account_id, subscription_id, price_id)
VALUES ('si_boot', 'primary', 'sub_boot', 'price_boot');
INSERT INTO np_stripe_invoices (id, source_account_id, customer_id, subscription_id, currency, amount_due, subtotal, total)
VALUES ('in_boot', 'primary', 'cus_boot', 'sub_boot', 'usd', 100, 100, 100);
INSERT INTO np_stripe_invoice_items (id, source_account_id, invoice_id, price_id, amount, currency)
VALUES ('ii_boot', 'primary', 'in_boot', 'price_boot', 100, 'usd');
INSERT INTO np_stripe_payment_intents (id, source_account_id, customer_id, invoice_id, amount, currency, status)
VALUES ('pi_boot', 'primary', 'cus_boot', 'in_boot', 100, 'usd', 'succeeded');
INSERT INTO np_stripe_charges (id, source_account_id, customer_id, payment_intent_id, payment_method_id, amount, currency, status)
VALUES ('ch_boot', 'primary', 'cus_boot', 'pi_boot', 'pm_boot', 100, 'usd', 'succeeded');
INSERT INTO np_stripe_refunds (id, source_account_id, charge_id, amount, currency, status)
VALUES ('re_boot', 'primary', 'ch_boot', 10, 'usd', 'succeeded');
INSERT INTO np_stripe_disputes (id, source_account_id, charge_id, amount, currency, status, reason)
VALUES ('dp_boot', 'primary', 'ch_boot', 10, 'usd', 'needs_response', 'fraudulent');
INSERT INTO np_stripe_balance_transactions (id, source_account_id, amount, currency, net, type, status)
VALUES ('txn_boot', 'primary', 100, 'usd', 100, 'charge', 'available');
INSERT INTO np_stripe_payouts (id, source_account_id, amount, currency, status, type)
VALUES ('po_boot', 'primary', 100, 'usd', 'paid', 'bank_account');
INSERT INTO np_stripe_checkout_sessions (id, source_account_id, customer_id, subscription_id, payment_intent_id, mode)
VALUES ('cs_boot', 'primary', 'cus_boot', 'sub_boot', 'pi_boot', 'subscription');
INSERT INTO np_stripe_discounts (id, source_account_id, coupon_id, promotion_code_id, customer_id, subscription_id, invoice_id, checkout_session_id)
VALUES ('di_boot', 'primary', 'coupon_boot', 'promo_boot', 'cus_boot', 'sub_boot', 'in_boot', 'cs_boot');
INSERT INTO np_stripe_setup_intents (id, source_account_id, customer_id, payment_method_id, status)
VALUES ('seti_boot', 'primary', 'cus_boot', 'pm_boot', 'succeeded');
INSERT INTO np_stripe_tax_ids (id, source_account_id, customer_id, type, value)
VALUES ('txi_boot', 'primary', 'cus_boot', 'eu_vat', 'TEST000000');
INSERT INTO np_stripe_tax_rates (id, source_account_id, display_name, percentage)
VALUES ('txr_boot', 'primary', 'Boot tax', 1.0000);
INSERT INTO np_stripe_webhook_events (id, source_account_id, type, data)
VALUES ('evt_boot', 'primary', 'customer.created', '{"object":{"id":"cus_boot"}}');

COMMIT;
