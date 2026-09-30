import { describe, expect, it } from 'vitest'
import { ccswitchVersion, detectPlatform, downloadOptions, recommendDownload } from './ccswitch-downloads'

describe('CCSwitch downloads', () => {
  it('copies every main-site desktop package with the pinned release version', () => {
    expect(downloadOptions).toHaveLength(4)
    expect(downloadOptions.map((item) => item.key)).toEqual(['windows-exe', 'windows-msi', 'mac-arm', 'mac-intel'])
    expect(downloadOptions.every((item) => item.url.startsWith('https://github.com/aikenchen0-ctrl/cc2cx/releases/'))).toBe(true)
    expect(downloadOptions.every((item) => item.url.includes(ccswitchVersion))).toBe(true)
  })

  it('recommends the matching package and uses the Windows installer for unknown platforms', () => {
    expect(detectPlatform('Mozilla/5.0 (Windows NT 10.0; Win64; x64)')).toBe('windows')
    expect(recommendDownload('Mozilla/5.0 (Macintosh; ARM Mac OS X)').key).toBe('mac-arm')
    expect(recommendDownload('Mozilla/5.0 (Macintosh; Intel Mac OS X)').key).toBe('mac-intel')
    expect(recommendDownload('Mozilla/5.0 (X11; Linux x86_64)').key).toBe('windows-exe')
  })
})
