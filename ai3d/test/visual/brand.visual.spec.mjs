import { expect, test } from '@playwright/test'

test('页面使用图生3D品牌标题', async ({ page }) => {
  await page.goto('/')

  await expect(page).toHaveTitle('图生3D')
  await expect(page.locator('.studio-brand strong')).toHaveText('图生3D')
  await expect(page.locator('.studio-brand span')).toHaveText('AI 3D 工作台')
})

test('手机端也显示完整品牌副标题', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/')

  await expect(page.locator('.studio-brand strong')).toHaveText('图生3D')
  await expect(page.locator('.studio-brand span')).toBeVisible()
  await expect(page.locator('.studio-brand span')).toHaveText('AI 3D 工作台')
})
