# Table Prefix Violations - Fix Guide

## Summary
331 tables across 48 plugins need np_ prefix added.

## Major Offenders:

### stripe (10 tables)
- stripe_customers → np_stripe_customers
- stripe_subscriptions → np_stripe_subscriptions
- stripe_invoices → np_stripe_invoices
- (... 7 more)

### shopify (9 tables)
- shopify_products → np_shopify_products
- shopify_orders → np_shopify_orders
- (... 7 more)

### auth (7 tables)
- auth_users → np_auth_users
- auth_sessions → np_auth_sessions
- (... 5 more)

### chat (6 tables)
- chat_messages → np_chat_messages
- chat_rooms → np_chat_rooms
- (... 4 more)

## Fix Process:
1. For each plugin, rename tables in database.ts CREATE TABLE statements
2. Update all INSERT/UPDATE/DELETE queries
3. Update all indexes
4. Update plugin.json tables array
5. Test multi-app isolation

## Estimated Time: 30-40 hours
