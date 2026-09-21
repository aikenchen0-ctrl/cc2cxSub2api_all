import { expect, test } from '@playwright/test'

const onePixelPng = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
  'base64',
)

async function stubHealth(page) {
  await page.route('**/api/3d/health', (route) => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({
      providers: {
        hunyuan: { configured: true },
        tripo: { configured: false },
        fal: { configured: false },
        rodin: { configured: false },
      },
    }),
  }))
}

test('首屏把图片生成和 GLB 导入拆成两个明确入口', async ({ page }) => {
  await stubHealth(page)
  await page.goto('/')

  await expect(page.getByRole('button', { name: '上传图片并生成 3D' })).toBeVisible()
  await expect(page.getByRole('button', { name: '导入 GLB' })).toBeVisible()
  await expect(page.locator('input[type=file]')).toHaveCount(2)
  await expect(page.locator('input[type=file]').nth(0)).toHaveAttribute('accept', 'image/*')
  await expect(page.locator('input[type=file]').nth(1)).toHaveAttribute('accept', '.glb,.gltf,model/gltf-binary,model/gltf+json')
})

test('草图模式把主入口改为添加草图并继续隐藏照片入口', async ({ page }) => {
  await stubHealth(page)
  await page.goto('/')

  await page.getByRole('button', { name: '混元 · 草图+提示' }).click()
  await expect(page.getByRole('button', { name: '添加草图' })).toBeVisible()
  await expect(page.getByRole('button', { name: '上传图片并生成 3D' })).toHaveCount(0)
  await expect(page.locator('input[type=file]').nth(0)).toHaveAttribute('accept', 'image/*')
})

test('手机首屏始终显示图片生成主按钮', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await stubHealth(page)
  await page.goto('/')

  const action = page.getByRole('button', { name: '上传图片并生成 3D' })
  await expect(action).toBeVisible()
  const box = await action.boundingBox()

  expect(box).toBeTruthy()
  expect(box.y).toBeGreaterThanOrEqual(0)
  expect(box.y + box.height).toBeLessThanOrEqual(844)
})

test('普通图片选择后先确认再提交生成任务', async ({ page }) => {
  let requestBody = null
  await stubHealth(page)
  await page.route('**/api/3d/analyze', (route) => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ configured: false }),
  }))
  await page.route('**/api/3d/generate', async (route) => {
    requestBody = JSON.parse(route.request().postData() || '{}')
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ taskId: 'ordinary-image-task', status: 'queued' }),
    })
  })

  await page.goto('/')
  await page.locator('input[type=file]').first().setInputFiles({
    name: 'cell.png',
    mimeType: 'image/png',
    buffer: onePixelPng,
  })

  await expect(page.getByRole('dialog', { name: '确认生成 3D' })).toBeVisible()
  await expect(page.getByText('预计 2–8 分钟')).toBeVisible()
  await expect(page.getByText('会消耗积分')).toBeVisible()
  await expect(page.getByText('输出：GLB')).toBeVisible()
  expect(requestBody).toBeNull()

  await page.getByRole('button', { name: '确认并生成' }).click()
  await expect.poll(() => requestBody).toMatchObject({
    provider: 'hunyuan',
    model: '3.1',
    generateType: 'Normal',
    prompt: '',
  })
})
