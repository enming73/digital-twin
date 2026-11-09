import { describe, expect, it } from 'vitest'
import { isAllowedOrigin } from '../../src/twin/proxyOrigin'

describe('development WebSocket origin policy', () => {
  it('accepts only explicitly supported local browser origins', () => {
    expect(isAllowedOrigin('http://127.0.0.1:4173')).toBe(true)
    expect(isAllowedOrigin('http://localhost:5173')).toBe(true)
    expect(isAllowedOrigin('https://evil.example')).toBe(false)
    expect(isAllowedOrigin('http://localhost:4173.evil.example')).toBe(false)
    expect(isAllowedOrigin(undefined)).toBe(false)
  })
})
