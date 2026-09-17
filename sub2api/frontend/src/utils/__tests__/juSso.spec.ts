import { afterEach, describe, expect, it, vi } from 'vitest'
import { openJuSso } from '../juSso'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function mockPopup() {
  const popup = {
    opener: window as Window | null,
    document: document.implementation.createHTMLDocument(''),
    location: { replace: vi.fn() },
    closed: false,
    close: vi.fn(),
  }
  const open = vi.spyOn(window, 'open').mockReturnValue(popup as unknown as Window)
  return { popup, open }
}

const startUrl = '/api/v1/auth/integrations/ju/start?next=%2Fprojects'
const redirectUrl = 'https://ju.example/api/auth/sso/callback?ticket=one-time-ticket'

describe('Yingce SSO handoff', () => {
  it('opens synchronously, detaches the opener, and sends only the JWT to the ticket endpoint', async () => {
    const { popup, open } = mockPopup()
    const fetchMock = vi.fn(async () => {
      expect(open).toHaveBeenCalledWith('about:blank', '_blank')
      expect(popup.opener).toBeNull()
      expect(popup.document.querySelector('meta[name="referrer"]')?.getAttribute('content')).toBe('no-referrer')
      return new Response(JSON.stringify({ code: 0, data: { redirect_url: redirectUrl } }))
    })
    vi.stubGlobal('fetch', fetchMock)

    await openJuSso(startUrl, 'session-jwt')

    expect(fetchMock).toHaveBeenCalledOnce()
    expect(fetchMock).toHaveBeenCalledWith(startUrl, {
      headers: { Authorization: 'Bearer session-jwt' },
      credentials: 'include',
      cache: 'no-store',
      redirect: 'error',
    })
    expect(popup.location.replace).toHaveBeenCalledOnce()
    expect(popup.location.replace).toHaveBeenCalledWith(redirectUrl)
    expect(popup.close).not.toHaveBeenCalled()
  })

  it('does not issue a ticket when the popup is blocked', async () => {
    vi.spyOn(window, 'open').mockReturnValue(null)
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    await expect(openJuSso(startUrl, 'session-jwt')).rejects.toThrow('blocked')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('does not open an unauthenticated handoff', async () => {
    const { open } = mockPopup()
    await expect(openJuSso(startUrl, null)).rejects.toThrow('authenticated')
    expect(open).not.toHaveBeenCalled()
  })

  it.each([
    new Response('{}', { status: 401 }),
    new Response(JSON.stringify({ code: 1, data: { redirect_url: redirectUrl } })),
    new Response(JSON.stringify({ code: 0, data: {} })),
    new Response(JSON.stringify({ code: 0, data: { redirect_url: 'javascript:alert(1)' } })),
    new Response(JSON.stringify({ code: 0, data: { redirect_url: 'https://user:password@ju.example/' } })),
  ])('closes the empty window on a failed or unsafe response', async (response) => {
    const { popup } = mockPopup()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response))
    await expect(openJuSso(startUrl, 'session-jwt')).rejects.toThrow()
    expect(popup.close).toHaveBeenCalledOnce()
    expect(popup.location.replace).not.toHaveBeenCalled()
  })

  it('does not navigate a window the user closed while the ticket was loading', async () => {
    const { popup } = mockPopup()
    vi.stubGlobal('fetch', vi.fn(async () => {
      popup.closed = true
      return new Response(JSON.stringify({ code: 0, data: { redirect_url: redirectUrl } }))
    }))
    await openJuSso(startUrl, 'session-jwt')
    expect(popup.location.replace).not.toHaveBeenCalled()
  })
})
