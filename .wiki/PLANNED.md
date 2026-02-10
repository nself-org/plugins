# Planned Plugins

This document outlines all planned nself plugins, with detailed reasoning for each integration.

## Plugin Prioritization

Plugins are prioritized based on:
1. **Developer demand** - How many nself users need this integration
2. **Data sync value** - How valuable is having this data locally
3. **Webhook importance** - How critical are real-time events
4. **Implementation complexity** - Effort required to build
5. **Maintenance burden** - Ongoing effort to keep updated

---

## Priority 1: Billing & Payments

### Payment Processors Overview

All payment processor plugins share common patterns:
- Transaction/payment history sync
- Customer/payer data sync
- Webhook handling for real-time updates
- Subscription/recurring payment tracking

| Processor | Priority | Use Case | Complexity |
|-----------|----------|----------|------------|
| **Stripe** | Implemented | SaaS, subscriptions, marketplaces | Medium |
| **PayPal** | High | Consumer payments, international | Medium |
| **Square** | Medium | Retail, POS, in-person | Medium |
| **Braintree** | Medium | Enterprise, PayPal-owned | Medium |
| **Paddle** | Medium | SaaS, tax handling, MoR | Low |
| **LemonSqueezy** | Medium | Digital products, MoR | Low |
| **Gumroad** | Low | Creators, digital products | Low |
| **Chargebee** | Medium | Subscription management | Medium |
| **Recurly** | Medium | Enterprise subscriptions | Medium |
| **FastSpring** | Low | Global digital commerce | Medium |
| **2Checkout** | Low | Global payments | Medium |
| **Adyen** | Low | Enterprise, omnichannel | High |
| **Mollie** | Low | EU payments | Low |
| **Razorpay** | Low | India payments | Low |
| **Mercado Pago** | Low | LATAM payments | Medium |

### Stripe (Implemented v1.0.0)

**Status**: Released

See [plugins/stripe/](../plugins/stripe/) for implementation.

**Environment Variables**:
```bash
STRIPE_API_KEY=sk_live_xxx
STRIPE_WEBHOOK_SECRET=whsec_xxx
```

---

### PayPal

**Priority**: High
**Category**: Payments
**Complexity**: Medium

**Why PayPal?**
- 400+ million active users globally
- Essential for international commerce
- Consumer trust and buyer protection
- PayPal Checkout, Venmo, Pay Later options

**Data to Sync**:
- `paypal_transactions` - Payment history
- `paypal_orders` - Order records
- `paypal_subscriptions` - Billing agreements
- `paypal_disputes` - Chargebacks and disputes
- `paypal_payouts` - Batch payouts
- `paypal_webhooks` - Event log

**Key Webhooks**:
- `PAYMENT.CAPTURE.COMPLETED` - Payment received
- `PAYMENT.CAPTURE.REFUNDED` - Refund processed
- `BILLING.SUBSCRIPTION.CREATED` - New subscription
- `BILLING.SUBSCRIPTION.CANCELLED` - Subscription ended
- `CUSTOMER.DISPUTE.CREATED` - Dispute opened

**Environment Variables**:
```bash
PAYPAL_CLIENT_ID=xxx
PAYPAL_CLIENT_SECRET=xxx
PAYPAL_WEBHOOK_ID=xxx
PAYPAL_MODE=sandbox|live
```

---

### Square

**Priority**: Medium
**Category**: Payments
**Complexity**: Medium

**Why Square?**
- Strong retail/POS presence
- Unified online + in-person payments
- Inventory and catalog sync
- Loyalty program integration

**Data to Sync**:
- `square_payments` - Transaction history
- `square_orders` - Order records
- `square_customers` - Customer profiles
- `square_catalog` - Product catalog
- `square_inventory` - Stock levels
- `square_locations` - Store locations

**Environment Variables**:
```bash
SQUARE_ACCESS_TOKEN=xxx
SQUARE_LOCATION_ID=xxx
SQUARE_WEBHOOK_SIGNATURE_KEY=xxx
SQUARE_ENVIRONMENT=sandbox|production
```

---

### Paddle

**Priority**: Medium
**Category**: Payments (Merchant of Record)
**Complexity**: Low

**Why Paddle?**
- Handles tax compliance globally
- Merchant of Record (you don't deal with taxes)
- Simple API for SaaS
- License key management

**Data to Sync**:
- `paddle_transactions` - Payment history
- `paddle_subscriptions` - Active subscriptions
- `paddle_customers` - Customer data
- `paddle_products` - Product catalog
- `paddle_prices` - Pricing tiers

**Environment Variables**:
```bash
PADDLE_API_KEY=xxx
PADDLE_WEBHOOK_SECRET=xxx
PADDLE_ENVIRONMENT=sandbox|production
```

---

### LemonSqueezy

**Priority**: Medium
**Category**: Payments (Merchant of Record)
**Complexity**: Low

**Why LemonSqueezy?**
- Modern alternative to Gumroad
- Merchant of Record for digital products
- Simple, developer-friendly API
- Built-in license key system

**Data to Sync**:
- `lemonsqueezy_orders` - Order history
- `lemonsqueezy_subscriptions` - Subscriptions
- `lemonsqueezy_customers` - Customer profiles
- `lemonsqueezy_products` - Product catalog
- `lemonsqueezy_license_keys` - License management

**Environment Variables**:
```bash
LEMONSQUEEZY_API_KEY=xxx
LEMONSQUEEZY_WEBHOOK_SECRET=xxx
LEMONSQUEEZY_STORE_ID=xxx
```

---

### Shopify

**Priority**: High
**Category**: E-Commerce
**Complexity**: Medium-High

**Why Shopify?**
- Largest e-commerce platform with millions of stores
- Rich webhook ecosystem for real-time order/inventory updates
- Complex data model (products, variants, orders, customers, inventory)
- Critical for headless commerce setups using nself

**Data to Sync**:
- `shopify_products` - Product catalog with variants, images, metafields
- `shopify_collections` - Product collections and smart collections
- `shopify_customers` - Customer profiles and addresses
- `shopify_orders` - Order history with line items
- `shopify_inventory` - Stock levels per location
- `shopify_fulfillments` - Shipment tracking
- `shopify_refunds` - Refund transactions
- `shopify_webhooks` - Webhook event log

**Key Webhooks**:
- `orders/create`, `orders/paid`, `orders/fulfilled`, `orders/cancelled`
- `products/create`, `products/update`, `products/delete`
- `inventory_levels/update`
- `customers/create`, `customers/update`
- `refunds/create`
- `fulfillments/create`, `fulfillments/update`

**Use Cases**:
1. Headless commerce with custom frontend
2. Real-time inventory sync across systems
3. Order processing automation
4. Customer analytics and segmentation
5. Multi-channel product management

**Environment Variables**:
```bash
SHOPIFY_STORE_URL=mystore.myshopify.com
SHOPIFY_ACCESS_TOKEN=shpat_xxx
SHOPIFY_WEBHOOK_SECRET=xxx
SHOPIFY_API_VERSION=2024-01
```

---

### Plaid

**Priority**: High
**Category**: Finance
**Complexity**: High

**Why Plaid?**
- De facto standard for fintech bank connections
- Enables bank account verification for payments
- Transaction data for expense tracking/analytics
- Identity verification for compliance

**Data to Sync**:
- `plaid_items` - Connected bank accounts
- `plaid_accounts` - Individual accounts per institution
- `plaid_transactions` - Transaction history
- `plaid_balances` - Account balance snapshots
- `plaid_investments` - Investment holdings (premium)
- `plaid_webhooks` - Webhook event log

**Key Webhooks**:
- `TRANSACTIONS_WEBHOOK` - New transactions available
- `ITEM_WEBHOOK` - Account connection status changes
- `AUTH_WEBHOOK` - Auth data updates
- `HOLDINGS_WEBHOOK` - Investment changes
- `LIABILITIES_WEBHOOK` - Debt updates

**Use Cases**:
1. Bank account verification for ACH payments
2. Expense categorization and budgeting
3. Financial reporting and analytics
4. Net worth tracking for wealth management
5. Income verification for lending

**Environment Variables**:
```bash
PLAID_CLIENT_ID=xxx
PLAID_SECRET=xxx
PLAID_ENV=sandbox|development|production
PLAID_WEBHOOK_URL=https://example.com/webhooks/plaid
```

---

## Priority 2: DevOps & Development

### GitHub

**Priority**: High
**Category**: DevOps
**Complexity**: Medium

**Why GitHub?**
- Most popular code hosting platform
- Rich event system via webhooks
- Essential for CI/CD integration
- Issue/PR tracking for project management

**Data to Sync**:
- `github_repositories` - Repository metadata
- `github_issues` - Issue tracking
- `github_pull_requests` - PR history and status
- `github_commits` - Commit history
- `github_releases` - Release versions
- `github_actions` - Workflow runs
- `github_webhooks` - Webhook event log

**Key Webhooks**:
- `push` - Code pushed to repository
- `pull_request` - PR opened/merged/closed
- `issues` - Issue created/updated
- `release` - New release published
- `workflow_run` - CI/CD completion
- `deployment`, `deployment_status`
- `create`, `delete` (branches/tags)

**Use Cases**:
1. Track development velocity and metrics
2. Automate deployments on merge
3. Sync issues with internal task management
4. Monitor CI/CD pipeline status
5. Auto-update documentation on release

**Environment Variables**:
```bash
GITHUB_TOKEN=ghp_xxx
GITHUB_WEBHOOK_SECRET=xxx
GITHUB_ORG=myorg  # or GITHUB_USER
```

---

### Linear

**Priority**: Medium-High
**Category**: Productivity
**Complexity**: Low-Medium

**Why Linear?**
- Popular modern issue tracker
- GraphQL API for efficient querying
- Real-time webhook support
- Clean data model

**Data to Sync**:
- `linear_teams` - Team structure
- `linear_projects` - Project hierarchy
- `linear_issues` - Issue tracking
- `linear_cycles` - Sprint/cycle planning
- `linear_labels` - Labels and tags
- `linear_webhooks` - Webhook event log

**Key Webhooks**:
- `Issue` - Created, updated, removed
- `Comment` - New comments
- `Project` - Project updates
- `Cycle` - Sprint changes
- `IssueLabel` - Label changes

**Use Cases**:
1. Sync tasks with development workflow
2. Track sprint progress and velocity
3. Connect issues to commits/PRs
4. Generate development reports
5. Aggregate across multiple projects

**Environment Variables**:
```bash
LINEAR_API_KEY=lin_api_xxx
LINEAR_WEBHOOK_SECRET=xxx
LINEAR_TEAM_ID=xxx  # optional
```

---

## Priority 3: Communication & Support

### Intercom

**Priority**: Medium
**Category**: Communication
**Complexity**: Medium

**Why Intercom?**
- Leading customer messaging platform
- Rich user and conversation data
- Critical for support analytics
- Product usage tracking

**Data to Sync**:
- `intercom_contacts` - User profiles
- `intercom_companies` - Company accounts
- `intercom_conversations` - Chat history
- `intercom_admins` - Team members
- `intercom_articles` - Help center content
- `intercom_webhooks` - Webhook event log

**Key Webhooks**:
- `conversation.user.created` - New conversation
- `conversation.user.replied` - User reply
- `conversation.admin.replied` - Agent reply
- `conversation.admin.closed` - Conversation resolved
- `contact.created`, `contact.updated`
- `user.tag.created`

**Use Cases**:
1. Support ticket analytics
2. Customer health scoring
3. Response time metrics
4. User feedback aggregation
5. Help content optimization

**Environment Variables**:
```bash
INTERCOM_ACCESS_TOKEN=xxx
INTERCOM_WEBHOOK_SECRET=xxx
INTERCOM_ADMIN_ID=xxx
```

---

### Email Service Providers

These plugins share common patterns for email event tracking.

#### Resend

**Priority**: Medium
**Category**: Communication
**Complexity**: Low

**Why Resend?**
- Modern email API designed for developers
- Simple, clean webhook events
- Growing rapidly in developer community
- Excellent deliverability

**Data to Sync**:
- `resend_emails` - Sent email history
- `resend_domains` - Verified domains
- `resend_webhooks` - Event log

**Key Webhooks**:
- `email.sent` - Email accepted for delivery
- `email.delivered` - Email delivered
- `email.opened` - Email opened (if tracking enabled)
- `email.clicked` - Link clicked
- `email.bounced` - Hard/soft bounce
- `email.complained` - Spam report

---

#### SendGrid

**Priority**: Medium
**Category**: Communication
**Complexity**: Low-Medium

**Why SendGrid?**
- Industry standard for transactional email
- Comprehensive event tracking
- High volume support

**Data to Sync**:
- `sendgrid_emails` - Sent emails
- `sendgrid_templates` - Email templates
- `sendgrid_stats` - Delivery statistics
- `sendgrid_webhooks` - Event log

**Key Webhooks**:
- `processed`, `delivered`, `open`, `click`
- `bounce`, `dropped`, `deferred`
- `spam_report`, `unsubscribe`
- `group_unsubscribe`, `group_resubscribe`

---

#### Postmark

**Priority**: Medium
**Category**: Communication
**Complexity**: Low

**Why Postmark?**
- Focused on transactional email
- Best-in-class deliverability
- Simple, reliable webhooks

**Data to Sync**:
- `postmark_messages` - Message history
- `postmark_servers` - Server configuration
- `postmark_templates` - Email templates
- `postmark_webhooks` - Event log

**Key Webhooks**:
- `Delivery`, `Bounce`, `SpamComplaint`
- `Open`, `Click`, `SubscriptionChange`

---

## Priority 4: Productivity & Documentation

### Notion

**Priority**: Medium
**Category**: Productivity
**Complexity**: Medium

**Why Notion?**
- Popular all-in-one workspace
- Complex nested data structures
- Growing API capabilities
- Content management for docs

**Data to Sync**:
- `notion_pages` - Page content
- `notion_databases` - Database definitions
- `notion_database_items` - Database rows
- `notion_users` - Team members
- `notion_comments` - Page comments

**Key Webhooks** (limited):
Notion webhooks are limited but include:
- Page content changes
- Database property changes
- Comment additions

**Use Cases**:
1. Sync documentation with codebase
2. Pull content for headless CMS
3. Track project documentation
4. Aggregate team knowledge base

**Environment Variables**:
```bash
NOTION_TOKEN=secret_xxx
NOTION_DATABASE_IDS=xxx,yyy,zzz  # optional
```

---

### Airtable

**Priority**: Medium
**Category**: Productivity
**Complexity**: Low-Medium

**Why Airtable?**
- Popular spreadsheet-database hybrid
- Flexible data modeling
- Strong automation features
- Good for non-technical teams

**Data to Sync**:
- `airtable_bases` - Base metadata
- `airtable_tables` - Table structure
- `airtable_records` - Record data
- `airtable_webhooks` - Event log

**Key Webhooks**:
- Record created, updated, deleted
- Field changed
- Comment added

**Use Cases**:
1. Sync CRM data from Airtable
2. Product inventory management
3. Content calendars
4. Lead tracking
5. Custom data pipelines

**Environment Variables**:
```bash
AIRTABLE_API_KEY=patXXX
AIRTABLE_BASE_ID=appXXX
AIRTABLE_WEBHOOK_SECRET=xxx
```

---

## Priority 5: Analytics & Data

### Segment

**Priority**: Medium
**Category**: Analytics
**Complexity**: Medium

**Why Segment?**
- Customer data platform standard
- Central hub for all analytics
- Rich event streaming
- Identity resolution

**Data to Sync**:
- `segment_users` - User profiles
- `segment_events` - Event stream
- `segment_groups` - Group/company data
- `segment_sources` - Data sources

**Key Webhooks**:
Segment uses reverse ETL and webhooks:
- `identify` - User identification
- `track` - Custom events
- `page` - Page views
- `group` - Group associations

**Use Cases**:
1. Centralize analytics data
2. User behavior tracking
3. Marketing attribution
4. Product analytics
5. Customer journey mapping

---

### Mixpanel

**Priority**: Low-Medium
**Category**: Analytics
**Complexity**: Medium

**Why Mixpanel?**
- Leading product analytics platform
- Event-based tracking
- Cohort analysis
- Funnel visualization

**Data to Sync**:
- `mixpanel_events` - Event data
- `mixpanel_users` - User profiles
- `mixpanel_cohorts` - User segments
- `mixpanel_funnels` - Funnel definitions

---

## Priority 6: Marketing & Growth

### HubSpot

**Priority**: Medium
**Category**: Marketing
**Complexity**: High

**Why HubSpot?**
- Complete marketing/sales platform
- CRM integration
- Email marketing
- Lead tracking

**Data to Sync**:
- `hubspot_contacts` - Contact records
- `hubspot_companies` - Company accounts
- `hubspot_deals` - Sales pipeline
- `hubspot_tickets` - Support tickets
- `hubspot_emails` - Email tracking

**Key Webhooks**:
- Contact events
- Company events
- Deal stage changes
- Ticket updates
- Email engagement

---

### Mailchimp

**Priority**: Low-Medium
**Category**: Marketing
**Complexity**: Low-Medium

**Why Mailchimp?**
- Popular email marketing platform
- Newsletter management
- Audience segmentation
- Campaign analytics

**Data to Sync**:
- `mailchimp_lists` - Audience lists
- `mailchimp_members` - Subscribers
- `mailchimp_campaigns` - Email campaigns
- `mailchimp_reports` - Campaign analytics

---

## Priority 7: Specialized Integrations

### Twilio

**Priority**: Low-Medium
**Category**: Communication
**Complexity**: Medium

**Why Twilio?**
- SMS and voice communications
- Two-factor authentication
- Notification delivery
- Call tracking

**Data to Sync**:
- `twilio_messages` - SMS history
- `twilio_calls` - Call logs
- `twilio_webhooks` - Event log

---

### Algolia

**Priority**: Low
**Category**: Search
**Complexity**: Low

**Why Algolia?**
- Hosted search engine
- Analytics on search queries
- A/B testing for search
- Personalization

**Data to Sync**:
- `algolia_indices` - Index metadata
- `algolia_analytics` - Search analytics
- `algolia_rules` - Search rules

---

### Clerk

**Priority**: Low-Medium
**Category**: Authentication
**Complexity**: Low

**Why Clerk?**
- Modern auth provider
- User management
- Session tracking
- Organization support

**Data to Sync**:
- `clerk_users` - User profiles
- `clerk_organizations` - Org structure
- `clerk_sessions` - Session data
- `clerk_webhooks` - Event log

---

## Implementation Timeline

### Phase 1 (v0.4.8) - Complete
- [x] Stripe - v1.0.0 Released
- [x] Shopify - v1.0.0 Released
- [x] GitHub - v1.0.0 Released

### Phase 2 (v0.4.9)
- [ ] Linear
- [ ] Intercom
- [ ] Resend

### Phase 3 (v0.5.0)
- [ ] Plaid
- [ ] Notion
- [ ] Airtable

### Future
- Remaining plugins based on community feedback

---

## Community Requests

Have a plugin you'd like to see? Open an issue on [GitHub](https://github.com/acamarata/nself-plugins/issues) with:

1. Service name
2. Use case description
3. Key data you'd want synced
4. Webhook events you need

---

## Contributing

Want to build a plugin? See [DEVELOPMENT.md](DEVELOPMENT.md) for the plugin development guide.

Plugin contributions are welcome for:
- New service integrations
- Bug fixes to existing plugins
- Documentation improvements
- Test coverage

---

*Last Updated: January 23, 2026*
