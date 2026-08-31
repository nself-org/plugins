# Analytics Plugin - Quick Start Guide

## Installation

```bash
cd /Users/admin/Sites/nself-plugins/plugins/analytics/ts
npm install
npm run build
```

## Configuration

Create `.env` file:

```bash
cp .env.example .env
```

Edit `.env` with your database credentials:

```env
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=nself
POSTGRES_USER=postgres
POSTGRES_PASSWORD=your_password
ANALYTICS_PLUGIN_PORT=3304
```

## Initialize Database

```bash
node dist/cli.js init
```

Expected output:
```
[analytics:db] INFO: Initializing analytics schema...
[analytics:db] SUCCESS: Analytics schema initialized
[analytics:cli] SUCCESS: Database schema initialized
```

## Start Server

```bash
node dist/cli.js server
```

Expected output:
```
[analytics:server] SUCCESS: Analytics plugin listening on 0.0.0.0:3304
```

## Test the API

### Track an Event

```bash
curl -X POST http://localhost:3304/v1/events \
  -H "Content-Type: application/json" \
  -d '{
    "event_name": "user_signup",
    "user_id": "user123",
    "properties": {
      "plan": "pro",
      "referral": "google"
    }
  }'
```

### Check Counter

```bash
curl "http://localhost:3304/v1/counters?counter_name=user_signup&period=all_time"
```

### View Dashboard

```bash
curl http://localhost:3304/v1/dashboard
```

## Create a Funnel

```bash
curl -X POST http://localhost:3304/v1/funnels \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Signup Funnel",
    "steps": [
      {"name": "Landing", "event_name": "page_view"},
      {"name": "Form", "event_name": "signup_started"},
      {"name": "Complete", "event_name": "user_signup"}
    ],
    "window_hours": 24
  }'
```

Save the returned `funnel_id`, then analyze:

```bash
curl http://localhost:3304/v1/funnels/{funnel_id}/analyze
```

## Create a Quota

```bash
curl -X POST http://localhost:3304/v1/quotas \
  -H "Content-Type: application/json" \
  -d '{
    "name": "API Rate Limit",
    "counter_name": "api_calls",
    "max_value": 1000,
    "period": "hourly",
    "scope": "user",
    "action_on_exceed": "block"
  }'
```

## CLI Usage

### Track Event from CLI

```bash
node dist/cli.js track \
  --name "button_click" \
  --user "user456" \
  --properties '{"button": "cta"}'
```

### View Status

```bash
node dist/cli.js status
```

### View Dashboard

```bash
node dist/cli.js dashboard
```

### List Counters

```bash
node dist/cli.js counters list
```

### Trigger Rollup

```bash
node dist/cli.js rollup
```

## Production Deployment

### Using PM2

```bash
pm2 start dist/cli.js --name analytics -- server --port 3304
pm2 save
```

### Using systemd

Create `/etc/systemd/system/analytics.service`:

```ini
[Unit]
Description=Analytics Plugin
After=network.target postgresql.service

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/nself-plugins/plugins/analytics/ts
Environment="NODE_ENV=production"
EnvironmentFile=/opt/nself-plugins/plugins/analytics/ts/.env
ExecStart=/usr/bin/node dist/cli.js server
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Then:

```bash
sudo systemctl enable analytics
sudo systemctl start analytics
sudo systemctl status analytics
```

## Health Checks

```bash
# Basic health
curl http://localhost:3304/health

# Database connectivity
curl http://localhost:3304/ready

# Detailed status
curl http://localhost:3304/live
```

## Multi-Account Support

Add header to requests:

```bash
curl -X POST http://localhost:3304/v1/events \
  -H "X-Source-Account-Id: my-app" \
  -H "Content-Type: application/json" \
  -d '{"event_name": "test"}'
```

## Common Operations

### Batch Track Events

```bash
curl -X POST http://localhost:3304/v1/events/batch \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      {"event_name": "page_view", "user_id": "user1"},
      {"event_name": "page_view", "user_id": "user2"},
      {"event_name": "button_click", "user_id": "user1"}
    ]
  }'
```

### Get Counter Timeseries

```bash
curl "http://localhost:3304/v1/counters/page_view/timeseries?period=daily&start_date=2024-01-01"
```

### Check Quota Before Action

```bash
curl -X POST http://localhost:3304/v1/quotas/check \
  -H "Content-Type: application/json" \
  -d '{
    "counter_name": "api_calls",
    "scope_id": "user123",
    "increment": 1
  }'
```

If `allowed: false`, don't proceed with action.

## Monitoring

### Prometheus Metrics (via stats endpoint)

```bash
curl http://localhost:3304/v1/status
```

Returns:
```json
{
  "plugin": "analytics",
  "version": "1.0.0",
  "status": "running",
  "stats": {
    "events": 12345,
    "counters": 56,
    "funnels": 3,
    "quotas": 5,
    "violations": 2,
    "lastEventAt": "2024-01-15T10:30:00.000Z"
  }
}
```

## Troubleshooting

### Database Connection Failed

Check PostgreSQL is running:
```bash
pg_isready -h localhost -p 5432
```

Check credentials in `.env`:
```bash
cat .env | grep POSTGRES
```

### Port Already in Use

Check what's using port 3304:
```bash
lsof -i :3304
```

Change port in `.env`:
```env
ANALYTICS_PLUGIN_PORT=3305
```

### TypeScript Errors

Rebuild:
```bash
npm run clean
npm run build
```

### View Logs

Set debug logging:
```env
LOG_LEVEL=debug
```

## Next Steps

1. Read the full [README.md](README.md) for complete API documentation
2. Review [IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md) for technical details
3. Integrate with your application using the REST API or SDK
4. Set up scheduled counter rollups (cron or external scheduler)
5. Configure alerts on quota violations
6. Set up data retention policies

## Support

For issues or questions:
- Check logs with `LOG_LEVEL=debug`
- Review the database schema in `IMPLEMENTATION_SUMMARY.md`
- Inspect the source code in `ts/src/`

## Development

```bash
# Run in watch mode
npm run dev

# Type check without building
npm run typecheck

# Clean build artifacts
npm run clean
```
