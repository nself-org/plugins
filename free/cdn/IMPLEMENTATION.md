# CDN Plugin - Full Implementation Guide

## Overview

The CDN plugin provides comprehensive CDN management with support for **Cloudflare** and **BunnyCDN**. The plugin handles cache purging, signed URLs for secure content delivery, and analytics synchronization.

## Current Status

**Infrastructure Status**: ✅ Complete (database, API endpoints, server)
**Provider Integration Status**: ⚠️ Placeholder (requires API implementation)

## What's Already Built

- ✅ Complete database schema for zones, purge requests, signed URLs, analytics
- ✅ Full REST API with all endpoints
- ✅ Multi-tenant support
- ✅ Rate limiting and authentication
- ✅ Signed URL generation with crypto

## What Needs Implementation

**Provider API Integration** - The actual CDN provider calls in:
- `purgeCache()` - Execute cache purge via provider API
- `syncAnalytics()` - Fetch analytics from provider
- `validateZone()` - Verify zone exists with provider

---

## Required Packages

All dependencies are **already installed** via `pnpm install`:

```json
{
  "@nself/plugin-utils": "file:../../../shared",
  "fastify": "^4.24.0",
  "@fastify/cors": "^8.4.0",
  "dotenv": "^16.3.1",
  "commander": "^11.1.0",
  "pg": "^8.11.3"
}
```

### Additional Packages for Provider Integration

Add these to `ts/package.json`:

```bash
# Cloudflare API client
pnpm add cloudflare

# BunnyCDN API client (unofficial - REST client works too)
pnpm add @bunny.net/edge-script-client

# Or use generic HTTP client
pnpm add axios
```

---

## Complete Implementation Code

### 1. Provider Integration Module

Create `ts/src/providers.ts`:

```typescript
/**
 * CDN Provider Integration
 * Handles Cloudflare and BunnyCDN API calls
 */

import axios from 'axios';

export interface PurgeOptions {
  urls?: string[];
  tags?: string[];
  prefixes?: string[];
  purgeAll?: boolean;
}

export interface AnalyticsData {
  timestamp: Date;
  requests: number;
  bandwidth: number;
  cache_hit_rate: number;
  status_2xx: number;
  status_3xx: number;
  status_4xx: number;
  status_5xx: number;
}

/**
 * Cloudflare CDN Provider
 */
export class CloudflareProvider {
  private apiToken: string;
  private accountId?: string;

  constructor(apiToken: string, accountId?: string) {
    this.apiToken = apiToken;
    this.accountId = accountId;
  }

  /**
   * Purge cache for a Cloudflare zone
   */
  async purgeCache(zoneId: string, options: PurgeOptions): Promise<{ success: boolean; purgeId?: string }> {
    const url = `https://api.cloudflare.com/client/v4/zones/${zoneId}/purge_cache`;

    const payload: Record<string, unknown> = {};

    if (options.purgeAll) {
      payload.purge_everything = true;
    } else {
      if (options.urls) payload.files = options.urls;
      if (options.tags) payload.tags = options.tags;
      if (options.prefixes) payload.prefixes = options.prefixes;
    }

    try {
      const response = await axios.post(url, payload, {
        headers: {
          'Authorization': `Bearer ${this.apiToken}`,
          'Content-Type': 'application/json',
        },
      });

      return {
        success: response.data.success,
        purgeId: response.data.result?.id,
      };
    } catch (error) {
      if (axios.isAxiosError(error)) {
        throw new Error(`Cloudflare purge failed: ${error.response?.data?.errors?.[0]?.message || error.message}`);
      }
      throw error;
    }
  }

  /**
   * Fetch analytics for a Cloudflare zone
   */
  async getAnalytics(zoneId: string, since: Date, until: Date): Promise<AnalyticsData[]> {
    const url = `https://api.cloudflare.com/client/v4/zones/${zoneId}/analytics/dashboard`;

    const params = {
      since: since.toISOString(),
      until: until.toISOString(),
      continuous: false,
    };

    try {
      const response = await axios.get(url, {
        headers: { 'Authorization': `Bearer ${this.apiToken}` },
        params,
      });

      if (!response.data.success) {
        throw new Error('Cloudflare analytics request failed');
      }

      const timeseries = response.data.result.timeseries;

      return timeseries.map((point: Record<string, unknown>) => ({
        timestamp: new Date(point.since as string),
        requests: (point.requests?.all as number) ?? 0,
        bandwidth: (point.bandwidth?.all as number) ?? 0,
        cache_hit_rate: (point.requests?.cached as number) / (point.requests?.all as number) * 100,
        status_2xx: (point.requests?.http_status?.['200'] as number) ?? 0,
        status_3xx: (point.requests?.http_status?.['301'] as number) ?? 0,
        status_4xx: (point.requests?.http_status?.['404'] as number) ?? 0,
        status_5xx: (point.requests?.http_status?.['500'] as number) ?? 0,
      }));
    } catch (error) {
      if (axios.isAxiosError(error)) {
        throw new Error(`Cloudflare analytics failed: ${error.response?.data?.errors?.[0]?.message || error.message}`);
      }
      throw error;
    }
  }

  /**
   * Verify zone exists and is accessible
   */
  async validateZone(zoneId: string): Promise<{ valid: boolean; name?: string }> {
    try {
      const response = await axios.get(`https://api.cloudflare.com/client/v4/zones/${zoneId}`, {
        headers: { 'Authorization': `Bearer ${this.apiToken}` },
      });

      return {
        valid: response.data.success,
        name: response.data.result?.name,
      };
    } catch (error) {
      return { valid: false };
    }
  }
}

/**
 * BunnyCDN Provider
 */
export class BunnyCDNProvider {
  private apiKey: string;
  private accountId: string;

  constructor(apiKey: string, accountId: string) {
    this.apiKey = apiKey;
    this.accountId = accountId;
  }

  /**
   * Purge cache for a BunnyCDN pull zone
   */
  async purgeCache(pullZoneId: string, options: PurgeOptions): Promise<{ success: boolean; purgeId?: string }> {
    const baseUrl = 'https://api.bunny.net';

    try {
      if (options.purgeAll) {
        // Purge entire pull zone
        const response = await axios.post(
          `${baseUrl}/pullzone/${pullZoneId}/purgeCache`,
          {},
          {
            headers: {
              'AccessKey': this.apiKey,
              'Content-Type': 'application/json',
            },
          }
        );

        return { success: response.status === 204, purgeId: pullZoneId };
      } else if (options.urls) {
        // Purge specific URLs
        const response = await axios.post(
          `${baseUrl}/purge`,
          { urls: options.urls },
          {
            headers: {
              'AccessKey': this.apiKey,
              'Content-Type': 'application/json',
            },
          }
        );

        return { success: response.status === 204 };
      }

      throw new Error('BunnyCDN requires either purgeAll or urls');
    } catch (error) {
      if (axios.isAxiosError(error)) {
        throw new Error(`BunnyCDN purge failed: ${error.response?.data?.message || error.message}`);
      }
      throw error;
    }
  }

  /**
   * Fetch analytics for a BunnyCDN pull zone
   */
  async getAnalytics(pullZoneId: string, since: Date, until: Date): Promise<AnalyticsData[]> {
    const url = `https://api.bunny.net/pullzone/${pullZoneId}/statistics`;

    const params = {
      dateFrom: since.toISOString().split('T')[0],
      dateTo: until.toISOString().split('T')[0],
    };

    try {
      const response = await axios.get(url, {
        headers: { 'AccessKey': this.apiKey },
        params,
      });

      // BunnyCDN returns aggregated stats - convert to daily points
      const data = response.data;

      return [{
        timestamp: since,
        requests: data.TotalRequestsServed ?? 0,
        bandwidth: data.TotalBandwidthUsed ?? 0,
        cache_hit_rate: data.CacheHitRate ?? 0,
        status_2xx: data.TotalRequestsServed ?? 0, // BunnyCDN doesn't break down by status
        status_3xx: 0,
        status_4xx: data.ErrorRequestsServed ?? 0,
        status_5xx: 0,
      }];
    } catch (error) {
      if (axios.isAxiosError(error)) {
        throw new Error(`BunnyCDN analytics failed: ${error.response?.data?.message || error.message}`);
      }
      throw error;
    }
  }

  /**
   * Verify pull zone exists
   */
  async validateZone(pullZoneId: string): Promise<{ valid: boolean; name?: string }> {
    try {
      const response = await axios.get(`https://api.bunny.net/pullzone/${pullZoneId}`, {
        headers: { 'AccessKey': this.apiKey },
      });

      return {
        valid: response.status === 200,
        name: response.data.Name,
      };
    } catch (error) {
      return { valid: false };
    }
  }
}

/**
 * Provider factory - creates appropriate provider instance
 */
export function createProvider(
  provider: string,
  config: Record<string, string>
): CloudflareProvider | BunnyCDNProvider {
  switch (provider.toLowerCase()) {
    case 'cloudflare':
      if (!config.CDN_CLOUDFLARE_API_TOKEN) {
        throw new Error('CDN_CLOUDFLARE_API_TOKEN is required for Cloudflare provider');
      }
      return new CloudflareProvider(
        config.CDN_CLOUDFLARE_API_TOKEN,
        config.CDN_CLOUDFLARE_ACCOUNT_ID
      );

    case 'bunnycdn':
      if (!config.CDN_BUNNYCDN_API_KEY || !config.CDN_BUNNYCDN_ACCOUNT_ID) {
        throw new Error('CDN_BUNNYCDN_API_KEY and CDN_BUNNYCDN_ACCOUNT_ID are required for BunnyCDN');
      }
      return new BunnyCDNProvider(
        config.CDN_BUNNYCDN_API_KEY,
        config.CDN_BUNNYCDN_ACCOUNT_ID
      );

    default:
      throw new Error(`Unsupported CDN provider: ${provider}`);
  }
}
```

### 2. Update Server to Use Providers

Modify `ts/src/server.ts` to integrate providers. Replace placeholder sections:

```typescript
import { createProvider } from './providers.js';

// In createServer() after config load:
const cdnProvider = fullConfig.provider !== 'local'
  ? createProvider(fullConfig.provider, process.env as Record<string, string>)
  : null;

// Update the purge endpoint (around line 199):
await scopedDb(request).createPurgeRequest(body.zone_id, purgeType, {
  urls: body.urls,
  tags: body.tags,
  prefixes: body.prefixes,
  requested_by: body.requested_by,
});

// Execute actual purge if provider is configured
if (cdnProvider) {
  try {
    const result = await cdnProvider.purgeCache(zone.zone_id, {
      urls: body.urls,
      tags: body.tags,
      prefixes: body.prefixes,
      purgeAll: false,
    });

    if (result.success) {
      await scopedDb(request).updatePurgeStatus(purgeRecord.id, 'completed', result.purgeId);
    } else {
      await scopedDb(request).updatePurgeStatus(purgeRecord.id, 'failed');
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    await scopedDb(request).updatePurgeStatus(purgeRecord.id, 'failed', undefined, [message]);
    throw error;
  }
} else {
  // Local/testing mode - mark as completed without actual purge
  await scopedDb(request).updatePurgeStatus(purgeRecord.id, 'completed');
}

// Update analytics sync endpoint (around line 374):
app.post('/sync', async (request, reply) => {
  try {
    logger.info('Starting analytics sync...');

    if (!cdnProvider) {
      return {
        success: false,
        message: 'No CDN provider configured - set CDN_PROVIDER environment variable',
      };
    }

    const zones = await scopedDb(request).listZones();
    let syncedCount = 0;
    const errors: string[] = [];

    for (const zone of zones) {
      try {
        // Fetch last 24 hours of analytics
        const until = new Date();
        const since = new Date(until.getTime() - 24 * 60 * 60 * 1000);

        const analyticsData = await cdnProvider.getAnalytics(zone.zone_id, since, until);

        // Store analytics in database
        for (const point of analyticsData) {
          await scopedDb(request).insertAnalytics(zone.id, {
            timestamp: point.timestamp,
            requests: point.requests,
            bandwidth: point.bandwidth,
            cache_hit_rate: point.cache_hit_rate,
            status_2xx: point.status_2xx,
            status_3xx: point.status_3xx,
            status_4xx: point.status_4xx,
            status_5xx: point.status_5xx,
          });
        }

        syncedCount++;
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        errors.push(`${zone.name}: ${message}`);
        logger.error('Zone analytics sync failed', { zone: zone.name, error: message });
      }
    }

    return {
      success: errors.length === 0,
      zones_synced: syncedCount,
      zones_total: zones.length,
      errors: errors.length > 0 ? errors : undefined,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Analytics sync failed', { error: message });
    return reply.status(500).send({ error: message });
  }
});
```

---

## Configuration Requirements

### Environment Variables

**Required for Cloudflare**:
```bash
CDN_PROVIDER=cloudflare
CDN_CLOUDFLARE_API_TOKEN=your_cloudflare_api_token
CDN_CLOUDFLARE_ZONE_IDS=zone1_id,zone2_id  # Comma-separated
```

**Required for BunnyCDN**:
```bash
CDN_PROVIDER=bunnycdn
CDN_BUNNYCDN_API_KEY=your_bunnycdn_api_key
CDN_BUNNYCDN_ACCOUNT_ID=your_account_id
CDN_BUNNYCDN_PULL_ZONE_IDS=zone1_id,zone2_id
```

**Optional**:
```bash
CDN_SIGNING_KEY=random_secret_for_signed_urls
CDN_SIGNED_URL_TTL=3600
CDN_PURGE_BATCH_SIZE=500
CDN_ANALYTICS_SYNC_INTERVAL=3600000  # 1 hour in ms
```

### Get API Credentials

**Cloudflare**:
1. Log in to Cloudflare Dashboard
2. Go to **My Profile** → **API Tokens**
3. Create token with permissions:
   - Zone → Cache Purge → Purge
   - Zone → Analytics → Read
4. Copy the token to `CDN_CLOUDFLARE_API_TOKEN`
5. Get Zone IDs from each domain's overview page

**BunnyCDN**:
1. Log in to BunnyCDN Panel
2. Go to **Account** → **API**
3. Copy **Account API Key** to `CDN_BUNNYCDN_API_KEY`
4. Account ID is in the URL: `https://panel.bunny.net/account/{ACCOUNT_ID}`
5. Get Pull Zone IDs from **CDN** → pull zone details

---

## Testing Instructions

### 1. Install Dependencies

```bash
cd plugins/cdn/ts
pnpm install
pnpm add axios cloudflare
```

### 2. Build the Plugin

```bash
pnpm build
```

### 3. Configure Environment

Create `.env` in the plugin directory:

```bash
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself_db
CDN_API_KEY=test-key
CDN_PORT=3036

# Choose your provider
CDN_PROVIDER=cloudflare
CDN_CLOUDFLARE_API_TOKEN=your_token_here
CDN_CLOUDFLARE_ZONE_IDS=your_zone_id
```

### 4. Start the Server

```bash
pnpm start
```

### 5. Test the API

**Create a Zone**:
```bash
curl -X POST http://localhost:3036/api/zones \
  -H "Content-Type: application/json" \
  -H "X-API-Key: test-key" \
  -d '{
    "provider": "cloudflare",
    "zone_id": "your_cloudflare_zone_id",
    "name": "example.com",
    "domain": "example.com"
  }'
```

**Purge Cache**:
```bash
curl -X POST http://localhost:3036/api/purge \
  -H "Content-Type: application/json" \
  -H "X-API-Key: test-key" \
  -d '{
    "zone_id": "zone_uuid_from_above",
    "urls": ["https://example.com/image.jpg"],
    "requested_by": "admin"
  }'
```

**Sync Analytics**:
```bash
curl -X POST http://localhost:3036/sync \
  -H "X-API-Key: test-key"
```

**Generate Signed URL**:
```bash
curl -X POST http://localhost:3036/api/sign \
  -H "Content-Type: application/json" \
  -H "X-API-Key: test-key" \
  -d '{
    "zone_id": "zone_uuid",
    "url": "https://example.com/private-video.mp4",
    "ttl": 3600,
    "ip_restriction": "1.2.3.4"
  }'
```

---

## Activation Checklist

- [ ] Install provider packages: `pnpm add axios`
- [ ] Create `providers.ts` with implementation above
- [ ] Update `server.ts` to use providers
- [ ] Add API credentials to `.env`
- [ ] Build plugin: `pnpm build`
- [ ] Start server: `pnpm start`
- [ ] Test API endpoints
- [ ] Set up analytics sync cron job (optional)

---

## Next Steps

1. **Scheduled Analytics Sync**: Add cron job to call `/sync` endpoint hourly
2. **Webhook Support**: Implement Cloudflare/BunnyCDN webhooks for real-time updates
3. **Multi-Zone Support**: Iterate over all configured zones during sync
4. **Error Alerting**: Send notifications when purge/sync fails
5. **Dashboard**: Build admin UI for viewing analytics and managing zones

---

## Support

- **Cloudflare API Docs**: https://developers.cloudflare.com/api/
- **BunnyCDN API Docs**: https://docs.bunny.net/reference/api-overview
- **Plugin Issues**: File issues in nself-plugins repository
