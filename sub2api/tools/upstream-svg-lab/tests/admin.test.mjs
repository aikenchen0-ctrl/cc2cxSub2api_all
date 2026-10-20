import test from 'node:test'
import assert from 'node:assert/strict'

import { AdminClient, accountSource } from '../lib/admin.mjs'

const ADMIN_KEY = 'admin-test-key'
const UPSTREAM_KEY = 'sk-live-1234567890'

function jsonResponse(value, { status = 200 } = {}) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { 'content-type': 'application/json' },
  })
}

function account(overrides = {}) {
  return {
    id: 42,
    name: 'primary-upstream',
    platform: 'openai',
    type: 'apikey',
    status: 'active',
    proxy_id: null,
    parent_account_id: null,
    credentials: { base_url: 'https://relay.example.test/v1' },
    credentials_status: { has_api_key: true },
    ...overrides,
  }
}

function adminClient(fetchImpl) {
  return new AdminClient({
    baseURL: 'https://sub2api.example.test',
    key: ADMIN_KEY,
    fetchImpl,
  })
}

test('accountSource accepts a redacted active API Key account', () => {
  const source = accountSource(account())

  assert.equal(source.eligible, true)
  assert.equal(source.source_url, 'https://relay.example.test/v1')
  assert.equal(Object.hasOwn(source, 'key'), false)
})

test('accountSource rejects proxy, shadow, and non-active accounts', () => {
  for (const [name, overrides, message] of [
    ['proxy', { proxy_id: 9 }, /依赖代理/],
    ['shadow', { parent_account_id: 7 }, /影子/],
    ['inactive', { status: 'error' }, /启用状态/],
  ]) {
    const result = accountSource(account(overrides))
    assert.equal(result.eligible, false, name)
    assert.match(result.reason, message, name)
  }
})

test('source reads only the selected account and returns a minimal in-memory source', async () => {
  const calls = []
  const fetchImpl = async (url, init) => {
    calls.push({ url, init })
    if (url.endsWith('/accounts/42')) {
      return jsonResponse({ code: 0, data: account() })
    }
    if (url.includes('/accounts/data?ids=42&include_proxies=false')) {
      return jsonResponse({
        code: 0,
        data: {
          accounts: [{
            name: 'primary-upstream',
            platform: 'openai',
            type: 'apikey',
            credentials: {
              base_url: 'https://relay.example.test/v1',
              api_key: UPSTREAM_KEY,
              refresh_token: 'must-not-escape',
            },
            extra: { internal: 'must-not-escape' },
            proxy_key: null,
          }],
        },
      })
    }
    throw new Error(`unexpected request: ${url}`)
  }

  const source = await adminClient(fetchImpl).source(42)

  assert.deepEqual(Object.keys(source).sort(), ['key', 'name', 'source_url', 'url'])
  assert.equal(source.name, 'primary-upstream')
  assert.equal(source.url, 'https://relay.example.test/v1')
  assert.equal(source.source_url, source.url)
  assert.equal(source.key, UPSTREAM_KEY)
  assert.equal(calls.length, 2)
  assert.deepEqual(calls.map(call => new URL(call.url).pathname), [
    '/api/v1/admin/accounts/42',
    '/api/v1/admin/accounts/data',
  ])
  assert.equal(new URL(calls[1].url).search, '?ids=42&include_proxies=false')
  assert.ok(calls.every(call => call.init.method === undefined))
  assert.ok(calls.every(call => call.init.headers['x-api-key'] === ADMIN_KEY))
  assert.equal(calls.some(call => /\/accounts\/data(?!\?ids=42&include_proxies=false)/.test(call.url)), false)
})

test('source does not fall back after a protected export returns 403', async () => {
  const calls = []
  const fetchImpl = async (url, init) => {
    calls.push({ url, init })
    if (url.endsWith('/accounts/42')) return jsonResponse({ code: 0, data: account() })
    return jsonResponse({ code: 'STEP_UP_ADMIN_API_KEY_FORBIDDEN', message: 'forbidden' }, { status: 403 })
  }

  await assert.rejects(
    () => adminClient(fetchImpl).source(42),
    error => {
      assert.match(error.message, /HTTP 403/)
      assert.doesNotMatch(error.message, new RegExp(UPSTREAM_KEY))
      return true
    },
  )
  assert.equal(calls.length, 2)
  assert.equal(calls.some(call => call.init.method && call.init.method !== 'GET'), false)
})

test('source rejects export metadata mismatch without exposing credentials', async () => {
  const fetchImpl = async url => {
    if (url.endsWith('/accounts/42')) return jsonResponse({ code: 0, data: account() })
    return jsonResponse({
      code: 0,
      data: {
        accounts: [{
          name: 'other-account',
          platform: 'openai',
          type: 'apikey',
          credentials: { base_url: 'https://relay.example.test/v1', api_key: UPSTREAM_KEY },
          proxy_key: null,
        }],
      },
    })
  }

  await assert.rejects(
    () => adminClient(fetchImpl).source(42),
    error => {
      assert.match(error.message, /不一致/)
      assert.doesNotMatch(error.message, new RegExp(UPSTREAM_KEY))
      return true
    },
  )
})
