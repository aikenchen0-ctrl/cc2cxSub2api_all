export async function openJuSso(startUrl: string): Promise<void> {
  // Open synchronously in the click handler, then navigate the top-level
  // window. The Sub2API start endpoint authenticates this navigation with its
  // HttpOnly browser session cookie and returns a one-time ticket redirect;
  // no browser-held JWT or API credential is ever read or forwarded here.
  const popup = window.open('about:blank', '_blank')
  if (!popup) throw new Error('Yingce SSO window was blocked')

  try {
    popup.opener = null
    popup.location.replace(startUrl)
  } catch (error) {
    popup.close()
    throw error
  }
}
