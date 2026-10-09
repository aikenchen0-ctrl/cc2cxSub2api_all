import { expect, test } from '@playwright/test'
import { BoxGeometry } from 'three'

test.use({ launchOptions: { args: ['--use-angle=swiftshader', '--enable-unsafe-swiftshader'] } })

// Small, deterministic generated-model fixture: no cloud calls or large assets.
function createBoxGltf() {
  const geometry = new BoxGeometry()
  const arrays = [geometry.attributes.position.array, geometry.attributes.normal.array, geometry.index.array]
  let byteOffset = 0
  const bufferViews = arrays.map((array) => {
    const view = { buffer: 0, byteOffset, byteLength: array.byteLength }
    byteOffset += array.byteLength
    return view
  })
  return {
    asset: { version: '2.0' },
    scene: 0,
    scenes: [{ nodes: [0] }],
    nodes: [{ mesh: 0 }],
    meshes: [{ primitives: [{ attributes: { POSITION: 0, NORMAL: 1 }, indices: 2 }] }],
    accessors: [
      { bufferView: 0, componentType: 5126, count: 24, type: 'VEC3', min: [-0.5, -0.5, -0.5], max: [0.5, 0.5, 0.5] },
      { bufferView: 1, componentType: 5126, count: 24, type: 'VEC3' },
      { bufferView: 2, componentType: 5123, count: 36, type: 'SCALAR' },
    ],
    bufferViews,
    buffers: [{ byteLength: byteOffset, uri: `data:application/octet-stream;base64,${Buffer.concat(arrays.map((array) => Buffer.from(array.buffer))).toString('base64')}` }],
  }
}

test('点击及拖动生成模型不弹出信息框，详情由信息按钮主动打开', async ({ page }) => {
  await page.route('**/api/**', (route) => route.fulfill({ json: { providers: {} } }))
  await page.route('**/generated-models/interaction.gltf', (route) => route.fulfill({ json: createBoxGltf() }))
  await page.addInitScript(() => {
    globalThis.localStorage.setItem('bio-demo-custom-cells', JSON.stringify([{
      id: 'custom-interaction', name: '交互测试模型', custom: true, template: 'animal',
      generation: { provider: 'hunyuan', status: 'success', modelUrl: '/generated-models/interaction.gltf' },
    }]))
    globalThis.localStorage.setItem('bio-demo-ui-state', JSON.stringify({ selectedCell: 'custom-interaction', uiStateVersion: 1 }))
  })
  await page.goto('/')
  const canvas = page.locator('.cell-viewer canvas')
  await expect(canvas).toBeVisible()
  await expect(page.getByRole('button', { name: '导出 3D', exact: true })).toBeEnabled()
  await expect(page.locator('.model-quality-card')).toContainText('12')
  // Selection proves that the click hit the loaded mesh, not an empty canvas.
  await expect(async () => {
    await canvas.click()
    const selectedPart = await page.evaluate(() => JSON.parse(globalThis.localStorage.getItem('cc2cx-ai3d:local:bio-demo-ui-state')).selectedOrganelle)
    expect(selectedPart).toBe('membrane')
  }).toPass({ timeout: 20_000 })
  await expect(page.locator('.inspector-zone')).toHaveCount(0)
  await expect(page.locator('.inspector-scrim')).toHaveCount(0)
  const box = await canvas.boundingBox()
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
  await page.mouse.down()
  await page.mouse.move(box.x + box.width / 2 + 60, box.y + box.height / 2 + 30, { steps: 8 })
  await page.mouse.up()
  await expect(page.locator('.inspector-zone')).toHaveCount(0)
  // Returning to the starting point can produce a click despite a long drag.
  // Repeat releases both on the mesh and outside the viewer.
  for (const returnToStart of [true, false]) {
    const x = box.x + box.width / 2
    const y = box.y + box.height / 2
    await page.mouse.move(x, y)
    await page.mouse.down()
    await page.mouse.move(x + 100, y + 50, { steps: 10 })
    await page.mouse.move(returnToStart ? x : box.x - 12, returnToStart ? y : box.y + box.height - 10, { steps: 10 })
    await page.mouse.up()
    await expect(page.locator('.inspector-zone')).toHaveCount(0)
    await expect(page.locator('.inspector-trigger')).toHaveAttribute('aria-expanded', 'false')
  }
  await page.screenshot({ path: 'test-results/model-interaction.png' })
  await page.getByRole('button', { name: '信息', exact: true }).click()
  await expect(page.locator('.inspector-zone')).toBeVisible()
  await expect(page.locator('.inspector-zone')).toContainText('交互测试模型')
  await page.locator('.inspector-close').click()
  await expect(page.locator('.inspector-zone')).toHaveCount(0)
  await page.getByRole('button', { name: '检查', exact: true }).click()
  await expect(page.locator('.inspector-zone')).toBeVisible()
})
