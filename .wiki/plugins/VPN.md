# VPN Plugin

**Version**: 1.0.0 | **Status**: ✅ Production Ready | **Port**: 3010

Multi-provider VPN management with P2P optimization, torrent downloads, kill switch, and leak protection.

---

## Overview

The VPN Plugin enables secure VPN connections across 10 major providers with specialized support for P2P/torrenting. It provides both REST API and CLI interfaces for managing VPN connections, downloading torrents securely, and orchestrating VPN operations for other plugins.

### Key Features

- ✅ **3 Providers Fully Implemented**: NordVPN, PIA, Mullvad
- ✅ **7 Providers Researched**: Surfshark, ExpressVPN, ProtonVPN, KeepSolid, CyberGhost, AirVPN, Windscribe
- ✅ **Port Forwarding**: PIA (all except US servers)
- ✅ **P2P Support**: All providers support P2P/torrenting
- ✅ **Kill Switch**: Automatic traffic blocking on VPN dropout
- ✅ **Leak Protection**: DNS, IP, IPv6, WebRTC leak detection
- ✅ **Database-Backed**: Complete PostgreSQL schema with analytics
- ✅ **REST API**: Full HTTP API for inter-plugin communication
- ✅ **CLI**: Comprehensive command-line interface
- ✅ **Torrent Integration**: WebTorrent with VPN interface binding

---

## Quick Start

### Installation

```bash
# Navigate to plugin directory
cd plugins/vpn/ts

# Install dependencies
npm install

# Build TypeScript
npm run build

# Initialize database
npx tsx src/cli.ts init
```

### Configuration

Create `.env` file:

```bash
# Required
DATABASE_URL=postgresql://user:password@localhost:5432/nself
ENCRYPTION_KEY=generate-with-openssl-rand-base64-32

# Optional
VPN_PROVIDER=nordvpn
DOWNLOAD_PATH=/tmp/vpn-downloads
ENABLE_KILL_SWITCH=true
PORT=3010
```

### Add Provider Credentials

**NordVPN:**
```bash
# Get token from: https://my.nordaccount.com/dashboard/nordvpn/manual-configuration/
npx tsx src/cli.ts providers add nordvpn --token YOUR_ACCESS_TOKEN
```

**PIA:**
```bash
npx tsx src/cli.ts providers add pia --username YOUR_USERNAME --password YOUR_PASSWORD
```

**Mullvad:**
```bash
npx tsx src/cli.ts providers add mullvad --account 1234567890123456
```

### Start Server

```bash
# Development
npm run dev

# Production
npm start
```

---

## CLI Commands

### Initialize

```bash
npx tsx src/cli.ts init
```

### Provider Management

```bash
# List all providers
npx tsx src/cli.ts providers list

# Add credentials
npx tsx src/cli.ts providers add <provider> [options]

# Options:
#   -t, --token <token>          Access token (NordVPN)
#   -u, --username <username>    Username
#   -p, --password <password>    Password
#   -a, --account <account>      Account number (Mullvad)
```

### Connection

```bash
# Connect to VPN
npx tsx src/cli.ts connect <provider> [options]

# Options:
#   -r, --region <region>        Region/country code (us, uk, nl)
#   -c, --city <city>            City name
#   -s, --server <server>        Specific server hostname
#   -p, --protocol <protocol>    Protocol (wireguard, openvpn_udp, openvpn_tcp)
#   --p2p                        Connect to best P2P server
#   --no-kill-switch             Disable kill switch
#   --port-forwarding            Enable port forwarding (PIA only)

# Examples:
npx tsx src/cli.ts connect nordvpn --p2p
npx tsx src/cli.ts connect pia --region us-east --port-forwarding
npx tsx src/cli.ts connect mullvad --region se --city sto

# Disconnect
npx tsx src/cli.ts disconnect

# Check status
npx tsx src/cli.ts status
```

### Server Management

```bash
# List servers
npx tsx src/cli.ts servers [options]

# Options:
#   -p, --provider <provider>    Filter by provider
#   -c, --country <country>      Filter by country code
#   --p2p                        Show only P2P servers
#   --port-forwarding            Show only servers with port forwarding
#   -l, --limit <number>         Limit results (default: 20)

# Sync server list from provider
npx tsx src/cli.ts sync <provider>
```

### Torrenting

```bash
# Download via torrent over VPN (requires server running)
curl -X POST http://localhost:3010/api/download \
  -H "Content-Type: application/json" \
  -d '{
    "magnet_link": "magnet:?xt=urn:btih:...",
    "provider": "nordvpn",
    "region": "nl",
    "requested_by": "cli"
  }'
```

### Security Testing

```bash
# Test for leaks
npx tsx src/cli.ts test

# Shows:
# ✓ DNS Leak: Pass
# ✓ IP Leak: Pass
# ✓ IPv6 Leak: Pass
# ✓ WebRTC Leak: Pass
```

### Statistics

```bash
# Show usage statistics
npx tsx src/cli.ts stats
```

---

## REST API

All endpoints are available at `http://localhost:3010`.

### Provider Endpoints

**GET `/api/providers`**
```bash
curl http://localhost:3010/api/providers
```
Returns list of all supported providers with metadata.

**GET `/api/providers/:id`**
```bash
curl http://localhost:3010/api/providers/nordvpn
```
Get specific provider details.

**POST `/api/providers/:id/credentials`**
```bash
curl -X POST http://localhost:3010/api/providers/nordvpn/credentials \
  -H "Content-Type: application/json" \
  -d '{"token": "YOUR_TOKEN"}'
```
Store provider credentials (encrypted).

### Server Endpoints

**GET `/api/servers`**
```bash
# All servers
curl http://localhost:3010/api/servers

# P2P only
curl http://localhost:3010/api/servers?p2p_only=true&country=nl&limit=10
```

**GET `/api/servers/p2p`**
```bash
curl http://localhost:3010/api/servers/p2p?provider=nordvpn
```

**POST `/api/servers/sync`**
```bash
curl -X POST http://localhost:3010/api/servers/sync \
  -H "Content-Type: application/json" \
  -d '{"provider": "nordvpn"}'
```

### Connection Endpoints

**POST `/api/connect`**
```bash
curl -X POST http://localhost:3010/api/connect \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "nordvpn",
    "region": "us",
    "protocol": "wireguard",
    "kill_switch": true,
    "requested_by": "api"
  }'
```

**POST `/api/disconnect`**
```bash
curl -X POST http://localhost:3010/api/disconnect
```

**GET `/api/status`**
```bash
curl http://localhost:3010/api/status
```

### Download Endpoints

**POST `/api/download`**
```bash
curl -X POST http://localhost:3010/api/download \
  -H "Content-Type: application/json" \
  -d '{
    "magnet_link": "magnet:?xt=urn:btih:...",
    "provider": "nordvpn",
    "region": "nl",
    "destination": "/tmp/downloads",
    "requested_by": "api"
  }'
```

**GET `/api/downloads`**
```bash
curl http://localhost:3010/api/downloads?limit=10
```

**GET `/api/downloads/:id`**
```bash
curl http://localhost:3010/api/downloads/download-id
```

**DELETE `/api/downloads/:id`**
```bash
curl -X DELETE http://localhost:3010/api/downloads/download-id
```

### Security Endpoints

**POST `/api/test-leak`**
```bash
curl -X POST http://localhost:3010/api/test-leak
```

### Statistics Endpoints

**GET `/api/stats`**
```bash
curl http://localhost:3010/api/stats
```

---

## Supported Providers

### Fully Implemented (Ready to Use)

| Provider | Status | P2P Servers | Port Forwarding | CLI Required |
|----------|--------|-------------|-----------------|--------------|
| **NordVPN** | ✅ Complete | 5,500+ (47 countries) | ❌ | Yes |
| **PIA** | ✅ Complete | All ~600 (except US PF) | ✅ Yes | Yes |
| **Mullvad** | ✅ Complete | All 674 servers | ❌ | Yes |

### Researched (Implementation Ready)

| Provider | P2P Servers | Port Forwarding | Notes |
|----------|-------------|-----------------|-------|
| **Surfshark** | All 4,500+ | ❌ | Manual WireGuard configs |
| **ExpressVPN** | All 3,000+ | ❌ | Lightway protocol, CLI available |
| **ProtonVPN** | 140+ dedicated | ✅ NAT-PMP | Python CLI, Plus plan required |
| **CyberGhost** | 87 locations | ❌ | CLI available, dedicated P2P |
| **AirVPN** | All 260 | ✅ 20 ports | Eddie CLI, power-user focused |
| **Windscribe** | 600+ | ✅ Pro only | CLI beta, free tier available |
| **KeepSolid** | **ONLY 5** | ❌ | ⚠️ NOT RECOMMENDED (limited P2P) |

---

## Provider Details

### NordVPN

**Features:**
- 5,500+ P2P servers in 47 countries
- NordLynx (WireGuard) for maximum speed
- Built-in kill switch
- Threat Protection Lite (ad blocking)
- DNS leak protection

**CLI Setup:**
```bash
# Install (Ubuntu/Debian)
sh <(curl -sSf https://downloads.nordcdn.com/apps/linux/install.sh)

# Add user to group
sudo usermod -aG nordvpn $USER

# Start daemon
sudo systemctl start nordvpnd.service
sudo systemctl enable nordvpnd.service

# Login
nordvpn login --token YOUR_TOKEN
```

**Usage:**
```bash
# Connect to best P2P server
npx tsx src/cli.ts connect nordvpn --p2p

# Connect to specific country
npx tsx src/cli.ts connect nordvpn --region nl

# Connect to specific server
npx tsx src/cli.ts connect nordvpn --server nl928
```

**Notes:**
- No port forwarding
- Excellent for speed and reliability
- Large server network

### Private Internet Access (PIA)

**Features:**
- Port forwarding on all servers except US
- All servers support P2P
- `piactl` CLI for automation
- Built-in kill switch (always active)

**CLI Setup:**
```bash
# Install PIA desktop app
# Download from: https://www.privateinternetaccess.com/download

# Credentials: username/password format
```

**Usage:**
```bash
# Connect with port forwarding
npx tsx src/cli.ts connect pia --region au-melbourne --port-forwarding

# Connect to specific region
npx tsx src/cli.ts connect pia --region ca-vancouver
```

**Port Forwarding:**
- Automatically enabled when connecting to supported regions
- Port is assigned dynamically
- Must refresh every 15 minutes
- Essential for optimal torrenting speeds

**Notes:**
- Best choice for P2P with port forwarding
- No port forwarding on US servers (legal reasons)
- Reliable for torrenting

### Mullvad

**Features:**
- Account number-based (no email required)
- All 674 servers support P2P
- Strong privacy focus
- Automatic WireGuard key rotation
- DAITA (traffic analysis defense)
- Lockdown mode (kill switch)

**CLI Setup:**
```bash
# Install Mullvad app
# Download from: https://mullvad.net/en/download

# Login with 16-digit account number
mullvad account login 1234567890123456
```

**Usage:**
```bash
# Connect to country
npx tsx src/cli.ts connect mullvad --region se

# Connect to specific city
npx tsx src/cli.ts connect mullvad --region au --city syd

# Connect to specific server
npx tsx src/cli.ts connect mullvad --server se-sto-wg-001
```

**Notes:**
- Port forwarding removed July 2023 (abuse concerns)
- Still excellent for P2P, just slightly slower seeding
- Best privacy posture
- Owns most of its hardware

---

## Inter-Plugin Communication

Other plugins can use the VPN plugin to download files securely via torrent.

### Example: OS Update Plugin

```typescript
// In osupd plugin
async downloadUbuntuISO(version: string) {
  // 1. Find torrent
  const magnetLink = await this.findTorrent('ubuntu', version);

  // 2. Request VPN download
  const response = await fetch('http://localhost:3010/api/download', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      magnet_link: magnetLink,
      provider: 'nordvpn',
      region: 'nl',
      destination: '/tmp/ubuntu-iso',
      requested_by: 'osupd'
    })
  });

  const { download_id } = await response.json();

  // 3. Poll for completion
  while (true) {
    const status = await fetch(`http://localhost:3010/api/downloads/${download_id}`)
      .then(r => r.json());

    if (status.status === 'completed') {
      return status.destination_path;
    }

    if (status.status === 'failed') {
      throw new Error(status.error_message);
    }

    await new Promise(r => setTimeout(r, 5000)); // Poll every 5s
  }
}
```

---

## Database Schema

### Tables

- **vpn_providers** - Provider metadata (10 providers)
- **vpn_credentials** - Encrypted credentials (pgcrypto)
- **vpn_servers** - Server list (20,000+ P2P servers)
- **vpn_connections** - Connection history
- **vpn_downloads** - Torrent download tracking
- **vpn_connection_logs** - Event logs
- **vpn_server_performance** - Performance metrics
- **vpn_leak_tests** - Security test results

### Views

- **vpn_active_connections** - Real-time status
- **vpn_server_stats** - Server rankings
- **vpn_download_history** - Download audit trail
- **vpn_provider_uptime** - Provider reliability

---

## Security Features

### Kill Switch

Automatically blocks all internet traffic if VPN drops:

- **NordVPN**: `nordvpn set killswitch on`
- **PIA**: Always active (cannot be disabled)
- **Mullvad**: Lockdown mode (`mullvad lockdown-mode set on`)

### Leak Protection

Comprehensive testing for:
- **DNS Leaks**: ISP DNS vs VPN DNS
- **IP Leaks**: Real IP vs VPN IP
- **IPv6 Leaks**: IPv6 should be blocked
- **WebRTC Leaks**: Browser-level IP exposure

Run leak test:
```bash
npx tsx src/cli.ts test
```

### Credential Encryption

All credentials stored encrypted with pgcrypto:
```sql
-- Encrypt
pgp_sym_encrypt('password', 'encryption_key')

-- Decrypt
pgp_sym_decrypt(password_encrypted, 'encryption_key')
```

---

## Troubleshooting

### VPN Not Connecting

```bash
# Check CLI is installed
nordvpn --version  # or piactl --version, mullvad version

# Check daemon is running
sudo systemctl status nordvpnd.service

# Check credentials
npx tsx src/cli.ts providers list

# Check logs
npx tsx src/cli.ts status
```

### Kill Switch Blocks Internet After Disconnect

```bash
# NordVPN
nordvpn set killswitch off

# Mullvad
mullvad lockdown-mode set off

# Or restart network
sudo systemctl restart NetworkManager
```

### Database Connection Error

```bash
# Verify PostgreSQL running
sudo systemctl status postgresql

# Test connection
psql $DATABASE_URL -c "SELECT 1"

# Check .env file
cat .env | grep DATABASE_URL
```

### Leak Test Fails

Ensure VPN is connected:
```bash
npx tsx src/cli.ts status

# If not connected
npx tsx src/cli.ts connect <provider>

# Then test again
npx tsx src/cli.ts test
```

---

## Performance

### Expected Speeds (P2P)

| Provider | Protocol | Avg Speed |
|----------|----------|-----------|
| NordVPN | NordLynx | 400-500 Mbps |
| PIA | WireGuard | 300-400 Mbps |
| Mullvad | WireGuard | 350-450 Mbps |

### API Response Times

- **Connect**: 10-30 seconds (VPN connection time)
- **Status**: <100ms
- **Download**: <500ms (queues torrent)
- **Servers**: <200ms (100 servers)

---

## Development

### Adding New Provider

1. Copy existing provider (e.g., `nordvpn.ts`)
2. Implement all abstract methods from `BaseVPNProvider`
3. Add to provider registry in `providers/index.ts`
4. Update provider metadata
5. Test and verify

### Testing

```bash
# Type check
npm run typecheck

# Build
npm run build

# Run CLI
npx tsx src/cli.ts --help

# Start server
npm run dev
```

---

## Links

- **GitHub**: https://github.com/acamarata/nself-plugins
- **Issues**: https://github.com/acamarata/nself-plugins/issues
- **Documentation**: https://github.com/acamarata/nself-plugins/wiki/VPN-Plugin

---

**Version**: 1.0.0 | **Last Updated**: February 11, 2026 | **Status**: Production Ready ✅
