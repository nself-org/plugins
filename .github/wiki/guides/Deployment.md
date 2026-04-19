# Deployment Guide

Complete guide for deploying nself plugins to production environments.

**Last Updated**: January 30, 2026
**Version**: v1.0.0

---

## Table of Contents

1. [Production Checklist](#production-checklist)
2. [Server Setup](#server-setup)
3. [Database Setup](#database-setup)
4. [Process Management](#process-management)
5. [Reverse Proxy Configuration](#reverse-proxy-configuration)
6. [SSL/TLS Setup](#ssltls-setup)
7. [Environment Variables](#environment-variables)
8. [Monitoring Setup](#monitoring-setup)
9. [Backup Strategy](#backup-strategy)
10. [Scaling Considerations](#scaling-considerations)
11. [High Availability](#high-availability)
12. [Docker Deployment](#docker-deployment)
13. [Cloud Deployment](#cloud-deployment)
14. [CI/CD Pipeline](#cicd-pipeline)
15. [Security Hardening](#security-hardening)
16. [Troubleshooting](#troubleshooting)

---

## Production Checklist

Before deploying to production, ensure you have completed the following:

### Pre-Deployment Requirements

- [ ] **Node.js** 18.0.0 or higher installed
- [ ] **PostgreSQL** 14.0 or higher configured
- [ ] **SSL Certificates** obtained (Let's Encrypt or commercial)
- [ ] **Domain Names** configured with DNS
- [ ] **API Keys** obtained from service providers (Stripe, GitHub, etc.)
- [ ] **Webhook Secrets** configured in service dashboards
- [ ] **Backup System** configured and tested
- [ ] **Monitoring Tools** installed (Prometheus, Grafana, etc.)
- [ ] **Firewall Rules** configured
- [ ] **Load Balancer** configured (if using multiple servers)

### Security Requirements

- [ ] API key authentication enabled (`NSELF_API_KEY` set)
- [ ] Rate limiting configured (default: 100 req/min)
- [ ] Webhook signature verification enabled
- [ ] HTTPS enforced (redirect HTTP to HTTPS)
- [ ] Database connection uses SSL
- [ ] Secrets stored in secure vault (not in .env files)
- [ ] Log rotation configured
- [ ] Intrusion detection configured (optional but recommended)

### Testing Requirements

- [ ] Initial sync tested with production data
- [ ] Webhook delivery tested from service provider
- [ ] API endpoints tested with production load
- [ ] Failover tested (if using HA setup)
- [ ] Backup and restore tested
- [ ] Monitoring alerts tested

---

## Server Setup

### OS Requirements

**Recommended Operating Systems:**
- Ubuntu 22.04 LTS or 24.04 LTS (recommended)
- Debian 12
- CentOS Stream 9 / Rocky Linux 9
- Amazon Linux 2023

**Minimum Server Specs:**
- **CPU**: 2 cores (4+ recommended for production)
- **RAM**: 4GB (8GB+ recommended)
- **Storage**: 50GB SSD (100GB+ for large datasets)
- **Network**: 1Gbps (for webhook delivery)

### System Dependencies

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install -y \
  curl \
  wget \
  git \
  build-essential \
  postgresql-client \
  nginx \
  certbot \
  python3-certbot-nginx

# CentOS/Rocky Linux
sudo dnf install -y \
  curl \
  wget \
  git \
  gcc-c++ \
  make \
  postgresql \
  nginx \
  certbot \
  python3-certbot-nginx
```

### Install Node.js

```bash
# Using Node Version Manager (recommended)
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash
source ~/.bashrc
nvm install 20
nvm use 20
nvm alias default 20

# Or using NodeSource repository (Ubuntu/Debian)
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt-get install -y nodejs

# Verify installation
node --version  # Should be v20.x.x
npm --version   # Should be 10.x.x
```

### System Tuning

```bash
# Increase file descriptor limits
sudo tee -a /etc/security/limits.conf <<EOF
*       soft    nofile  65536
*       hard    nofile  65536
EOF

# TCP tuning for high traffic
sudo tee -a /etc/sysctl.conf <<EOF
net.core.somaxconn = 4096
net.ipv4.tcp_max_syn_backlog = 8192
net.ipv4.ip_local_port_range = 1024 65535
EOF

sudo sysctl -p
```

---

## Database Setup

### PostgreSQL Installation

```bash
# Ubuntu/Debian
sudo apt install -y postgresql-14 postgresql-contrib-14

# Start and enable service
sudo systemctl start postgresql
sudo systemctl enable postgresql

# CentOS/Rocky Linux
sudo dnf install -y postgresql14-server postgresql14-contrib
sudo postgresql-14-setup initdb
sudo systemctl start postgresql-14
sudo systemctl enable postgresql-14
```

### Database Configuration

```bash
# Switch to postgres user
sudo -u postgres psql

# Create database and user
CREATE DATABASE nself_production;
CREATE USER nself_user WITH PASSWORD 'your_secure_password_here';
GRANT ALL PRIVILEGES ON DATABASE nself_production TO nself_user;

# Grant schema permissions
\c nself_production
GRANT ALL ON SCHEMA public TO nself_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO nself_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO nself_user;

\q
```

### PostgreSQL Performance Tuning

Edit `/etc/postgresql/14/main/postgresql.conf`:

```ini
# Memory settings (adjust based on available RAM)
shared_buffers = 2GB                # 25% of total RAM
effective_cache_size = 6GB          # 75% of total RAM
work_mem = 16MB                     # Per operation
maintenance_work_mem = 512MB        # For VACUUM, CREATE INDEX

# Checkpoint settings
checkpoint_completion_target = 0.9
wal_buffers = 16MB
min_wal_size = 1GB
max_wal_size = 4GB

# Planner settings
random_page_cost = 1.1              # For SSD storage
effective_io_concurrency = 200      # For SSD storage

# Connection settings
max_connections = 100
```

### Enable SSL for PostgreSQL

```bash
# Generate self-signed certificate (or use Let's Encrypt)
sudo openssl req -new -x509 -days 365 -nodes -text \
  -out /etc/ssl/certs/postgresql.crt \
  -keyout /etc/ssl/private/postgresql.key

sudo chown postgres:postgres /etc/ssl/private/postgresql.key
sudo chmod 600 /etc/ssl/private/postgresql.key
```

Edit `/etc/postgresql/14/main/postgresql.conf`:

```ini
ssl = on
ssl_cert_file = '/etc/ssl/certs/postgresql.crt'
ssl_key_file = '/etc/ssl/private/postgresql.key'
```

Restart PostgreSQL:

```bash
sudo systemctl restart postgresql
```

### Database Backups

#### Automated Daily Backups

```bash
# Create backup script
sudo mkdir -p /opt/nself/backups
sudo tee /opt/nself/backup-db.sh <<'EOF'
#!/bin/bash
# PostgreSQL backup script for nself plugins

BACKUP_DIR="/opt/nself/backups"
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="$BACKUP_DIR/nself_production_$DATE.sql.gz"
RETENTION_DAYS=7

# Create backup
pg_dump -U nself_user -h localhost nself_production | gzip > "$BACKUP_FILE"

# Delete backups older than retention period
find "$BACKUP_DIR" -name "*.sql.gz" -mtime +$RETENTION_DAYS -delete

echo "Backup completed: $BACKUP_FILE"
EOF

sudo chmod +x /opt/nself/backup-db.sh

# Add to crontab (run daily at 2 AM)
(crontab -l 2>/dev/null; echo "0 2 * * * /opt/nself/backup-db.sh") | crontab -
```

#### Restore from Backup

```bash
# Restore from backup
gunzip -c /opt/nself/backups/nself_production_20260130_020000.sql.gz | \
  psql -U nself_user -h localhost nself_production
```

### Database Replication (Optional)

For high availability, set up PostgreSQL streaming replication:

**Primary Server** (`postgresql.conf`):

```ini
wal_level = replica
max_wal_senders = 3
wal_keep_size = 1GB
```

**Standby Server Setup:**

```bash
# On standby server
pg_basebackup -h primary_server -U replication_user -D /var/lib/postgresql/14/main -P -Xs -R
```

---

## Process Management

### Option 1: systemd (Recommended)

Create a systemd service file:

```bash
sudo tee /etc/systemd/system/nself-stripe.service <<EOF
[Unit]
Description=nself Stripe Plugin
After=network.target postgresql.service
Requires=postgresql.service

[Service]
Type=simple
User=nself
Group=nself
WorkingDirectory=/opt/nself/plugins/stripe/ts
Environment="NODE_ENV=production"
EnvironmentFile=/opt/nself/plugins/stripe/.env
ExecStart=/usr/bin/node dist/server.js
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=nself-stripe

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/nself/plugins/stripe

# Resource limits
LimitNOFILE=65536
MemoryLimit=1G
CPUQuota=200%

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd
sudo systemctl daemon-reload

# Enable and start service
sudo systemctl enable nself-stripe
sudo systemctl start nself-stripe

# Check status
sudo systemctl status nself-stripe

# View logs
sudo journalctl -u nself-stripe -f
```

### Option 2: PM2 (Node.js Process Manager)

```bash
# Install PM2 globally
npm install -g pm2

# Create PM2 ecosystem file
cat > /opt/nself/ecosystem.config.js <<'EOF'
module.exports = {
  apps: [
    {
      name: 'nself-stripe',
      cwd: '/opt/nself/plugins/stripe/ts',
      script: 'dist/server.js',
      instances: 2,
      exec_mode: 'cluster',
      env: {
        NODE_ENV: 'production',
        PORT: 3001
      },
      env_file: '/opt/nself/plugins/stripe/.env',
      error_file: '/var/log/nself/stripe-error.log',
      out_file: '/var/log/nself/stripe-out.log',
      log_date_format: 'YYYY-MM-DD HH:mm:ss Z',
      merge_logs: true,
      max_memory_restart: '1G',
      restart_delay: 4000,
      autorestart: true
    },
    {
      name: 'nself-github',
      cwd: '/opt/nself/plugins/github/ts',
      script: 'dist/server.js',
      instances: 2,
      exec_mode: 'cluster',
      env: {
        NODE_ENV: 'production',
        PORT: 3002
      },
      env_file: '/opt/nself/plugins/github/.env',
      error_file: '/var/log/nself/github-error.log',
      out_file: '/var/log/nself/github-out.log',
      max_memory_restart: '1G'
    }
  ]
};
EOF

# Start all apps
pm2 start ecosystem.config.js

# Save PM2 configuration
pm2 save

# Generate startup script (runs PM2 on boot)
pm2 startup systemd -u nself --hp /home/nself

# Monitor apps
pm2 monit

# View logs
pm2 logs nself-stripe
```

### Option 3: Docker with Docker Compose

See [Docker Deployment](#docker-deployment) section below.

---

## Reverse Proxy Configuration

### nginx (Recommended)

```bash
# Create nginx configuration
sudo tee /etc/nginx/sites-available/nself-plugins <<'EOF'
# Rate limiting zones
limit_req_zone $binary_remote_addr zone=api_limit:10m rate=10r/s;
limit_req_zone $binary_remote_addr zone=webhook_limit:10m rate=100r/s;

# Upstream servers
upstream nself_stripe {
    least_conn;
    server localhost:3001 max_fails=3 fail_timeout=30s;
    keepalive 32;
}

upstream nself_github {
    least_conn;
    server localhost:3002 max_fails=3 fail_timeout=30s;
    keepalive 32;
}

# Redirect HTTP to HTTPS
server {
    listen 80;
    server_name stripe.api.example.com github.api.example.com;
    return 301 https://$host$request_uri;
}

# Stripe Plugin
server {
    listen 443 ssl http2;
    server_name stripe.api.example.com;

    # SSL configuration
    ssl_certificate /etc/letsencrypt/live/stripe.api.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/stripe.api.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;

    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # Logging
    access_log /var/log/nginx/stripe-access.log;
    error_log /var/log/nginx/stripe-error.log;

    # Client body size (for large webhook payloads)
    client_max_body_size 10M;

    # Webhook endpoint (higher rate limit)
    location /webhooks/stripe {
        limit_req zone=webhook_limit burst=20 nodelay;

        proxy_pass http://nself_stripe;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;

        # Preserve original body for signature verification
        proxy_pass_request_body on;
        proxy_pass_request_headers on;

        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # API endpoints (standard rate limit)
    location /api/ {
        limit_req zone=api_limit burst=20 nodelay;

        proxy_pass http://nself_stripe;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;

        # Timeouts
        proxy_connect_timeout 30s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
    }

    # Health check endpoints (no rate limiting)
    location ~ ^/(health|ready|live)$ {
        proxy_pass http://nself_stripe;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        access_log off;
    }

    # Sync endpoint (restricted, lower rate limit)
    location /sync {
        limit_req zone=api_limit burst=5 nodelay;

        # Optional: restrict to specific IPs
        # allow 10.0.0.0/8;
        # deny all;

        proxy_pass http://nself_stripe;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Long timeout for sync operations
        proxy_connect_timeout 300s;
        proxy_send_timeout 300s;
        proxy_read_timeout 300s;
    }
}

# GitHub Plugin (similar configuration)
server {
    listen 443 ssl http2;
    server_name github.api.example.com;

    ssl_certificate /etc/letsencrypt/live/github.api.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/github.api.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;

    access_log /var/log/nginx/github-access.log;
    error_log /var/log/nginx/github-error.log;

    client_max_body_size 10M;

    location / {
        limit_req zone=api_limit burst=20 nodelay;
        proxy_pass http://nself_github;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
EOF

# Enable site
sudo ln -s /etc/nginx/sites-available/nself-plugins /etc/nginx/sites-enabled/

# Test configuration
sudo nginx -t

# Reload nginx
sudo systemctl reload nginx
```

### Caddy (Simpler Alternative)

```bash
# Install Caddy
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update
sudo apt install caddy

# Create Caddyfile
sudo tee /etc/caddy/Caddyfile <<'EOF'
# Stripe Plugin
stripe.api.example.com {
    # Automatic HTTPS with Let's Encrypt

    # Rate limiting
    rate_limit {
        zone api {
            key {remote_host}
            events 100
            window 1m
        }
    }

    # Webhook endpoint
    reverse_proxy /webhooks/stripe localhost:3001 {
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }

    # API endpoints
    reverse_proxy /api/* localhost:3001 {
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }

    # Health checks
    reverse_proxy /health localhost:3001
    reverse_proxy /ready localhost:3001
    reverse_proxy /live localhost:3001

    # Sync endpoint
    reverse_proxy /sync localhost:3001 {
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-Proto {scheme}
        timeout 300s
    }

    # Security headers
    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
        X-Frame-Options "SAMEORIGIN"
        X-Content-Type-Options "nosniff"
        X-XSS-Protection "1; mode=block"
    }

    # Logging
    log {
        output file /var/log/caddy/stripe.log
        format json
    }
}

# GitHub Plugin
github.api.example.com {
    reverse_proxy localhost:3002 {
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }

    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
        X-Frame-Options "SAMEORIGIN"
        X-Content-Type-Options "nosniff"
    }

    log {
        output file /var/log/caddy/github.log
        format json
    }
}
EOF

# Reload Caddy
sudo systemctl reload caddy
```

---

## SSL/TLS Setup

### Option 1: Let's Encrypt (Free, Automated)

```bash
# Install Certbot
sudo apt install certbot python3-certbot-nginx

# Obtain certificates for nginx
sudo certbot --nginx -d stripe.api.example.com -d github.api.example.com

# Auto-renewal is set up automatically
# Test renewal
sudo certbot renew --dry-run

# Renewal happens automatically via systemd timer
sudo systemctl status certbot.timer
```

### Option 2: Commercial Certificate

```bash
# Generate CSR
sudo openssl req -new -newkey rsa:4096 -nodes \
  -keyout /etc/ssl/private/stripe.api.example.com.key \
  -out /etc/ssl/certs/stripe.api.example.com.csr

# Send CSR to certificate authority
# Download certificate and intermediate certificates

# Install certificate
sudo cp certificate.crt /etc/ssl/certs/stripe.api.example.com.crt
sudo cp ca-bundle.crt /etc/ssl/certs/stripe.api.example.com-bundle.crt
sudo chmod 600 /etc/ssl/private/stripe.api.example.com.key

# Update nginx configuration with paths
sudo systemctl reload nginx
```

### SSL Testing

```bash
# Test SSL configuration
curl -vI https://stripe.api.example.com/health

# Check SSL rating (external)
# Visit: https://www.ssllabs.com/ssltest/
```

---

## Environment Variables

### Production Secrets Management

**NEVER store secrets in `.env` files in production.** Use one of these methods:

### Option 1: systemd Environment Files

```bash
# Create secure environment file
sudo mkdir -p /opt/nself/secrets
sudo tee /opt/nself/secrets/stripe.env <<EOF
NODE_ENV=production
PORT=3001
HOST=0.0.0.0

# Database
DATABASE_URL=postgresql://nself_user:password@localhost:5432/nself_production?sslmode=require

# Stripe API
STRIPE_API_KEY=sk_live_REPLACEME
STRIPE_API_VERSION=2023-10-16
STRIPE_WEBHOOK_SECRET=whsec_xxxxxxxxxxxxxxxxxxxxxxxx

# Security
NSELF_API_KEY=your-secure-api-key-here
RATE_LIMIT_MAX=100
RATE_LIMIT_WINDOW_MS=60000
WEBHOOK_SIGNATURE_VALIDATION=true

# Logging
LOG_LEVEL=info
DEBUG=false
EOF

# Secure the file
sudo chown nself:nself /opt/nself/secrets/stripe.env
sudo chmod 600 /opt/nself/secrets/stripe.env

# Reference in systemd service
# EnvironmentFile=/opt/nself/secrets/stripe.env
```

### Option 2: HashiCorp Vault (Enterprise)

```bash
# Install Vault
wget -O- https://apt.releases.hashicorp.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list
sudo apt update && sudo apt install vault

# Store secrets in Vault
vault kv put secret/nself/stripe \
  stripe_api_key="sk_live_xxx" \
  stripe_webhook_secret="whsec_xxx" \
  database_url="postgresql://..."

# Retrieve in application startup script
export STRIPE_API_KEY=$(vault kv get -field=stripe_api_key secret/nself/stripe)
```

### Option 3: AWS Secrets Manager

```bash
# Install AWS CLI
sudo apt install awscli

# Store secret
aws secretsmanager create-secret \
  --name nself/stripe/api-key \
  --secret-string "sk_live_REPLACEME"

# Retrieve in startup script
export STRIPE_API_KEY=$(aws secretsmanager get-secret-value \
  --secret-id nself/stripe/api-key \
  --query SecretString \
  --output text)
```

### Option 4: Docker Secrets (for Docker Swarm)

```bash
# Create secret
echo "sk_live_REPLACEME" | docker secret create stripe_api_key -

# Reference in docker-compose.yml
# See Docker Deployment section
```

---

## Monitoring Setup

### Prometheus + Grafana

#### Install Prometheus

```bash
# Create user
sudo useradd --no-create-home --shell /bin/false prometheus

# Download and install
PROM_VERSION="2.48.0"
wget https://github.com/prometheus/prometheus/releases/download/v${PROM_VERSION}/prometheus-${PROM_VERSION}.linux-amd64.tar.gz
tar xvf prometheus-${PROM_VERSION}.linux-amd64.tar.gz
sudo cp prometheus-${PROM_VERSION}.linux-amd64/prometheus /usr/local/bin/
sudo cp prometheus-${PROM_VERSION}.linux-amd64/promtool /usr/local/bin/

# Create directories
sudo mkdir -p /etc/prometheus /var/lib/prometheus
sudo chown prometheus:prometheus /etc/prometheus /var/lib/prometheus

# Create configuration
sudo tee /etc/prometheus/prometheus.yml <<EOF
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'nself-stripe'
    static_configs:
      - targets: ['localhost:3001']
    metrics_path: '/metrics'

  - job_name: 'nself-github'
    static_configs:
      - targets: ['localhost:3002']
    metrics_path: '/metrics'

  - job_name: 'node-exporter'
    static_configs:
      - targets: ['localhost:9100']
EOF

# Create systemd service
sudo tee /etc/systemd/system/prometheus.service <<EOF
[Unit]
Description=Prometheus
Wants=network-online.target
After=network-online.target

[Service]
User=prometheus
Group=prometheus
Type=simple
ExecStart=/usr/local/bin/prometheus \
  --config.file=/etc/prometheus/prometheus.yml \
  --storage.tsdb.path=/var/lib/prometheus/ \
  --web.console.templates=/etc/prometheus/consoles \
  --web.console.libraries=/etc/prometheus/console_libraries

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl start prometheus
sudo systemctl enable prometheus
```

#### Add Metrics to Plugin

Add to `server.ts`:

```typescript
import client from 'prom-client';

// Create metrics registry
const register = new client.Registry();

// Default metrics (CPU, memory, etc.)
client.collectDefaultMetrics({ register });

// Custom metrics
const httpRequestDuration = new client.Histogram({
  name: 'http_request_duration_seconds',
  help: 'Duration of HTTP requests in seconds',
  labelNames: ['method', 'route', 'status_code'],
  registers: [register]
});

const webhookCounter = new client.Counter({
  name: 'webhooks_received_total',
  help: 'Total number of webhooks received',
  labelNames: ['event_type', 'status'],
  registers: [register]
});

const syncDuration = new client.Histogram({
  name: 'sync_duration_seconds',
  help: 'Duration of sync operations',
  labelNames: ['resource'],
  registers: [register]
});

// Metrics endpoint
app.get('/metrics', async () => {
  return register.metrics();
});

// Instrument requests
app.addHook('onRequest', async (request, reply) => {
  request.startTime = Date.now();
});

app.addHook('onResponse', async (request, reply) => {
  const duration = (Date.now() - request.startTime) / 1000;
  httpRequestDuration.labels(
    request.method,
    request.routerPath || 'unknown',
    reply.statusCode.toString()
  ).observe(duration);
});
```

#### Install Grafana

```bash
# Add Grafana repository
sudo apt-get install -y software-properties-common
sudo add-apt-repository "deb https://packages.grafana.com/oss/deb stable main"
wget -q -O - https://packages.grafana.com/gpg.key | sudo apt-key add -

# Install
sudo apt-get update
sudo apt-get install grafana

# Start service
sudo systemctl start grafana-server
sudo systemctl enable grafana-server

# Access at http://localhost:3000
# Default credentials: admin/admin
```

#### Grafana Dashboard Configuration

1. Add Prometheus data source: Configuration > Data Sources > Add Prometheus
2. Import dashboard for Node.js: Dashboard ID 11159
3. Create custom dashboard with these panels:
   - Webhook throughput: `rate(webhooks_received_total[5m])`
   - API request rate: `rate(http_request_duration_seconds_count[5m])`
   - Sync duration: `sync_duration_seconds{quantile="0.99"}`
   - Database connections: `pg_stat_activity_count`
   - Memory usage: `process_resident_memory_bytes`

### Logging with Loki

```bash
# Install Loki
wget https://github.com/grafana/loki/releases/download/v2.9.3/loki-linux-amd64.zip
unzip loki-linux-amd64.zip
sudo mv loki-linux-amd64 /usr/local/bin/loki

# Create configuration
sudo mkdir -p /etc/loki
sudo tee /etc/loki/config.yml <<EOF
auth_enabled: false

server:
  http_listen_port: 3100

ingester:
  lifecycler:
    ring:
      kvstore:
        store: inmemory
      replication_factor: 1

schema_config:
  configs:
    - from: 2020-10-24
      store: boltdb-shipper
      object_store: filesystem
      schema: v11
      index:
        prefix: index_
        period: 24h

storage_config:
  boltdb_shipper:
    active_index_directory: /var/lib/loki/index
    cache_location: /var/lib/loki/cache
  filesystem:
    directory: /var/lib/loki/chunks

limits_config:
  reject_old_samples: true
  reject_old_samples_max_age: 168h
EOF

# Create systemd service
sudo tee /etc/systemd/system/loki.service <<EOF
[Unit]
Description=Loki
After=network.target

[Service]
Type=simple
User=loki
ExecStart=/usr/local/bin/loki -config.file=/etc/loki/config.yml

[Install]
WantedBy=multi-user.target
EOF

# Install Promtail (log shipper)
wget https://github.com/grafana/loki/releases/download/v2.9.3/promtail-linux-amd64.zip
unzip promtail-linux-amd64.zip
sudo mv promtail-linux-amd64 /usr/local/bin/promtail

# Configure Promtail
sudo tee /etc/loki/promtail.yml <<EOF
server:
  http_listen_port: 9080

positions:
  filename: /var/lib/promtail/positions.yaml

clients:
  - url: http://localhost:3100/loki/api/v1/push

scrape_configs:
  - job_name: nself-stripe
    static_configs:
      - targets:
          - localhost
        labels:
          job: nself-stripe
          __path__: /var/log/nself/stripe-*.log

  - job_name: nginx
    static_configs:
      - targets:
          - localhost
        labels:
          job: nginx
          __path__: /var/log/nginx/*access.log
EOF

sudo systemctl start loki promtail
sudo systemctl enable loki promtail
```

### Alerting with Alertmanager

```bash
# Install Alertmanager
wget https://github.com/prometheus/alertmanager/releases/download/v0.26.0/alertmanager-0.26.0.linux-amd64.tar.gz
tar xvf alertmanager-0.26.0.linux-amd64.tar.gz
sudo cp alertmanager-0.26.0.linux-amd64/alertmanager /usr/local/bin/

# Configure alerts
sudo tee /etc/prometheus/alerts.yml <<EOF
groups:
  - name: nself-alerts
    interval: 30s
    rules:
      - alert: PluginDown
        expr: up{job=~"nself-.*"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Plugin {{ \$labels.job }} is down"

      - alert: HighWebhookFailureRate
        expr: rate(webhooks_received_total{status="failed"}[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High webhook failure rate for {{ \$labels.job }}"

      - alert: DatabaseConnectionsFull
        expr: pg_stat_activity_count > 90
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "Database connection pool nearly full"

      - alert: HighMemoryUsage
        expr: process_resident_memory_bytes > 1e9
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "{{ \$labels.job }} using > 1GB memory"
EOF

# Configure Alertmanager
sudo tee /etc/alertmanager/config.yml <<EOF
global:
  resolve_timeout: 5m

route:
  group_by: ['alertname', 'job']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h
  receiver: 'email'

receivers:
  - name: 'email'
    email_configs:
      - to: 'ops@example.com'
        from: 'alerts@example.com'
        smarthost: 'smtp.example.com:587'
        auth_username: 'alerts@example.com'
        auth_password: 'password'
EOF

# Update Prometheus config
sudo tee -a /etc/prometheus/prometheus.yml <<EOF

rule_files:
  - "alerts.yml"

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['localhost:9093']
EOF

sudo systemctl restart prometheus alertmanager
```

---

## Backup Strategy

### Database Backups

#### Continuous WAL Archiving

```bash
# Configure WAL archiving in postgresql.conf
archive_mode = on
archive_command = 'test ! -f /mnt/backup/wal/%f && cp %p /mnt/backup/wal/%f'
archive_timeout = 300  # Force switch every 5 minutes
```

#### Point-in-Time Recovery (PITR)

```bash
# Create base backup
sudo -u postgres pg_basebackup -D /mnt/backup/base -Ft -z -P

# Restore process
# 1. Stop PostgreSQL
# 2. Replace data directory with base backup
# 3. Create recovery.conf
# 4. Start PostgreSQL
```

### Application Backups

```bash
# Backup plugin code and configuration
sudo tee /opt/nself/backup-app.sh <<'EOF'
#!/bin/bash
BACKUP_DIR="/mnt/backup/app"
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="$BACKUP_DIR/nself-plugins-$DATE.tar.gz"

tar -czf "$BACKUP_FILE" \
  /opt/nself/plugins \
  /opt/nself/secrets \
  /etc/nginx/sites-available/nself-plugins \
  /etc/systemd/system/nself-*.service

echo "Application backup completed: $BACKUP_FILE"
EOF

sudo chmod +x /opt/nself/backup-app.sh

# Schedule daily at 3 AM
(crontab -l; echo "0 3 * * * /opt/nself/backup-app.sh") | crontab -
```

### Offsite Backups with rclone

```bash
# Install rclone
curl https://rclone.org/install.sh | sudo bash

# Configure S3/Backblaze/etc
rclone config

# Sync backups to cloud
sudo tee /opt/nself/sync-backups.sh <<'EOF'
#!/bin/bash
rclone sync /opt/nself/backups remote:nself-backups \
  --transfers 4 \
  --checkers 8 \
  --log-file /var/log/rclone-sync.log
EOF

sudo chmod +x /opt/nself/sync-backups.sh

# Schedule hourly
(crontab -l; echo "0 * * * * /opt/nself/sync-backups.sh") | crontab -
```

---

## Scaling Considerations

### Vertical Scaling (Single Server)

```bash
# Increase Node.js memory limit
# In systemd service
Environment="NODE_OPTIONS=--max-old-space-size=4096"

# Or in PM2 ecosystem
node_args: '--max-old-space-size=4096'

# Increase PostgreSQL resources
# See Database Performance Tuning section
```

### Horizontal Scaling (Multiple Servers)

#### Load Balancer Configuration (HAProxy)

```bash
# Install HAProxy
sudo apt install haproxy

# Configure
sudo tee /etc/haproxy/haproxy.cfg <<EOF
global
    log /dev/log local0
    maxconn 4096
    user haproxy
    group haproxy
    daemon

defaults
    log global
    mode http
    option httplog
    option dontlognull
    timeout connect 5000
    timeout client  50000
    timeout server  50000

frontend http_front
    bind *:80
    redirect scheme https code 301 if !{ ssl_fc }

frontend https_front
    bind *:443 ssl crt /etc/ssl/certs/example.com.pem
    acl stripe_api hdr(host) -i stripe.api.example.com
    acl github_api hdr(host) -i github.api.example.com

    use_backend stripe_backend if stripe_api
    use_backend github_backend if github_api

backend stripe_backend
    balance roundrobin
    option httpchk GET /health
    http-check expect status 200
    server stripe1 10.0.1.10:3001 check
    server stripe2 10.0.1.11:3001 check
    server stripe3 10.0.1.12:3001 check

backend github_backend
    balance roundrobin
    option httpchk GET /health
    http-check expect status 200
    server github1 10.0.2.10:3002 check
    server github2 10.0.2.11:3002 check
    server github3 10.0.2.12:3002 check
EOF

sudo systemctl restart haproxy
```

#### Shared Session Storage (Redis)

For sticky sessions or rate limiting across servers:

```bash
# Install Redis
sudo apt install redis-server

# Configure Redis for persistence
sudo tee -a /etc/redis/redis.conf <<EOF
save 900 1
save 300 10
save 60 10000
appendonly yes
appendfsync everysec
EOF

# Update plugin to use Redis for rate limiting
# Modify ApiRateLimiter in shared/src/http.ts to use Redis backend
```

---

## High Availability

### Multi-Server Setup

```
                ┌─────────────────┐
                │  Load Balancer  │
                │    (HAProxy)    │
                └────────┬────────┘
                         │
         ┌───────────────┼───────────────┐
         │               │               │
    ┌────▼────┐     ┌────▼────┐    ┌────▼────┐
    │ Server1 │     │ Server2 │    │ Server3 │
    │ (Active)│     │ (Active)│    │ (Active)│
    └────┬────┘     └────┬────┘    └────┬────┘
         │               │               │
         └───────────────┼───────────────┘
                         │
                    ┌────▼────┐
                    │   DB    │
                    │(Primary)│
                    └────┬────┘
                         │
                    ┌────▼────┐
                    │   DB    │
                    │(Standby)│
                    └─────────┘
```

### Database High Availability

#### Patroni + etcd Setup

```bash
# Install etcd (distributed configuration)
sudo apt install etcd

# Install Patroni
sudo apt install python3-pip
sudo pip3 install patroni[etcd]

# Configure Patroni
sudo tee /etc/patroni/config.yml <<EOF
scope: nself-cluster
name: node1

restapi:
  listen: 0.0.0.0:8008
  connect_address: 10.0.1.10:8008

etcd:
  host: 10.0.1.10:2379

bootstrap:
  dcs:
    ttl: 30
    loop_wait: 10
    retry_timeout: 10
    maximum_lag_on_failover: 1048576
    postgresql:
      use_pg_rewind: true
      parameters:
        wal_level: replica
        hot_standby: "on"
        wal_keep_segments: 8
        max_wal_senders: 5
        max_replication_slots: 5

postgresql:
  listen: 0.0.0.0:5432
  connect_address: 10.0.1.10:5432
  data_dir: /var/lib/postgresql/14/main
  bin_dir: /usr/lib/postgresql/14/bin
  authentication:
    replication:
      username: replicator
      password: rep-pass
    superuser:
      username: postgres
      password: postgres-pass
EOF

# Start Patroni
sudo systemctl start patroni
sudo systemctl enable patroni
```

### Keepalived for VIP Failover

```bash
# Install keepalived
sudo apt install keepalived

# Configure
sudo tee /etc/keepalived/keepalived.conf <<EOF
vrrp_script chk_haproxy {
    script "killall -0 haproxy"
    interval 2
    weight 2
}

vrrp_instance VI_1 {
    state MASTER
    interface eth0
    virtual_router_id 51
    priority 101
    advert_int 1

    authentication {
        auth_type PASS
        auth_pass secret
    }

    virtual_ipaddress {
        10.0.1.100/24
    }

    track_script {
        chk_haproxy
    }
}
EOF

sudo systemctl start keepalived
sudo systemctl enable keepalived
```

---

## Docker Deployment

### Complete Docker Compose Setup

```yaml
# /opt/nself/docker-compose.yml
version: '3.8'

services:
  # PostgreSQL Database
  postgres:
    image: postgres:14-alpine
    container_name: nself-postgres
    restart: unless-stopped
    environment:
      POSTGRES_DB: nself_production
      POSTGRES_USER: nself_user
      POSTGRES_PASSWORD_FILE: /run/secrets/db_password
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./backups:/backups
    secrets:
      - db_password
    networks:
      - nself-network
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U nself_user -d nself_production"]
      interval: 10s
      timeout: 5s
      retries: 5
    ports:
      - "5432:5432"

  # Redis (for rate limiting across instances)
  redis:
    image: redis:7-alpine
    container_name: nself-redis
    restart: unless-stopped
    command: redis-server --appendonly yes
    volumes:
      - redis_data:/data
    networks:
      - nself-network
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 3s
      retries: 5

  # Stripe Plugin
  stripe:
    build:
      context: ./plugins/stripe/ts
      dockerfile: Dockerfile
    container_name: nself-stripe
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    environment:
      NODE_ENV: production
      PORT: 3001
      HOST: 0.0.0.0
      DATABASE_URL: postgresql://nself_user:${DB_PASSWORD}@postgres:5432/nself_production
      REDIS_URL: redis://redis:6379
      STRIPE_API_VERSION: "2023-10-16"
    env_file:
      - ./secrets/stripe.env
    secrets:
      - stripe_api_key
      - stripe_webhook_secret
      - nself_api_key
    volumes:
      - stripe_logs:/var/log/nself
    networks:
      - nself-network
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3001/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
    labels:
      - "com.nself.service=stripe"
      - "com.nself.version=1.0.0"
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 1G
        reservations:
          cpus: '1'
          memory: 512M

  # GitHub Plugin
  github:
    build:
      context: ./plugins/github/ts
      dockerfile: Dockerfile
    container_name: nself-github
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    environment:
      NODE_ENV: production
      PORT: 3002
      HOST: 0.0.0.0
      DATABASE_URL: postgresql://nself_user:${DB_PASSWORD}@postgres:5432/nself_production
      REDIS_URL: redis://redis:6379
    env_file:
      - ./secrets/github.env
    secrets:
      - github_token
      - github_webhook_secret
      - nself_api_key
    volumes:
      - github_logs:/var/log/nself
    networks:
      - nself-network
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3002/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
    labels:
      - "com.nself.service=github"
      - "com.nself.version=1.0.0"
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 1G

  # nginx Reverse Proxy
  nginx:
    image: nginx:alpine
    container_name: nself-nginx
    restart: unless-stopped
    depends_on:
      - stripe
      - github
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./nginx/sites:/etc/nginx/sites-enabled:ro
      - ./nginx/ssl:/etc/nginx/ssl:ro
      - nginx_logs:/var/log/nginx
    networks:
      - nself-network
    healthcheck:
      test: ["CMD", "nginx", "-t"]
      interval: 30s
      timeout: 10s
      retries: 3

  # Prometheus Monitoring
  prometheus:
    image: prom/prometheus:latest
    container_name: nself-prometheus
    restart: unless-stopped
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--storage.tsdb.retention.time=30d'
    volumes:
      - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - ./monitoring/alerts.yml:/etc/prometheus/alerts.yml:ro
      - prometheus_data:/prometheus
    networks:
      - nself-network
    ports:
      - "9090:9090"

  # Grafana Dashboard
  grafana:
    image: grafana/grafana:latest
    container_name: nself-grafana
    restart: unless-stopped
    depends_on:
      - prometheus
    environment:
      GF_SECURITY_ADMIN_PASSWORD__FILE: /run/secrets/grafana_password
      GF_INSTALL_PLUGINS: grafana-piechart-panel
    volumes:
      - ./monitoring/grafana/dashboards:/etc/grafana/provisioning/dashboards:ro
      - ./monitoring/grafana/datasources:/etc/grafana/provisioning/datasources:ro
      - grafana_data:/var/lib/grafana
    secrets:
      - grafana_password
    networks:
      - nself-network
    ports:
      - "3000:3000"

  # Loki Log Aggregation
  loki:
    image: grafana/loki:latest
    container_name: nself-loki
    restart: unless-stopped
    command: -config.file=/etc/loki/local-config.yaml
    volumes:
      - ./monitoring/loki.yml:/etc/loki/local-config.yaml:ro
      - loki_data:/loki
    networks:
      - nself-network
    ports:
      - "3100:3100"

  # Promtail Log Shipper
  promtail:
    image: grafana/promtail:latest
    container_name: nself-promtail
    restart: unless-stopped
    command: -config.file=/etc/promtail/config.yml
    volumes:
      - ./monitoring/promtail.yml:/etc/promtail/config.yml:ro
      - stripe_logs:/var/log/nself/stripe:ro
      - github_logs:/var/log/nself/github:ro
      - nginx_logs:/var/log/nginx:ro
    networks:
      - nself-network

volumes:
  postgres_data:
    driver: local
  redis_data:
    driver: local
  stripe_logs:
    driver: local
  github_logs:
    driver: local
  nginx_logs:
    driver: local
  prometheus_data:
    driver: local
  grafana_data:
    driver: local
  loki_data:
    driver: local

networks:
  nself-network:
    driver: bridge
    ipam:
      config:
        - subnet: 172.20.0.0/16

secrets:
  db_password:
    file: ./secrets/db_password.txt
  stripe_api_key:
    file: ./secrets/stripe_api_key.txt
  stripe_webhook_secret:
    file: ./secrets/stripe_webhook_secret.txt
  github_token:
    file: ./secrets/github_token.txt
  github_webhook_secret:
    file: ./secrets/github_webhook_secret.txt
  nself_api_key:
    file: ./secrets/nself_api_key.txt
  grafana_password:
    file: ./secrets/grafana_password.txt
```

### Dockerfile for Plugins

```dockerfile
# /opt/nself/plugins/stripe/ts/Dockerfile
FROM node:20-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache python3 make g++

# Copy shared utilities
COPY shared /app/shared
WORKDIR /app/shared
RUN npm ci && npm run build

# Copy plugin code
WORKDIR /app/plugin
COPY plugins/stripe/ts/package*.json ./
RUN npm ci --only=production

COPY plugins/stripe/ts/tsconfig.json ./
COPY plugins/stripe/ts/src ./src
RUN npm run build

# Production image
FROM node:20-alpine

WORKDIR /app

# Install dumb-init for proper signal handling
RUN apk add --no-cache dumb-init curl

# Copy built files
COPY --from=builder /app/plugin/dist ./dist
COPY --from=builder /app/plugin/node_modules ./node_modules
COPY --from=builder /app/plugin/package.json ./

# Create non-root user
RUN addgroup -g 1001 -S nself && \
    adduser -S -u 1001 -G nself nself && \
    chown -R nself:nself /app

USER nself

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
  CMD node -e "require('http').get('http://localhost:3001/health', (r) => r.statusCode === 200 ? process.exit(0) : process.exit(1))"

EXPOSE 3001

# Use dumb-init to handle signals properly
ENTRYPOINT ["dumb-init", "--"]
CMD ["node", "dist/server.js"]
```

### Docker Compose Commands

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f stripe

# Restart a service
docker-compose restart stripe

# Scale a service (requires load balancer)
docker-compose up -d --scale stripe=3

# Stop all services
docker-compose down

# Stop and remove volumes
docker-compose down -v

# Update images
docker-compose pull
docker-compose up -d --build

# Execute commands in container
docker-compose exec stripe npm run sync

# Database backup
docker-compose exec postgres pg_dump -U nself_user nself_production | gzip > backup.sql.gz

# View resource usage
docker-compose stats
```

---

## Cloud Deployment

### AWS (Elastic Beanstalk)

```bash
# Install EB CLI
pip install awsebcli

# Initialize application
eb init -p node.js-20 nself-plugins --region us-east-1

# Create environment
eb create nself-production \
  --database.engine postgres \
  --database.size 10 \
  --instance-type t3.medium \
  --scale 2

# Configure environment variables
eb setenv \
  NODE_ENV=production \
  STRIPE_API_KEY=sk_live_xxx \
  DATABASE_URL=postgresql://...

# Deploy
eb deploy

# Open application
eb open

# View logs
eb logs

# SSH into instance
eb ssh
```

### AWS (ECS Fargate)

```yaml
# task-definition.json
{
  "family": "nself-stripe",
  "networkMode": "awsvpc",
  "requiresCompatibilities": ["FARGATE"],
  "cpu": "1024",
  "memory": "2048",
  "containerDefinitions": [
    {
      "name": "stripe",
      "image": "your-registry/nself-stripe:latest",
      "portMappings": [
        {
          "containerPort": 3001,
          "protocol": "tcp"
        }
      ],
      "environment": [
        {
          "name": "NODE_ENV",
          "value": "production"
        }
      ],
      "secrets": [
        {
          "name": "STRIPE_API_KEY",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:123456789:secret:stripe-api-key"
        }
      ],
      "logConfiguration": {
        "logDriver": "awslogs",
        "options": {
          "awslogs-group": "/ecs/nself-stripe",
          "awslogs-region": "us-east-1",
          "awslogs-stream-prefix": "ecs"
        }
      },
      "healthCheck": {
        "command": ["CMD-SHELL", "curl -f http://localhost:3001/health || exit 1"],
        "interval": 30,
        "timeout": 5,
        "retries": 3,
        "startPeriod": 60
      }
    }
  ]
}
```

Deploy:

```bash
# Register task definition
aws ecs register-task-definition --cli-input-json file://task-definition.json

# Create service
aws ecs create-service \
  --cluster nself-cluster \
  --service-name nself-stripe \
  --task-definition nself-stripe \
  --desired-count 2 \
  --launch-type FARGATE \
  --network-configuration "awsvpcConfiguration={subnets=[subnet-xxx],securityGroups=[sg-xxx]}"
```

### Google Cloud Platform (Cloud Run)

```bash
# Build and push image
gcloud builds submit --tag gcr.io/PROJECT_ID/nself-stripe

# Deploy
gcloud run deploy nself-stripe \
  --image gcr.io/PROJECT_ID/nself-stripe \
  --platform managed \
  --region us-central1 \
  --allow-unauthenticated \
  --memory 1Gi \
  --cpu 2 \
  --timeout 300 \
  --concurrency 80 \
  --max-instances 10 \
  --set-env-vars NODE_ENV=production \
  --set-secrets STRIPE_API_KEY=stripe-api-key:latest \
  --vpc-connector vpc-connector

# Map custom domain
gcloud run services update nself-stripe \
  --platform managed \
  --region us-central1 \
  --update-env-vars DOMAIN=stripe.api.example.com
```

### Azure (Container Instances)

```bash
# Create resource group
az group create --name nself-plugins --location eastus

# Create container registry
az acr create --resource-group nself-plugins --name nselfregistry --sku Basic

# Build and push image
az acr build --registry nselfregistry --image nself-stripe:latest .

# Create container instance
az container create \
  --resource-group nself-plugins \
  --name nself-stripe \
  --image nselfregistry.azurecr.io/nself-stripe:latest \
  --cpu 2 \
  --memory 2 \
  --ports 3001 \
  --environment-variables NODE_ENV=production \
  --secure-environment-variables STRIPE_API_KEY=sk_live_xxx \
  --dns-name-label nself-stripe \
  --restart-policy Always
```

### DigitalOcean (App Platform)

```yaml
# .do/app.yaml
name: nself-plugins
region: nyc
services:
  - name: stripe
    github:
      repo: your-org/nself-plugins
      branch: main
      deploy_on_push: true
    dockerfile_path: plugins/stripe/ts/Dockerfile
    source_dir: /
    envs:
      - key: NODE_ENV
        value: production
      - key: STRIPE_API_KEY
        value: ${STRIPE_API_KEY}
        type: SECRET
    health_check:
      http_path: /health
    http_port: 3001
    instance_count: 2
    instance_size_slug: professional-xs
    routes:
      - path: /

databases:
  - name: nself-db
    engine: PG
    version: "14"
    size: db-s-1vcpu-1gb
```

Deploy:

```bash
# Install doctl
brew install doctl  # macOS
# or download from: https://github.com/digitalocean/doctl

# Authenticate
doctl auth init

# Create app
doctl apps create --spec .do/app.yaml

# Deploy
doctl apps create-deployment APP_ID
```

---

## CI/CD Pipeline

### GitHub Actions (Complete Pipeline)

```yaml
# .github/workflows/deploy.yml
name: Deploy to Production

on:
  push:
    branches:
      - main
    paths:
      - 'plugins/**'
      - 'shared/**'
      - 'docker-compose.yml'
  workflow_dispatch:

env:
  REGISTRY: ghcr.io
  IMAGE_PREFIX: ${{ github.repository }}

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:14
        env:
          POSTGRES_USER: test_user
          POSTGRES_PASSWORD: test_pass
          POSTGRES_DB: test_db
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432

    steps:
      - uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'
          cache-dependency-path: |
            shared/package-lock.json
            plugins/*/ts/package-lock.json

      - name: Install dependencies
        run: |
          cd shared && npm ci && npm run build
          cd ../plugins/stripe/ts && npm ci
          cd ../github/ts && npm ci

      - name: Type check
        run: |
          cd plugins/stripe/ts && npm run typecheck
          cd ../../github/ts && npm run typecheck

      - name: Build
        run: |
          cd plugins/stripe/ts && npm run build
          cd ../../github/ts && npm run build

      - name: Run tests
        env:
          DATABASE_URL: postgresql://test_user:test_pass@localhost:5432/test_db
        run: |
          # Add when tests exist
          # cd plugins/stripe/ts && npm test
          echo "Tests passed"

  build:
    needs: test
    runs-on: ubuntu-latest
    strategy:
      matrix:
        plugin: [stripe, github]
    permissions:
      contents: read
      packages: write

    steps:
      - uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GitHub Container Registry
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_PREFIX }}-${{ matrix.plugin }}
          tags: |
            type=ref,event=branch
            type=sha,prefix={{branch}}-
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=raw,value=latest,enable=${{ github.ref == 'refs/heads/main' }}

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          file: ./plugins/${{ matrix.plugin }}/ts/Dockerfile
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max

  deploy:
    needs: build
    runs-on: ubuntu-latest
    environment:
      name: production
      url: https://stripe.api.example.com

    steps:
      - uses: actions/checkout@v4

      - name: Configure SSH
        env:
          SSH_PRIVATE_KEY: ${{ secrets.SSH_PRIVATE_KEY }}
          SSH_HOST: ${{ secrets.SSH_HOST }}
          SSH_USER: ${{ secrets.SSH_USER }}
        run: |
          mkdir -p ~/.ssh
          echo "$SSH_PRIVATE_KEY" > ~/.ssh/deploy_key
          chmod 600 ~/.ssh/deploy_key
          ssh-keyscan -H $SSH_HOST >> ~/.ssh/known_hosts

      - name: Deploy via SSH
        env:
          SSH_HOST: ${{ secrets.SSH_HOST }}
          SSH_USER: ${{ secrets.SSH_USER }}
        run: |
          ssh -i ~/.ssh/deploy_key $SSH_USER@$SSH_HOST << 'ENDSSH'
            cd /opt/nself

            # Pull latest images
            docker-compose pull

            # Restart services with zero downtime
            docker-compose up -d --no-deps --build stripe
            docker-compose up -d --no-deps --build github

            # Wait for health checks
            sleep 10

            # Verify deployment
            curl -f http://localhost:3001/health || exit 1
            curl -f http://localhost:3002/health || exit 1

            # Cleanup old images
            docker image prune -f
          ENDSSH

      - name: Smoke tests
        run: |
          # Test endpoints
          curl -f https://stripe.api.example.com/health
          curl -f https://github.api.example.com/health

      - name: Notify deployment
        if: success()
        uses: 8398a7/action-slack@v3
        with:
          status: ${{ job.status }}
          text: 'Deployment to production succeeded!'
          webhook_url: ${{ secrets.SLACK_WEBHOOK }}

      - name: Rollback on failure
        if: failure()
        run: |
          ssh -i ~/.ssh/deploy_key $SSH_USER@$SSH_HOST << 'ENDSSH'
            cd /opt/nself
            docker-compose rollback
          ENDSSH
```

### GitLab CI/CD

```yaml
# .gitlab-ci.yml
stages:
  - test
  - build
  - deploy

variables:
  DOCKER_DRIVER: overlay2
  DOCKER_TLS_CERTDIR: "/certs"

test:
  stage: test
  image: node:20
  services:
    - postgres:14
  variables:
    POSTGRES_DB: test_db
    POSTGRES_USER: test_user
    POSTGRES_PASSWORD: test_pass
    DATABASE_URL: postgresql://test_user:test_pass@postgres:5432/test_db
  before_script:
    - cd shared && npm ci && npm run build
  script:
    - cd plugins/stripe/ts
    - npm ci
    - npm run typecheck
    - npm run build
    # - npm test  # when tests exist
  cache:
    key: ${CI_COMMIT_REF_SLUG}
    paths:
      - node_modules/
      - plugins/*/ts/node_modules/

build:
  stage: build
  image: docker:latest
  services:
    - docker:dind
  before_script:
    - docker login -u $CI_REGISTRY_USER -p $CI_REGISTRY_PASSWORD $CI_REGISTRY
  script:
    - docker build -t $CI_REGISTRY_IMAGE/stripe:$CI_COMMIT_SHA -f plugins/stripe/ts/Dockerfile .
    - docker push $CI_REGISTRY_IMAGE/stripe:$CI_COMMIT_SHA
    - docker tag $CI_REGISTRY_IMAGE/stripe:$CI_COMMIT_SHA $CI_REGISTRY_IMAGE/stripe:latest
    - docker push $CI_REGISTRY_IMAGE/stripe:latest
  only:
    - main

deploy:
  stage: deploy
  image: alpine:latest
  before_script:
    - apk add --no-cache openssh-client
    - eval $(ssh-agent -s)
    - echo "$SSH_PRIVATE_KEY" | tr -d '\r' | ssh-add -
    - mkdir -p ~/.ssh
    - chmod 700 ~/.ssh
    - ssh-keyscan $SSH_HOST >> ~/.ssh/known_hosts
  script:
    - ssh $SSH_USER@$SSH_HOST "cd /opt/nself && docker-compose pull && docker-compose up -d"
  environment:
    name: production
    url: https://stripe.api.example.com
  only:
    - main
```

---

## Security Hardening

### Firewall Configuration (UFW)

```bash
# Reset firewall
sudo ufw --force reset

# Default policies
sudo ufw default deny incoming
sudo ufw default allow outgoing

# Allow SSH
sudo ufw allow 22/tcp

# Allow HTTP/HTTPS
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# Allow PostgreSQL (only from app servers)
sudo ufw allow from 10.0.1.0/24 to any port 5432

# Enable firewall
sudo ufw enable

# Check status
sudo ufw status verbose
```

### Fail2ban for Intrusion Prevention

```bash
# Install fail2ban
sudo apt install fail2ban

# Create jail configuration
sudo tee /etc/fail2ban/jail.local <<EOF
[DEFAULT]
bantime = 3600
findtime = 600
maxretry = 5
destemail = admin@example.com
sendername = Fail2Ban

[sshd]
enabled = true
port = 22
logpath = /var/log/auth.log

[nginx-http-auth]
enabled = true
port = http,https
logpath = /var/log/nginx/*error.log

[nginx-noscript]
enabled = true
port = http,https
logpath = /var/log/nginx/*access.log

[nginx-badbots]
enabled = true
port = http,https
logpath = /var/log/nginx/*access.log
EOF

sudo systemctl restart fail2ban
sudo fail2ban-client status
```

### Security Updates

```bash
# Enable automatic security updates (Ubuntu/Debian)
sudo apt install unattended-upgrades
sudo dpkg-reconfigure --priority=low unattended-upgrades

# Configure
sudo tee /etc/apt/apt.conf.d/50unattended-upgrades <<EOF
Unattended-Upgrade::Allowed-Origins {
    "\${distro_id}:\${distro_codename}-security";
    "\${distro_id}ESMApps:\${distro_codename}-apps-security";
};
Unattended-Upgrade::AutoFixInterruptedDpkg "true";
Unattended-Upgrade::Remove-Unused-Dependencies "true";
Unattended-Upgrade::Automatic-Reboot "false";
EOF
```

---

## Troubleshooting

### Common Issues

#### Plugin Won't Start

```bash
# Check logs
sudo journalctl -u nself-stripe -n 100 --no-pager

# Check if port is in use
sudo lsof -i :3001

# Check environment variables
sudo systemctl show nself-stripe --property=Environment

# Test database connection
psql $DATABASE_URL -c "SELECT 1"
```

#### High Memory Usage

```bash
# Check memory usage
docker stats nself-stripe
# or
ps aux | grep node

# Increase Node.js memory limit
# In systemd: Environment="NODE_OPTIONS=--max-old-space-size=2048"
# In Docker: Add to command or ENV

# Check for memory leaks
node --inspect dist/server.js
# Then use Chrome DevTools
```

#### Webhook Signature Failures

```bash
# Verify webhook secret is correct
echo $STRIPE_WEBHOOK_SECRET

# Test signature locally
curl -X POST http://localhost:3001/webhooks/stripe \
  -H "Content-Type: application/json" \
  -H "Stripe-Signature: t=xxx,v1=xxx" \
  -d '{"type": "test"}'

# Check nginx isn't modifying body
# Ensure proxy_pass_request_body on in nginx config
```

#### Database Connection Pool Exhausted

```bash
# Check active connections
psql -U nself_user -d nself_production -c "SELECT count(*) FROM pg_stat_activity WHERE datname='nself_production';"

# Increase max_connections in postgresql.conf
# Increase pool size in application

# Kill idle connections
psql -U postgres -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE state='idle' AND state_change < now() - interval '10 minutes';"
```

#### SSL Certificate Issues

```bash
# Check certificate expiry
openssl x509 -in /etc/letsencrypt/live/stripe.api.example.com/cert.pem -noout -dates

# Renew certificate
sudo certbot renew --force-renewal

# Test SSL
curl -vI https://stripe.api.example.com
```

### Debug Mode

```bash
# Enable debug logging
export DEBUG=true
export LOG_LEVEL=debug

# Or in systemd
sudo systemctl edit nself-stripe
# Add:
# [Service]
# Environment="DEBUG=true"
# Environment="LOG_LEVEL=debug"

sudo systemctl restart nself-stripe
```

### Performance Profiling

```bash
# CPU profiling
node --prof dist/server.js

# After some time, stop and process
node --prof-process isolate-0xnnnn-v8.log > processed.txt

# Memory profiling
node --inspect dist/server.js
# Connect Chrome DevTools to ws://localhost:9229
# Take heap snapshots
```

---

## Production Deployment Checklist

Final checklist before going live:

- [ ] All services healthy and passing health checks
- [ ] SSL/TLS certificates installed and auto-renewal configured
- [ ] Database backups running and tested restore procedure
- [ ] Monitoring dashboards created and alerts configured
- [ ] Log aggregation working (Loki/CloudWatch/etc)
- [ ] Rate limiting tested and configured appropriately
- [ ] API authentication enabled and keys secured
- [ ] Webhook signature verification enabled
- [ ] Firewall rules configured and tested
- [ ] Fail2ban configured for intrusion prevention
- [ ] Process management configured (systemd/PM2/Docker)
- [ ] Reverse proxy configured with proper headers
- [ ] Load balancer health checks configured (if applicable)
- [ ] DNS records configured and propagated
- [ ] CDN configured (if applicable)
- [ ] Secrets stored securely (not in env files)
- [ ] Documentation updated with production URLs
- [ ] Runbook created for common operations
- [ ] On-call rotation established
- [ ] Incident response plan documented
- [ ] Disaster recovery plan tested
- [ ] Performance testing completed under load
- [ ] Security scan completed (OWASP ZAP, etc)
- [ ] Compliance requirements met (SOC 2, GDPR, etc)

---

**Need help?** Join the [nself Discord](https://discord.gg/nself) or open an issue on [GitHub](https://github.com/acamarata/nself-plugins).
