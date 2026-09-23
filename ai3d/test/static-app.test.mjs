import assert from 'node:assert/strict'
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { after, before, describe, it } from 'node:test'
import { serveStaticApp } from '../server/static-app.mjs'

describe('production static app server', () => {
  let root

  before(async () => {
    root = await mkdtemp(path.join(os.tmpdir(), 'ai3d-static-'))
    await mkdir(path.join(root, 'assets'))
    await writeFile(path.join(root, 'index.html'), '<div id="root">AI3D</div>')
    await writeFile(path.join(root, 'assets', 'app.js'), 'globalThis.ai3d = true')
  })

  after(async () => {
    await rm(root, { recursive: true, force: true })
  })

  it('serves the built index and immutable assets', async () => {
    const index = createResponse()
    assert.equal(await serveStaticApp(createRequest(), index, new URL('http://ai3d.local/'), root), true)
    assert.equal(index.statusCode, 200)
    assert.equal(index.headers['cache-control'], 'no-cache')
    assert.match(index.body.toString(), /AI3D/)

    const asset = createResponse()
    assert.equal(await serveStaticApp(createRequest('*/*'), asset, new URL('http://ai3d.local/assets/app.js'), root), true)
    assert.equal(asset.statusCode, 200)
    assert.equal(asset.headers['content-type'], 'text/javascript; charset=utf-8')
    assert.match(asset.headers['cache-control'], /immutable/)
  })

  it('uses index.html for extensionless browser routes', async () => {
    const response = createResponse()
    await serveStaticApp(createRequest(), response, new URL('http://ai3d.local/projects/demo'), root)
    assert.equal(response.statusCode, 200)
    assert.match(response.body.toString(), /AI3D/)
  })

  it('leaves API routes to the API handler', async () => {
    const response = createResponse()
    assert.equal(await serveStaticApp(createRequest(), response, new URL('http://ai3d.local/api/auth/me'), root), false)
    assert.equal(response.body, undefined)
  })

  it('rejects encoded path traversal and missing assets', async () => {
    const traversal = createResponse()
    await serveStaticApp(createRequest(), traversal, new URL('http://ai3d.local/%2e%2e%2fsecrets'), root)
    assert.equal(traversal.statusCode, 400)

    const missing = createResponse()
    await serveStaticApp(createRequest('*/*'), missing, new URL('http://ai3d.local/assets/missing.js'), root)
    assert.equal(missing.statusCode, 404)
  })
})

function createRequest(accept = 'text/html') {
  return { method: 'GET', headers: { accept } }
}

function createResponse() {
  return {
    statusCode: 0,
    headers: {},
    setHeader(name, value) {
      this.headers[String(name).toLowerCase()] = value
    },
    end(body) {
      this.body = body
    },
  }
}
