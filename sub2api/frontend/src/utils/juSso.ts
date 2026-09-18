export async function openJuSso(startUrl: string, token: string | null): Promise<void> {
  if (!token) throw new Error('Yingce SSO requires an authenticated session')

  // noopener/noreferrer window features return null even when the tab opens.
  // Keep the blank tab handle, then detach its opener before any async work.
  const popup = window.open('about:blank', '_blank')
  if (!popup) throw new Error('Yingce SSO window was blocked')

  try {
    popup.opener = null
    const referrer = popup.document.createElement('meta')
    referrer.name = 'referrer'
    referrer.content = 'no-referrer'
    popup.document.head.appendChild(referrer)

    const response = await fetch(startUrl, {
      headers: { Authorization: `Bearer ${token}` },
      credentials: 'include',
      cache: 'no-store',
      redirect: 'error',
    })
    const body = await response.json().catch(() => null) as {
      code?: number
      message?: string
      data?: { redirect_url?: string }
    } | null
    if (!response.ok) {
      const detail = body?.message?.trim()
      if (response.status === 401) throw new Error('Yingce SSO requires an authenticated session')
      if (response.status === 503) throw new Error(detail || 'Yingce SSO is not configured')
      throw new Error(detail || 'Yingce SSO request failed')
    }
    if (!body || body.code !== 0 || !body.data?.redirect_url) throw new Error('Invalid Yingce SSO response')
    const target = new URL(body.data.redirect_url)
    if (!['https:', 'http:'].includes(target.protocol) || target.username || target.password) {
      throw new Error('Invalid Yingce SSO destination')
    }
    if (popup.closed) return
    popup.location.replace(target.toString())
  } catch (error) {
    popup.close()
    throw error
  }
}
