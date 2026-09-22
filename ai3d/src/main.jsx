/* eslint-disable react-refresh/only-export-components */
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.jsx'

function AuthRequired() {
  return (
    <main style={{ minHeight: '100vh', display: 'grid', placeItems: 'center', padding: 24, fontFamily: 'system-ui, sans-serif' }}>
      <section style={{ maxWidth: 520, textAlign: 'center' }}>
        <h1>请从 Sub2API 进入图生 3D</h1>
        <p>当前登录已失效或尚未建立单点登录会话，请返回控制台后重新打开此应用。</p>
      </section>
    </main>
  )
}

async function resolveIdentity() {
  try {
    const response = await fetch('/api/auth/me', { credentials: 'include', cache: 'no-store' })
    if (response.status === 401) return { namespace: 'anonymous', authenticated: false }
    // A non-authentication error (for example a 404/500 from a reverse
    // proxy) is not proof of a local session. Loading the app under a local
    // namespace here would expose stale browser data and make a managed
    // deployment appear usable while every API call is unauthorized.
    if (!response.ok) return { namespace: 'anonymous', authenticated: false }
    const payload = await response.json()
    return { namespace: String(payload.id || 'local'), authenticated: true }
  } catch {
    // The Docker image currently serves the UI through Vite's dev server even
    // though the container is a production deployment. Do not use
    // import.meta.env.DEV as an authentication signal: an API/proxy outage
    // must never make a managed browser load a cached local namespace.
    return { namespace: 'anonymous', authenticated: false }
  }
}

resolveIdentity().then(({ namespace, authenticated }) => {
  globalThis.__AI3D_USER_NAMESPACE__ = namespace
  createRoot(document.getElementById('root')).render(
    <StrictMode>
      {authenticated ? <App /> : <AuthRequired />}
    </StrictMode>,
  )
})
