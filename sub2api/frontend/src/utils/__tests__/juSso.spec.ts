import { afterEach, describe, expect, it, vi } from 'vitest'
import { openJuSso } from '../juSso'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function mockPopup() {
  const popup = {
    opener: window as Window | null,
    location: { replace: vi.fn() },
    closed: false,
    close: vi.fn(),
  }
  const open = vi.spyOn(window, 'open').mockReturnValue(popup as unknown as Window)
  return { popup, open }
}

const startUrl = '/api/v1/auth/integrations/ju/start?next=%2Fprojects'

describe('Yingce SSO handoff', () => {
  it('opens synchronously, detaches the opener, and performs a cookie-authenticated top-level navigation', async () => {
    const { popup, open } = mockPopup()
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    await openJuSso(startUrl)

    expect(open).toHaveBeenCalledWith('about:blank', '_blank')
    expect(popup.opener).toBeNull()
    expect(popup.location.replace).toHaveBeenCalledOnce()
    expect(popup.location.replace).toHaveBeenCalledWith(startUrl)
    expect(fetchMock).not.toHaveBeenCalled()
    expect(popup.close).not.toHaveBeenCalled()
  })

  it('does not issue a navigation when the popup is blocked', async () => {
    vi.spyOn(window, 'open').mockReturnValue(null)
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    await expect(openJuSso(startUrl)).rejects.toThrow('blocked')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('closes the empty window if navigation throws', async () => {
    const { popup } = mockPopup()
    popup.location.replace.mockImplementation(() => { throw new Error('navigation failed') })

    await expect(openJuSso(startUrl)).rejects.toThrow('navigation failed')
    expect(popup.close).toHaveBeenCalledOnce()
  })
})
