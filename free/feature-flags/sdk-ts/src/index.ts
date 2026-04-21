/**
 * @nself/feature-flags-client
 *
 * Thin TypeScript/React SDK over the feature-flags plugin REST API.
 *
 * Features:
 * - 60s LRU cache with bounded size (prevents memory leak on long-running apps)
 * - React hook: useFlag(key, defaultValue)
 * - Node helper: evaluateFlag(key, ctx)
 * - Subscribe API: subscribeFlagChanges() via Redis pub/sub relay
 */

import { useEffect, useState } from 'react'

// ---- Types ------------------------------------------------------------------

export type FlagValue = boolean | string | number | Record<string, unknown>

export interface EvaluateRequest {
  flag_key: string
  user_id?: string
  context?: Record<string, unknown>
}

export interface EvaluateResponse {
  flag_key: string
  value: FlagValue
  enabled: boolean
  reason: string
}

export interface FeatureFlagsClientOptions {
  /** Base URL for the feature-flags plugin. Default: http://127.0.0.1:3305/v1 */
  baseURL?: string
  /** Cache TTL in milliseconds. Default: 60000 (60 seconds) */
  cacheTTL?: number
  /** Max number of cached entries. Default: 500 */
  cacheMaxSize?: number
  /** WebSocket URL for pub/sub invalidation relay. If omitted, pub/sub is disabled. */
  pubsubURL?: string
}

// ---- LRU Cache --------------------------------------------------------------

interface CacheEntry {
  value: FlagValue
  expiresAt: number
  lruKey: number
}

let lruCounter = 0

class BoundedLRUCache {
  private entries = new Map<string, CacheEntry>()
  private readonly maxSize: number
  private readonly ttlMs: number

  constructor(maxSize: number, ttlMs: number) {
    this.maxSize = maxSize
    this.ttlMs = ttlMs
  }

  get(key: string): FlagValue | undefined {
    const entry = this.entries.get(key)
    if (!entry) return undefined
    if (Date.now() > entry.expiresAt) {
      this.entries.delete(key)
      return undefined
    }
    // Update LRU order
    entry.lruKey = ++lruCounter
    return entry.value
  }

  set(key: string, value: FlagValue): void {
    if (this.entries.size >= this.maxSize && !this.entries.has(key)) {
      // Evict oldest (lowest lruKey)
      let oldestKey = ''
      let oldestLru = Infinity
      for (const [k, e] of this.entries) {
        if (e.lruKey < oldestLru) {
          oldestLru = e.lruKey
          oldestKey = k
        }
      }
      if (oldestKey) this.entries.delete(oldestKey)
    }
    this.entries.set(key, {
      value,
      expiresAt: Date.now() + this.ttlMs,
      lruKey: ++lruCounter,
    })
  }

  invalidate(key: string): void {
    this.entries.delete(key)
  }

  clear(): void {
    this.entries.clear()
  }
}

// ---- Client -----------------------------------------------------------------

const DEFAULT_BASE_URL = 'http://127.0.0.1:3305/v1'
const DEFAULT_TTL = 60_000
const DEFAULT_MAX_SIZE = 500

let globalClient: FeatureFlagsClient | null = null

export class FeatureFlagsClient {
  private readonly baseURL: string
  private readonly cache: BoundedLRUCache
  private readonly subscribers = new Set<(key: string) => void>()
  private ws: WebSocket | null = null
  private readonly pubsubURL: string | undefined

  constructor(opts: FeatureFlagsClientOptions = {}) {
    this.baseURL = opts.baseURL ?? DEFAULT_BASE_URL
    this.cache = new BoundedLRUCache(
      opts.cacheMaxSize ?? DEFAULT_MAX_SIZE,
      opts.cacheTTL ?? DEFAULT_TTL,
    )
    this.pubsubURL = opts.pubsubURL
    if (this.pubsubURL) {
      this.connectPubSub()
    }
  }

  /** Evaluate a feature flag for the given user context. Returns defaultValue on any error. */
  async evaluate<T extends FlagValue>(
    key: string,
    defaultValue: T,
    ctx?: EvaluateRequest,
  ): Promise<T> {
    const cached = this.cache.get(key)
    if (cached !== undefined) {
      return cached as T
    }
    try {
      const resp = await fetch(`${this.baseURL}/evaluate`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ flag_key: key, ...ctx }),
      })
      if (!resp.ok) return defaultValue
      const data: EvaluateResponse = await resp.json()
      const value = data.value as T
      this.cache.set(key, value)
      return value
    } catch {
      return defaultValue
    }
  }

  /** Force-refresh a flag, bypassing the cache. */
  async forceRefresh<T extends FlagValue>(key: string, defaultValue: T): Promise<T> {
    this.cache.invalidate(key)
    return this.evaluate<T>(key, defaultValue)
  }

  /** Invalidate a cached flag (called by pub/sub listener). */
  invalidate(key: string): void {
    this.cache.invalidate(key)
    for (const sub of this.subscribers) {
      try { sub(key) } catch { /* ignore subscriber errors */ }
    }
  }

  /** Subscribe to flag-change notifications from pub/sub. */
  onFlagChange(handler: (key: string) => void): () => void {
    this.subscribers.add(handler)
    return () => this.subscribers.delete(handler)
  }

  private connectPubSub(): void {
    if (typeof WebSocket === 'undefined') return
    const connect = () => {
      try {
        this.ws = new WebSocket(this.pubsubURL!)
        this.ws.onmessage = (ev) => {
          try {
            const { key } = JSON.parse(ev.data as string)
            if (key) this.invalidate(key)
          } catch { /* malformed */ }
        }
        this.ws.onerror = () => { /* silent — fall back to TTL expiry */ }
        this.ws.onclose = () => {
          // Reconnect after 5s on unexpected close
          setTimeout(connect, 5_000)
        }
      } catch { /* WebSocket unavailable — fall back to TTL */ }
    }
    connect()
  }
}

// ---- Singleton helpers ------------------------------------------------------

/** Initialize or retrieve the global client instance. */
export function initFeatureFlags(opts?: FeatureFlagsClientOptions): FeatureFlagsClient {
  globalClient = new FeatureFlagsClient(opts)
  return globalClient
}

function getClient(): FeatureFlagsClient {
  if (!globalClient) {
    globalClient = new FeatureFlagsClient()
  }
  return globalClient
}

// ---- Node helper ------------------------------------------------------------

/**
 * Evaluate a feature flag (Node.js / non-React context).
 *
 * @param key - Flag key (e.g. 'ai.safety.jailbreak_filter')
 * @param defaultValue - Returned on error or when flag is absent
 * @param ctx - Optional user context for rule evaluation
 */
export async function evaluateFlag<T extends FlagValue>(
  key: string,
  defaultValue: T,
  ctx?: Omit<EvaluateRequest, 'flag_key'>,
): Promise<T> {
  return getClient().evaluate(key, defaultValue, ctx)
}

/**
 * Subscribe to flag-change events from the pub/sub relay.
 *
 * @param handler - Called with the invalidated flag key (<5s after kill/disable)
 * @returns Unsubscribe function
 */
export function subscribeFlagChanges(handler: (key: string) => void): () => void {
  return getClient().onFlagChange(handler)
}

// ---- React hook -------------------------------------------------------------

/**
 * React hook: evaluate a feature flag.
 *
 * Fetches the flag on mount (with 60s LRU cache). Re-evaluates when the
 * pub/sub relay fires a change for this key.
 *
 * @param key - Flag key
 * @param defaultValue - Returned while loading or on error
 * @param ctx - Optional user context
 */
export function useFlag<T extends FlagValue>(
  key: string,
  defaultValue: T,
  ctx?: Omit<EvaluateRequest, 'flag_key'>,
): T {
  const [value, setValue] = useState<T>(defaultValue)

  useEffect(() => {
    let cancelled = false
    const client = getClient()

    const load = async () => {
      const v = await client.evaluate<T>(key, defaultValue, ctx)
      if (!cancelled) setValue(v)
    }

    load()

    // Re-load on pub/sub invalidation for this specific key
    const unsub = client.onFlagChange((changedKey) => {
      if (changedKey === key) load()
    })

    return () => {
      cancelled = true
      unsub()
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key])

  return value
}
