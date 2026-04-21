/**
 * @nself/feature-flags-client — unit tests
 */

import { FeatureFlagsClient } from './index.js'

// Minimal fetch mock
const mockFetch = jest.fn()
global.fetch = mockFetch as unknown as typeof fetch

function mockEvaluateResponse(value: unknown) {
  mockFetch.mockResolvedValueOnce({
    ok: true,
    json: async () => ({ flag_key: 'test.flag', value, enabled: true, reason: 'rule_match' }),
  })
}

beforeEach(() => {
  mockFetch.mockClear()
})

describe('FeatureFlagsClient', () => {
  it('evaluate: calls REST and returns value', async () => {
    mockEvaluateResponse(true)
    const client = new FeatureFlagsClient({ baseURL: 'http://localhost:3305/v1' })
    const result = await client.evaluate('test.flag', false)
    expect(result).toBe(true)
    expect(mockFetch).toHaveBeenCalledTimes(1)
  })

  it('evaluate: cache hit avoids second REST call', async () => {
    mockEvaluateResponse(true)
    const client = new FeatureFlagsClient({ baseURL: 'http://localhost:3305/v1' })
    await client.evaluate('test.flag', false)
    const result = await client.evaluate('test.flag', false)
    expect(result).toBe(true)
    expect(mockFetch).toHaveBeenCalledTimes(1) // Only one fetch
  })

  it('evaluate: returns defaultValue on network error', async () => {
    mockFetch.mockRejectedValueOnce(new Error('network error'))
    const client = new FeatureFlagsClient({ baseURL: 'http://localhost:3305/v1' })
    const result = await client.evaluate('test.flag', 'fallback')
    expect(result).toBe('fallback')
  })

  it('invalidate: clears cache and notifies subscribers', async () => {
    mockEvaluateResponse(true)
    mockEvaluateResponse(false) // second call after invalidation
    const client = new FeatureFlagsClient({ baseURL: 'http://localhost:3305/v1' })
    await client.evaluate('test.flag', false)

    const notifications: string[] = []
    client.onFlagChange((key) => notifications.push(key))

    client.invalidate('test.flag')
    const result = await client.evaluate('test.flag', false)

    expect(result).toBe(false)
    expect(notifications).toContain('test.flag')
    expect(mockFetch).toHaveBeenCalledTimes(2)
  })
})
