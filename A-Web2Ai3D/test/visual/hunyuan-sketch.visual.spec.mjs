import { expect, test } from '@playwright/test'

const onePixelPng = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
  'base64',
)

test('草图提示模式先确认线稿再提交 3.0 Sketch 请求', async ({ page }) => {
  let requestBody = null

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
      body: JSON.stringify({ taskId: 'visual-sketch-task', status: 'queued' }),
    })
  })
  await page.route('**/api/3d/status/**', (route) => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ status: 'failed', error: 'test stop' }),
  }))

  await page.goto('/')
  await page.evaluate(() => localStorage.clear())
  await page.reload()

  await page.getByRole('button', { name: '混元 · 草图+提示' }).click()
  await expect(page.getByText('草图模式才能图+词一起发。')).toBeVisible()
  await expect(page.locator('#hunyuan-sketch-prompt')).toBeVisible()
  await expect(page.locator('input[type=file]').first()).toHaveAttribute('accept', 'image/*')

  await page.locator('#hunyuan-sketch-prompt').fill('一个带细胞核的上皮细胞')
  await page.locator('input[type=file]').first().setInputFiles({
    name: 'sketch.png',
    mimeType: 'image/png',
    buffer: onePixelPng,
  })
  await expect(page.getByRole('dialog', { name: '确认草图' })).toBeVisible()
  expect(requestBody).toBeNull()

  await page.getByRole('button', { name: '确认并生成' }).click()
  await expect.poll(() => requestBody).toMatchObject({
    provider: 'hunyuan',
    model: '3.0',
    generateType: 'Sketch',
    prompt: '一个带细胞核的上皮细胞',
  })
})
