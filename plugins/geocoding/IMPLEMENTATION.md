# Geocoding Plugin - Full Implementation Guide

## Overview

The Geocoding plugin provides forward/reverse geocoding, place search, autocomplete, and geofencing capabilities with support for **Google Maps**, **Mapbox**, and **Nominatim** (OSM).

## Current Status

**Infrastructure Status**: ✅ Complete (database, API endpoints, caching, geofences)
**Provider Integration Status**: ⚠️ Placeholder (requires API implementation)

## What's Already Built

- ✅ Complete database schema for cache, places, geofences, events
- ✅ Full REST API with all endpoints
- ✅ Multi-provider caching system
- ✅ Geofence evaluation with haversine distance calculation
- ✅ Batch geocoding support
- ✅ Multi-tenant support

## What Needs Implementation

**Provider API Integration** - The actual geocoding calls in:
- `forwardGeocode()` - Address → Coordinates
- `reverseGeocode()` - Coordinates → Address
- `autocomplete()` - Place search suggestions
- `placeSearch()` - Find nearby places

---

## Required Packages

Base dependencies **already installed**:

```json
{
  "@nself/plugin-utils": "file:../../../shared",
  "fastify": "^4.24.0",
  "@fastify/cors": "^8.4.0",
  "pg": "^8.11.3"
}
```

### Additional Packages for Provider Integration

```bash
# Google Maps client
pnpm add @googlemaps/google-maps-services-js

# Mapbox SDK
pnpm add @mapbox/mapbox-sdk

# Nominatim client (OpenStreetMap)
pnpm add nominatim-client

# Or use generic HTTP client for all
pnpm add axios
```

---

## Complete Implementation Code

### 1. Provider Integration Module

Create `ts/src/providers.ts`:

```typescript
/**
 * Geocoding Provider Integration
 * Supports Google Maps, Mapbox, and Nominatim (OSM)
 */

import { Client as GoogleMapsClient } from '@googlemaps/google-maps-services-js';
import MapboxClient from '@mapbox/mapbox-sdk/services/geocoding.js';
import axios from 'axios';
import type { GeoResult } from './types.js';

export interface GeocodeRequest {
  address?: string;
  city?: string;
  state?: string;
  country?: string;
  lat?: number;
  lng?: number;
}

export interface AutocompleteRequest {
  input: string;
  lat?: number;
  lng?: number;
  radius?: number;
  types?: string[];
}

export interface PlaceSearchRequest {
  query: string;
  lat?: number;
  lng?: number;
  radius?: number;
  category?: string;
}

/**
 * Google Maps Provider
 */
export class GoogleMapsProvider {
  private client: GoogleMapsClient;
  private apiKey: string;

  constructor(apiKey: string) {
    this.apiKey = apiKey;
    this.client = new GoogleMapsClient({});
  }

  /**
   * Forward geocode - address to coordinates
   */
  async forwardGeocode(request: GeocodeRequest): Promise<GeoResult[]> {
    const address = [request.address, request.city, request.state, request.country]
      .filter(Boolean)
      .join(', ');

    try {
      const response = await this.client.geocode({
        params: {
          address,
          key: this.apiKey,
        },
      });

      return response.data.results.map(result => this.parseGoogleResult(result));
    } catch (error) {
      throw new Error(`Google Maps geocode failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Reverse geocode - coordinates to address
   */
  async reverseGeocode(lat: number, lng: number): Promise<GeoResult[]> {
    try {
      const response = await this.client.reverseGeocode({
        params: {
          latlng: { lat, lng },
          key: this.apiKey,
        },
      });

      return response.data.results.map(result => this.parseGoogleResult(result));
    } catch (error) {
      throw new Error(`Google Maps reverse geocode failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Place autocomplete
   */
  async autocomplete(request: AutocompleteRequest): Promise<Array<{ description: string; place_id: string }>> {
    try {
      const response = await this.client.placeAutocomplete({
        params: {
          input: request.input,
          key: this.apiKey,
          location: request.lat && request.lng ? { lat: request.lat, lng: request.lng } : undefined,
          radius: request.radius,
          types: request.types?.join('|'),
        },
      });

      return response.data.predictions.map(pred => ({
        description: pred.description,
        place_id: pred.place_id,
      }));
    } catch (error) {
      throw new Error(`Google autocomplete failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Place search
   */
  async placeSearch(request: PlaceSearchRequest): Promise<GeoResult[]> {
    try {
      const response = await this.client.findPlaceFromText({
        params: {
          input: request.query,
          inputtype: 'textquery',
          key: this.apiKey,
          locationbias: request.lat && request.lng
            ? `circle:${request.radius ?? 5000}@${request.lat},${request.lng}`
            : undefined,
        },
      });

      return response.data.candidates.map(place => ({
        lat: place.geometry?.location?.lat ?? 0,
        lng: place.geometry?.location?.lng ?? 0,
        formatted_address: place.formatted_address,
        place_id: place.place_id,
        place_type: place.types?.[0],
        provider: 'google',
      }));
    } catch (error) {
      throw new Error(`Google place search failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Parse Google Maps result to GeoResult
   */
  private parseGoogleResult(result: Record<string, unknown>): GeoResult {
    const components = (result.address_components as Array<Record<string, unknown>>) ?? [];
    const geometry = result.geometry as Record<string, unknown>;
    const location = geometry.location as { lat: number; lng: number };

    const getComponent = (type: string): string | undefined => {
      return components.find(c => (c.types as string[]).includes(type))?.long_name as string | undefined;
    };

    const getShortComponent = (type: string): string | undefined => {
      return components.find(c => (c.types as string[]).includes(type))?.short_name as string | undefined;
    };

    return {
      lat: location.lat,
      lng: location.lng,
      formatted_address: result.formatted_address as string,
      street_number: getComponent('street_number'),
      street_name: getComponent('route'),
      city: getComponent('locality') ?? getComponent('postal_town'),
      state: getComponent('administrative_area_level_1'),
      state_code: getShortComponent('administrative_area_level_1'),
      country: getComponent('country'),
      country_code: getShortComponent('country'),
      postal_code: getComponent('postal_code'),
      place_id: result.place_id as string,
      place_type: (result.types as string[])?.[0],
      accuracy: this.mapGoogleAccuracy(geometry.location_type as string),
      provider: 'google',
    };
  }

  private mapGoogleAccuracy(locationType: string): GeoResult['accuracy'] {
    switch (locationType) {
      case 'ROOFTOP': return 'rooftop';
      case 'RANGE_INTERPOLATED': return 'interpolated';
      case 'GEOMETRIC_CENTER': return 'geometric_center';
      case 'APPROXIMATE': return 'approximate';
      default: return 'approximate';
    }
  }
}

/**
 * Mapbox Provider
 */
export class MapboxProvider {
  private client: ReturnType<typeof MapboxClient>;

  constructor(accessToken: string) {
    this.client = MapboxClient({ accessToken });
  }

  /**
   * Forward geocode
   */
  async forwardGeocode(request: GeocodeRequest): Promise<GeoResult[]> {
    const query = [request.address, request.city, request.state, request.country]
      .filter(Boolean)
      .join(', ');

    try {
      const response = await this.client.forwardGeocode({
        query,
        limit: 5,
      }).send();

      return response.body.features.map(feature => this.parseMapboxFeature(feature));
    } catch (error) {
      throw new Error(`Mapbox geocode failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Reverse geocode
   */
  async reverseGeocode(lat: number, lng: number): Promise<GeoResult[]> {
    try {
      const response = await this.client.reverseGeocode({
        query: [lng, lat], // Mapbox uses [lng, lat]
        limit: 5,
      }).send();

      return response.body.features.map(feature => this.parseMapboxFeature(feature));
    } catch (error) {
      throw new Error(`Mapbox reverse geocode failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Parse Mapbox feature to GeoResult
   */
  private parseMapboxFeature(feature: Record<string, unknown>): GeoResult {
    const geometry = feature.geometry as { coordinates: [number, number] };
    const context = (feature.context as Array<Record<string, unknown>>) ?? [];

    const getContext = (id: string): string | undefined => {
      return context.find(c => (c.id as string).startsWith(id))?.text as string | undefined;
    };

    return {
      lat: geometry.coordinates[1],
      lng: geometry.coordinates[0],
      formatted_address: feature.place_name as string,
      street_number: (feature.address as string),
      street_name: (feature.text as string),
      city: getContext('place'),
      state: getContext('region'),
      country: getContext('country'),
      postal_code: getContext('postcode'),
      place_id: feature.id as string,
      place_type: (feature.place_type as string[])?.[0],
      accuracy: feature.accuracy ? 'rooftop' : 'approximate',
      provider: 'mapbox',
    };
  }
}

/**
 * Nominatim (OpenStreetMap) Provider
 */
export class NominatimProvider {
  private baseUrl: string;
  private email: string;

  constructor(baseUrl = 'https://nominatim.openstreetmap.org', email: string) {
    this.baseUrl = baseUrl;
    this.email = email;
  }

  /**
   * Forward geocode
   */
  async forwardGeocode(request: GeocodeRequest): Promise<GeoResult[]> {
    const query = [request.address, request.city, request.state, request.country]
      .filter(Boolean)
      .join(', ');

    try {
      const response = await axios.get(`${this.baseUrl}/search`, {
        params: {
          q: query,
          format: 'json',
          addressdetails: 1,
          limit: 5,
          email: this.email,
        },
      });

      return response.data.map((result: Record<string, unknown>) => this.parseNominatimResult(result));
    } catch (error) {
      throw new Error(`Nominatim geocode failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Reverse geocode
   */
  async reverseGeocode(lat: number, lng: number): Promise<GeoResult[]> {
    try {
      const response = await axios.get(`${this.baseUrl}/reverse`, {
        params: {
          lat,
          lon: lng,
          format: 'json',
          addressdetails: 1,
          email: this.email,
        },
      });

      return [this.parseNominatimResult(response.data)];
    } catch (error) {
      throw new Error(`Nominatim reverse geocode failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Parse Nominatim result to GeoResult
   */
  private parseNominatimResult(result: Record<string, unknown>): GeoResult {
    const address = (result.address as Record<string, string>) ?? {};

    return {
      lat: parseFloat(result.lat as string),
      lng: parseFloat(result.lon as string),
      formatted_address: result.display_name as string,
      street_number: address.house_number,
      street_name: address.road,
      city: address.city ?? address.town ?? address.village,
      state: address.state,
      country: address.country,
      postal_code: address.postcode,
      place_id: result.place_id as string,
      place_type: result.type as string,
      accuracy: result.importance ? 'rooftop' : 'approximate',
      provider: 'nominatim',
    };
  }
}

/**
 * Provider factory
 */
export function createGeocodingProvider(
  provider: string,
  config: Record<string, string>
): GoogleMapsProvider | MapboxProvider | NominatimProvider {
  switch (provider.toLowerCase()) {
    case 'google':
      if (!config.GEOCODING_GOOGLE_API_KEY) {
        throw new Error('GEOCODING_GOOGLE_API_KEY is required for Google Maps provider');
      }
      return new GoogleMapsProvider(config.GEOCODING_GOOGLE_API_KEY);

    case 'mapbox':
      if (!config.GEOCODING_MAPBOX_ACCESS_TOKEN) {
        throw new Error('GEOCODING_MAPBOX_ACCESS_TOKEN is required for Mapbox provider');
      }
      return new MapboxProvider(config.GEOCODING_MAPBOX_ACCESS_TOKEN);

    case 'nominatim':
      if (!config.GEOCODING_NOMINATIM_EMAIL) {
        throw new Error('GEOCODING_NOMINATIM_EMAIL is required for Nominatim provider');
      }
      return new NominatimProvider(
        config.GEOCODING_NOMINATIM_URL ?? 'https://nominatim.openstreetmap.org',
        config.GEOCODING_NOMINATIM_EMAIL
      );

    default:
      throw new Error(`Unsupported geocoding provider: ${provider}`);
  }
}
```

### 2. Update Server to Use Providers

Modify `ts/src/server.ts` to integrate providers:

```typescript
import { createGeocodingProvider } from './providers.js';

// In createServer() after config load:
const geocodingProviders = fullConfig.providers.map(provider =>
  createGeocodingProvider(provider, process.env as Record<string, string>)
);

// Helper to try providers in order until one succeeds
async function geocodeWithFallback<T>(
  operation: (provider: ReturnType<typeof createGeocodingProvider>) => Promise<T>
): Promise<T> {
  let lastError: Error | undefined;

  for (const provider of geocodingProviders) {
    try {
      return await operation(provider);
    } catch (error) {
      lastError = error instanceof Error ? error : new Error('Unknown error');
      continue;
    }
  }

  throw lastError ?? new Error('All geocoding providers failed');
}

// Update forward geocode endpoint (around line 117):
app.post('/api/geocode', async (request, reply) => {
  try {
    const body = request.body as ForwardGeocodeRequest;

    if (!body.address) {
      return reply.status(400).send({ error: 'Address is required' });
    }

    const queryText = [body.address, body.city, body.state, body.country]
      .filter(Boolean)
      .join(', ');

    // Check cache first
    if (fullConfig.cacheEnabled) {
      const cached = await scopedDb(request).getCachedGeocodeAnyProvider('forward', queryText);
      if (cached) {
        const result: GeoResult = {
          lat: cached.lat ?? 0,
          lng: cached.lng ?? 0,
          formatted_address: cached.formatted_address,
          street_number: cached.street_number,
          street_name: cached.street_name,
          city: cached.city,
          state: cached.state,
          state_code: cached.state_code,
          country: cached.country,
          country_code: cached.country_code,
          postal_code: cached.postal_code,
          place_id: cached.place_id,
          place_type: cached.place_type,
          accuracy: cached.accuracy as GeoResult['accuracy'],
          provider: cached.provider,
          cached: true,
        };
        return { data: [result] };
      }
    }

    // Call provider with fallback
    const results = await geocodeWithFallback(provider =>
      provider.forwardGeocode({
        address: body.address,
        city: body.city,
        state: body.state,
        country: body.country,
      })
    );

    // Cache the result
    if (fullConfig.cacheEnabled && results.length > 0) {
      const result = results[0]!;
      await scopedDb(request).cacheGeocodeResult('forward', queryText, result.provider, result);
    }

    return { data: results };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Forward geocode failed', { error: message });
    return reply.status(500).send({ error: message });
  }
});

// Update reverse geocode endpoint (around line 168):
app.post('/api/reverse', async (request, reply) => {
  try {
    const body = request.body as ReverseGeocodeRequest;

    if (body.lat === undefined || body.lng === undefined) {
      return reply.status(400).send({ error: 'lat and lng are required' });
    }

    const queryText = `${body.lat},${body.lng}`;

    // Check cache
    if (fullConfig.cacheEnabled) {
      const cached = await scopedDb(request).getCachedGeocodeAnyProvider('reverse', queryText);
      if (cached) {
        const result: GeoResult = {
          lat: cached.lat ?? 0,
          lng: cached.lng ?? 0,
          formatted_address: cached.formatted_address,
          street_number: cached.street_number,
          street_name: cached.street_name,
          city: cached.city,
          state: cached.state,
          state_code: cached.state_code,
          country: cached.country,
          country_code: cached.country_code,
          postal_code: cached.postal_code,
          place_id: cached.place_id,
          place_type: cached.place_type,
          accuracy: cached.accuracy as GeoResult['accuracy'],
          provider: cached.provider,
          cached: true,
        };
        return { data: [result] };
      }
    }

    // Call provider
    const results = await geocodeWithFallback(provider =>
      provider.reverseGeocode(body.lat, body.lng)
    );

    // Cache
    if (fullConfig.cacheEnabled && results.length > 0) {
      const result = results[0]!;
      await scopedDb(request).cacheGeocodeResult('reverse', queryText, result.provider, result);
    }

    return { data: results };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Reverse geocode failed', { error: message });
    return reply.status(500).send({ error: message });
  }
});

// Update autocomplete endpoint (around line 241):
app.post('/api/autocomplete', async (request, reply) => {
  try {
    const body = request.body as AutocompleteRequest;

    if (!body.input) {
      return reply.status(400).send({ error: 'Input is required' });
    }

    // Only Google Maps supports autocomplete currently
    const googleProvider = geocodingProviders.find(p => p instanceof GoogleMapsProvider);
    if (!googleProvider) {
      return reply.status(400).send({ error: 'Google Maps provider required for autocomplete' });
    }

    const results = await (googleProvider as GoogleMapsProvider).autocomplete({
      input: body.input,
      lat: body.lat,
      lng: body.lng,
      radius: body.radius,
      types: body.types,
    });

    return { data: results };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Autocomplete failed', { error: message });
    return reply.status(500).send({ error: message });
  }
});
```

---

## Configuration Requirements

### Environment Variables

**Option 1: Google Maps**:
```bash
GEOCODING_PROVIDERS=google
GEOCODING_GOOGLE_API_KEY=your_google_maps_api_key
GEOCODING_CACHE_ENABLED=true
GEOCODING_CACHE_TTL_DAYS=30
```

**Option 2: Mapbox**:
```bash
GEOCODING_PROVIDERS=mapbox
GEOCODING_MAPBOX_ACCESS_TOKEN=your_mapbox_access_token
GEOCODING_CACHE_ENABLED=true
```

**Option 3: Nominatim (Free/OSM)**:
```bash
GEOCODING_PROVIDERS=nominatim
GEOCODING_NOMINATIM_URL=https://nominatim.openstreetmap.org
GEOCODING_NOMINATIM_EMAIL=your_email@example.com
GEOCODING_CACHE_ENABLED=true
```

**Multi-Provider Fallback**:
```bash
GEOCODING_PROVIDERS=google,mapbox,nominatim
GEOCODING_GOOGLE_API_KEY=xxx
GEOCODING_MAPBOX_ACCESS_TOKEN=yyy
GEOCODING_NOMINATIM_EMAIL=your_email@example.com
```

### Get API Credentials

**Google Maps**:
1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create project and enable **Geocoding API** and **Places API**
3. Create API key in **Credentials**
4. Restrict key to your IP/domain

**Mapbox**:
1. Sign up at [mapbox.com](https://www.mapbox.com/)
2. Go to **Account** → **Access tokens**
3. Create token with `geocoding:read` scope

**Nominatim**:
- Free, no API key needed
- **Must provide valid email** in User-Agent (OSM policy)
- Rate limited to 1 req/sec

---

## Testing Instructions

### 1. Install Dependencies

```bash
cd plugins/geocoding/ts
pnpm install
pnpm add @googlemaps/google-maps-services-js @mapbox/mapbox-sdk axios
```

### 2. Build

```bash
pnpm build
```

### 3. Configure

Create `.env`:

```bash
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself_db
GEOCODING_API_KEY=test-key
GEOCODING_PORT=3203

GEOCODING_PROVIDERS=google
GEOCODING_GOOGLE_API_KEY=your_api_key_here
GEOCODING_CACHE_ENABLED=true
GEOCODING_CACHE_TTL_DAYS=30
```

### 4. Start Server

```bash
pnpm start
```

### 5. Test API

**Forward Geocode**:
```bash
curl -X POST http://localhost:3203/api/geocode \
  -H "Content-Type: application/json" \
  -H "X-API-Key: test-key" \
  -d '{
    "address": "1600 Amphitheatre Parkway",
    "city": "Mountain View",
    "state": "CA",
    "country": "USA"
  }'
```

**Reverse Geocode**:
```bash
curl -X POST http://localhost:3203/api/reverse \
  -H "Content-Type: application/json" \
  -H "X-API-Key: test-key" \
  -d '{"lat": 37.4224764, "lng": -122.0842499}'
```

**Autocomplete**:
```bash
curl -X POST http://localhost:3203/api/autocomplete \
  -H "Content-Type: application/json" \
  -H "X-API-Key: test-key" \
  -d '{
    "input": "Golden Gate Bridge",
    "lat": 37.7749,
    "lng": -122.4194,
    "radius": 50000
  }'
```

**Create Geofence**:
```bash
curl -X POST http://localhost:3203/api/geofences \
  -H "Content-Type: application/json" \
  -H "X-API-Key: test-key" \
  -d '{
    "name": "Home",
    "center_lat": 37.4224764,
    "center_lng": -122.0842499,
    "radius_meters": 100,
    "is_active": true
  }'
```

**Check Point in Geofence**:
```bash
curl -X POST http://localhost:3203/api/geofences/evaluate \
  -H "Content-Type: application/json" \
  -H "X-API-Key: test-key" \
  -d '{
    "lat": 37.4224764,
    "lng": -122.0842499,
    "entity_id": "user_123",
    "entity_type": "user"
  }'
```

---

## Activation Checklist

- [ ] Install provider packages: `pnpm add @googlemaps/google-maps-services-js @mapbox/mapbox-sdk axios`
- [ ] Create `providers.ts` with implementation
- [ ] Update `server.ts` to use providers
- [ ] Add API credentials to `.env`
- [ ] Build: `pnpm build`
- [ ] Start: `pnpm start`
- [ ] Test geocoding endpoints
- [ ] Verify cache working (check database)
- [ ] Test geofences

---

## Cost Considerations

**Google Maps Pricing** (as of 2024):
- Geocoding API: $5 per 1000 requests
- Places API: $17 per 1000 requests
- **Monthly credit**: $200 free

**Mapbox Pricing**:
- 100,000 free requests/month
- $0.50 per 1000 thereafter

**Nominatim**:
- **Free** but rate-limited (1 req/sec)
- Must provide email in User-Agent
- Consider self-hosting for heavy usage

**Recommendation**: Use caching aggressively to minimize API costs.

---

## Support

- **Google Maps API**: https://developers.google.com/maps/documentation/geocoding
- **Mapbox API**: https://docs.mapbox.com/api/search/geocoding/
- **Nominatim**: https://nominatim.org/release-docs/latest/api/Overview/
