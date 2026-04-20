# @nself/feature-flags-client

TypeScript SDK for the nself feature-flags plugin.

## Install

```bash
pnpm add @nself/feature-flags-client
```

## Usage

### React hook

```tsx
import { useFlag } from '@nself/feature-flags-client'

function MyComponent() {
  const enabled = useFlag('ai.safety.jailbreak_filter', false)
  if (!enabled) return null
  return <AdvancedFeature />
}
```

### Node helper

```ts
import { evaluateFlag } from '@nself/feature-flags-client'

const enabled = await evaluateFlag('ai.safety.jailbreak_filter', false, {
  user_id: 'u_123',
  context: { country: 'US' },
})
```

### Subscribe to real-time invalidation

```ts
import { initFeatureFlags, subscribeFlagChanges } from '@nself/feature-flags-client'

// Initialize once at app startup (configure pubsub relay if available)
initFeatureFlags({
  baseURL: 'http://127.0.0.1:3305/v1',
  pubsubURL: 'ws://127.0.0.1:3305/ws/flags',
})

// Subscribe to flag changes (<5s after kill/disable)
const unsubscribe = subscribeFlagChanges((key) => {
  console.log(`Flag ${key} was invalidated`)
})

// Later:
unsubscribe()
```

## Cache

- 60s LRU cache with configurable max size (default 500 entries)
- `forceRefresh(key, defaultValue)` bypasses cache for one call
- Invalidated via pub/sub when Redis pubsub relay is configured
- Falls back to 60s TTL expiry if pub/sub is unavailable

## Options

```ts
initFeatureFlags({
  baseURL: 'http://127.0.0.1:3305/v1', // Plugin REST base URL
  cacheTTL: 60_000,                     // Cache TTL in ms (default: 60000)
  cacheMaxSize: 500,                    // Max cached entries (default: 500)
  pubsubURL: 'ws://...',                // WebSocket relay for instant invalidation
})
```

## Requirements

- nself feature-flags plugin installed and running
- React 18+ (for `useFlag` hook; optional for Node helpers)
