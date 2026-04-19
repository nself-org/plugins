# mDNS Plugin

mDNS (Multicast DNS) and Bonjour service discovery plugin for zero-configuration LAN advertising. Makes your nself services discoverable on local networks without manual configuration.

| Property | Value |
|----------|-------|
| **Port** | `3216` |
| **Category** | `networking` |
| **Multi-App** | `source_account_id` (UUID) |
| **Min nself** | `0.4.8` |

---

## Quick Start

```bash
nself plugin run mdns init
nself plugin run mdns server
```

---

## Features

- **Zero-Configuration Networking** - Services automatically discoverable on LAN
- **Bonjour/Avahi Compatible** - Works with Apple Bonjour and Linux Avahi
- **Service Advertisement** - Broadcast your services to the network
- **Service Discovery** - Find other mDNS services on your network
- **Cross-Platform** - Works on macOS, Linux, Windows (with Bonjour)
- **Discovery Logging** - Track discovered services over time
- **Multiple Service Types** - Support for any mDNS service type

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MDNS_PLUGIN_PORT` | `3216` | Server port |
| `MDNS_SERVICE_TYPE` | `_ntv._tcp` | Default mDNS service type |
| `MDNS_INSTANCE_NAME` | - | Service instance name (hostname if not set) |
| `MDNS_DOMAIN` | `local` | mDNS domain |

### Service Types

Common mDNS service types:

| Service Type | Description | Discovery |
|--------------|-------------|-----------|
| `_http._tcp` | HTTP services | Web servers |
| `_https._tcp` | HTTPS services | Secure web servers |
| `_ssh._tcp` | SSH services | Remote terminals |
| `_ftp._tcp` | FTP services | File transfer |
| `_smb._tcp` | SMB/CIFS shares | Windows file sharing |
| `_afp._tcp` | Apple File Protocol | macOS file sharing |
| `_ntv._tcp` | nself TV | Custom nself service |

---

## Installation

```bash
# Install plugin
nself plugin install mdns

# Initialize database
nself plugin run mdns init

# Start server
nself plugin run mdns server
```

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (2 tables) |
| `server` | Start the HTTP API server (`-p`/`--port`) |
| `advertise` | Start advertising a service (`--name`, `--type`, `--port`, `--txt?`) |
| `discover` | Discover services on network (`--type?`, `--timeout?`) |
| `services` | List currently advertised services |
| `stats` | Show mDNS statistics (advertised, discovered, log entries) |

---

## REST API

### Health & Status

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (DB) |

### Service Advertisement

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/mdns/advertise` | Start advertising a service (body: `name`, `service_type`, `port`, `txt?`, `domain?`) |
| `GET` | `/api/mdns/advertised` | List all advertised services |
| `DELETE` | `/api/mdns/advertise/:id` | Stop advertising a service |

### Service Discovery

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/mdns/discover` | Discover services (body: `service_type?`, `timeout?`) |
| `GET` | `/api/mdns/discovered` | List recently discovered services |
| `GET` | `/api/mdns/log` | Get discovery log (query: `limit?`, `offset?`, `service_type?`) |

---

## Webhook Events

| Event | Description |
|-------|-------------|
| `mdns.service.advertised` | Service advertisement started |
| `mdns.service.stopped` | Service advertisement stopped |
| `mdns.service.discovered` | New service discovered on network |
| `mdns.service.lost` | Previously discovered service no longer responding |

---

## Database Schema

### `np_mdns_services`

Services being advertised by this plugin.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Service ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `instance_name` | `VARCHAR(255)` | Service instance name |
| `service_type` | `VARCHAR(100)` | mDNS service type (e.g., `_http._tcp`) |
| `domain` | `VARCHAR(100)` | mDNS domain (default: `local`) |
| `port` | `INTEGER` | Service port number |
| `txt_records` | `JSONB` | TXT record key-value pairs |
| `is_active` | `BOOLEAN` | Whether currently advertising |
| `started_at` | `TIMESTAMPTZ` | When advertisement started |
| `stopped_at` | `TIMESTAMPTZ` | When advertisement stopped |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `np_mdns_discovery_log`

Log of discovered services on the network.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Log entry ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `instance_name` | `VARCHAR(255)` | Discovered service name |
| `service_type` | `VARCHAR(100)` | Service type |
| `domain` | `VARCHAR(100)` | Domain |
| `hostname` | `VARCHAR(255)` | Host advertising the service |
| `ip_addresses` | `TEXT[]` | Array of IP addresses |
| `port` | `INTEGER` | Service port |
| `txt_records` | `JSONB` | TXT records from service |
| `first_seen_at` | `TIMESTAMPTZ` | First discovery timestamp |
| `last_seen_at` | `TIMESTAMPTZ` | Most recent discovery |
| `is_online` | `BOOLEAN` | Whether service is currently responding |
| `created_at` | `TIMESTAMPTZ` | Log entry creation |

---

## Usage Examples

### Advertise nself Services

```bash
# Advertise nself backend API
nself plugin run mdns advertise \
  --name "nself-backend" \
  --type "_http._tcp" \
  --port 3000 \
  --txt "version=0.9.9,env=production"

# Advertise nself TV
nself plugin run mdns advertise \
  --name "nself-tv" \
  --type "_ntv._tcp" \
  --port 3210
```

### API Usage

```bash
# Start advertising
curl -X POST http://localhost:3216/api/mdns/advertise \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My nself Server",
    "service_type": "_http._tcp",
    "port": 3000,
    "txt": {
      "version": "0.9.9",
      "env": "production"
    }
  }'

# List advertised services
curl http://localhost:3216/api/mdns/advertised

# Stop advertising
curl -X DELETE http://localhost:3216/api/mdns/advertise/{service_id}
```

### Discover Services

```bash
# Discover all HTTP services
nself plugin run mdns discover --type "_http._tcp" --timeout 10

# Discover via API
curl -X POST http://localhost:3216/api/mdns/discover \
  -H "Content-Type: application/json" \
  -d '{
    "service_type": "_http._tcp",
    "timeout": 10
  }'

# View discovery log
curl http://localhost:3216/api/mdns/log?limit=20
```

### TXT Records

TXT records provide additional service information:

```bash
# Advertise with metadata
curl -X POST http://localhost:3216/api/mdns/advertise \
  -H "Content-Type: application/json" \
  -d '{
    "name": "nself-backend",
    "service_type": "_http._tcp",
    "port": 3000,
    "txt": {
      "path": "/api/v1",
      "version": "0.9.9",
      "protocol": "https",
      "api_key_required": "true"
    }
  }'
```

---

## How It Works

### mDNS Protocol

mDNS uses multicast UDP on `224.0.0.251:5353` (IPv4) and `ff02::fb:5353` (IPv6).

**Service Advertisement:**
1. Plugin sends multicast DNS announcement
2. Network devices cache the service information
3. Periodic re-announcements keep the service active
4. Goodbye packet sent when service stops

**Service Discovery:**
1. Plugin sends multicast DNS query for a service type
2. Network devices respond if they offer that service
3. Plugin receives and logs all responses
4. Continuous monitoring detects when services go offline

### Service Naming Convention

Format: `{instance_name}.{service_type}.{domain}`

Example: `nself-backend._http._tcp.local`

---

## Platform Requirements

### macOS

Bonjour is built-in, no additional setup needed.

### Linux

Install Avahi daemon:

```bash
# Ubuntu/Debian
sudo apt-get install avahi-daemon avahi-utils

# CentOS/RHEL
sudo yum install avahi avahi-tools

# Start service
sudo systemctl start avahi-daemon
sudo systemctl enable avahi-daemon
```

### Windows

Install [Bonjour Print Services](https://support.apple.com/kb/DL999) or [Bonjour SDK](https://developer.apple.com/bonjour/).

---

## Common Use Cases

### Home Network Device Discovery

```bash
# Advertise nself server for mobile apps
nself plugin run mdns advertise \
  --name "nself-home" \
  --type "_ntv._tcp" \
  --port 3000

# Mobile app discovers automatically (no IP configuration needed)
```

### Multi-Server Coordination

```bash
# Server 1 advertises
curl -X POST http://localhost:3216/api/mdns/advertise \
  -d '{"name":"nself-1","service_type":"_ntv._tcp","port":3000}'

# Server 2 discovers Server 1
curl -X POST http://localhost:3216/api/mdns/discover \
  -d '{"service_type":"_ntv._tcp"}'

# Returns: {"services":[{"name":"nself-1","ip":"192.168.1.100","port":3000}]}
```

### Network Service Monitoring

```bash
# Continuous discovery
while true; do
  nself plugin run mdns discover --type "_http._tcp"
  sleep 60
done

# Track services coming online/offline in discovery log
curl http://localhost:3216/api/mdns/log
```

---

## Troubleshooting

**"No services discovered"** -- Verify firewall allows UDP port 5353 (multicast DNS). Check that target devices are on the same network subnet.

**"Service not showing in Finder/Network"** -- macOS Finder filters service types. Use `dns-sd -B` command line tool to verify: `dns-sd -B _http._tcp`.

**"Advertisement not starting"** -- Ensure mDNS daemon is running (Avahi on Linux, Bonjour on macOS/Windows). Check port is not already in use.

**"Discovery timeout"** -- Increase `timeout` parameter. Some services respond slowly or only after repeat queries. Default 5-10 seconds recommended.

**"Duplicate services in log"** -- This is normal. Services re-announce periodically. The `last_seen_at` timestamp updates on each announcement.

**"Service advertised but not accessible"** -- Verify the actual service is running on the advertised port. mDNS only advertises, it doesn't proxy traffic.

---

## Security Considerations

- **Local Network Only** - mDNS is not routable across networks (stays on LAN)
- **No Authentication** - Any device on LAN can discover advertised services
- **TXT Record Visibility** - Don't put secrets in TXT records (they're plaintext)
- **Firewall Rules** - Allow UDP 5353 for mDNS to work
- **Service Enumeration** - Attackers on LAN can enumerate services

**Best Practices:**
- Use mDNS for discovery, not as primary security boundary
- Still require authentication on discovered services
- Consider VPN for remote access instead of exposing mDNS to internet
- Don't advertise sensitive internal services if guests are on network

---

## Advanced Configuration

### Using with nself Backend

Add to your `.env.dev`:

```bash
# Enable mDNS plugin
MDNS_PLUGIN_ENABLED=true
MDNS_PLUGIN_PORT=3216

# Configure service advertisement
MDNS_SERVICE_TYPE=_ntv._tcp
MDNS_INSTANCE_NAME=nself-backend
MDNS_DOMAIN=local
```

### Multiple Service Advertisements

```bash
# Advertise multiple services
for service in api:3000 web:3001 admin:3002; do
  IFS=: read name port <<< "$service"
  curl -X POST http://localhost:3216/api/mdns/advertise \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$name\",\"service_type\":\"_http._tcp\",\"port\":$port}"
done
```

### Custom Service Types

Create your own service types for app-specific discovery:

```bash
# Define custom service type
SERVICE_TYPE="_myapp._tcp"

# Server advertises
nself plugin run mdns advertise \
  --name "myapp-backend" \
  --type "$SERVICE_TYPE" \
  --port 8080

# Client discovers
nself plugin run mdns discover --type "$SERVICE_TYPE"
```

---

## DNS-SD vs mDNS

- **DNS-SD** (DNS Service Discovery) - Protocol for describing services
- **mDNS** (Multicast DNS) - Transport mechanism for local network

This plugin implements both: mDNS for transport, DNS-SD for service description.

---

## Testing & Debugging

### Command-Line Tools

**macOS:**
```bash
# Browse for services
dns-sd -B _http._tcp

# Lookup specific service
dns-sd -L "nself-backend" _http._tcp

# Resolve hostname
dns-sd -G v4 nself-backend.local
```

**Linux:**
```bash
# Browse services
avahi-browse -a

# Resolve service
avahi-resolve -n nself-backend.local
```

**Cross-Platform:**
```bash
# Query mDNS directly
nslookup nself-backend.local
dig @224.0.0.251 -p 5353 nself-backend.local
```

---

## Performance

- **Minimal overhead** - Announcement packets are ~100-200 bytes
- **Low CPU usage** - Passive listening with periodic announcements
- **No DNS server needed** - Fully peer-to-peer
- **Fast discovery** - Services respond within 1-2 seconds
- **Scales well** - Works with hundreds of devices on LAN

---

## Integration Examples

### iOS/macOS Discovery (Swift)

```swift
let browser = NWBrowser(for: .bonjour(type: "_ntv._tcp", domain: "local"), using: .tcp)
browser.browseResultsChangedHandler = { results, changes in
    for result in results {
        print("Found: \(result.endpoint)")
    }
}
browser.start(queue: .main)
```

### Android Discovery (Kotlin)

```kotlin
val nsdManager = getSystemService(Context.NSD_SERVICE) as NsdManager
val listener = object : NsdManager.DiscoveryListener {
    override fun onServiceFound(service: NsdServiceInfo) {
        println("Found: ${service.serviceName}")
    }
}
nsdManager.discoverServices("_ntv._tcp", NsdManager.PROTOCOL_DNS_SD, listener)
```

### Node.js Discovery

```javascript
const bonjour = require('bonjour')();
bonjour.find({ type: 'ntv' }, (service) => {
  console.log('Found:', service.name, service.addresses, service.port);
});
```
