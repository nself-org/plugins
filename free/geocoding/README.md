# geocoding

Geocoding and location services plugin

## Installation

```bash
nself plugin install geocoding
```

## Configuration

See plugin.json for environment variables and configuration options.

## Current Features

### ✅ Infrastructure
- Plugin framework and API structure
- Database schema for geofences and places
- Configuration management

## Planned Features

### 🔄 Google Maps API Integration
- Forward geocoding (address → coordinates)
- Reverse geocoding (coordinates → address)
- Place autocomplete
- Place search
- Geofence management
- Distance calculations

## Current API Endpoints

All endpoints are defined but return empty/default responses until Google Maps API integration is implemented:

| Endpoint | Method | Status | Returns |
|----------|--------|--------|---------|
| `/geocode` | POST | Placeholder | `{"lat": 0, "lng": 0}` |
| `/reverse-geocode` | POST | Placeholder | `{"address": ""}` |
| `/autocomplete` | POST | Placeholder | `{"suggestions": []}` |
| `/search-places` | POST | Placeholder | `{"places": []}` |
| `/geofences` | GET/POST | Placeholder | Empty arrays |
| `/distance` | POST | Placeholder | `{"distance": 0}` |

## Usage

**Note:** The plugin is currently in infrastructure-only state. Geocoding features require Google Maps API integration (coming soon).

For available CLI commands, see `plugin.json`.

## License

See LICENSE file in repository root.
