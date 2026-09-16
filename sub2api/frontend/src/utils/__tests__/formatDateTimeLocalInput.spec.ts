import { describe, expect, it } from 'vitest'

import { formatDateTimeLocalInput, getBrowserTimeZone, parseDateTimeLocalInput } from '../format'

describe('formatDateTimeLocalInput', () => {
  it('round-trips a minute-precision local timestamp', () => {
    const timestamp = Math.floor(new Date(2026, 7, 30, 7, 24).getTime() / 1000)
    expect(formatDateTimeLocalInput(timestamp)).toBe('2026-08-30T07:24')
    expect(parseDateTimeLocalInput('2026-08-30T07:24')).toBe(timestamp)
  })
})

describe('parseDateTimeLocalInput', () => {
  it('rejects timezone-bearing and malformed values', () => {
    expect(parseDateTimeLocalInput('2026-08-30T07:24:10+08:00')).toBeNull()
    expect(parseDateTimeLocalInput('2026-02-30T07:24')).toBeNull()
    expect(parseDateTimeLocalInput('2026-08-30T24:00')).toBeNull()
  })
})

describe('getBrowserTimeZone', () => {
  it('returns a timezone identifier or UTC fallback', () => {
    expect(getBrowserTimeZone()).toBeTruthy()
  })
})
