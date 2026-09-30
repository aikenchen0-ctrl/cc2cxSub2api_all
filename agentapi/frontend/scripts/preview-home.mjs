// Isolated visual-review fixture. Run from frontend: node scripts/preview-home.mjs
// No upstream, credentials, database, or production configuration is loaded.
import { createServer } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('../', import.meta.url))
const branding = {
  name: '视觉验收站', site_name: 'AgentAPI 演示站', site_logo: '/logo.svg',
  doc_url: '', contact_info: 'demo@example.test',
  site_subtitle: '统一接入 AI 模型\n一个 API，连接你的创作与开发',
  compact_home_enabled: true, home_content: '',
}
const user = { id: 'visual-owner', username: '演示站长', role: 'user', agent_admin: true }
const server = await createServer({
  configFile: false, root, mode: 'test', envPrefix: 'AGENTAPI_VISUAL_QA_',
  resolve: { alias: { '@': fileURLToPath(new URL('../src', import.meta.url)) } },
  server: { host: '127.0.0.1', port: 18419, strictPort: true },
  plugins: [vue(), {
    name: 'agent-home-visual-fixtures',
    configureServer(vite) {
      vite.middlewares.use(async (req, res, next) => {
        const path = new URL(req.url, 'http://127.0.0.1').pathname
        if (!path.startsWith('/api/') && !path.startsWith('/v1/')) return next()
        res.setHeader('Content-Type', 'application/json; charset=utf-8')
        res.setHeader('Cache-Control', 'no-store')
        const reply = (data) => res.end(JSON.stringify({ code: 0, data }))
        if (path === '/api/v1/settings/public') return reply({ ...branding, registration_enabled: false })
        if (path === '/api/v1/auth/me') return reply(user)
        if (path === '/api/v1/agent/context') return reply({
          authenticated: true, is_agent_admin: true,
          agent: { ...branding, agent_id: 'visual-only', domain: '127.0.0.1:18419', status: 'active', billing_mode: 'user_upstream' },
          user: { main_user_id: user.id, balance_cents: 0 },
        })
        if (path === '/api/v1/agent/admin/branding') {
          if (req.method === 'PUT') {
            let body = ''
            for await (const chunk of req) {
              body += chunk
              if (body.length > 500000) { res.statusCode = 413; return res.end('{}') }
            }
            try {
              const input = JSON.parse(body)
              for (const key of Object.keys(branding)) {
                if (typeof input[key] === typeof branding[key]) branding[key] = input[key]
              }
            } catch { res.statusCode = 400; return res.end('{}') }
          }
          return reply(branding)
        }
        if (path === '/api/v1/agent/announcements' || path === '/api/v1/agent/content-pages') return reply({ items: [], total: 0, unread: 0 })
        res.statusCode = 404
        res.end(JSON.stringify({ code: 404, message: 'No fixture for this endpoint' }))
      })
    },
  }],
})
await server.listen()
server.printUrls()
