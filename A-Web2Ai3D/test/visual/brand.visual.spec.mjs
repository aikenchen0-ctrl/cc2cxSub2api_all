import { expect, test } from '@playwright/test'

test('页面使用 cc2cx 品牌和 AI3D生成标题', async ({ page }) => {
  await page.goto('/')

  await expect(page).toHaveTitle('AI3D生成 | cc2cx')
  await expect(page.locator('.studio-brand strong')).toHaveText('cc2cx')
  await expect(page.locator('.studio-brand span')).toHaveText('AI3D生成')
})

test('手机端也显示完整品牌副标题', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/')

  await expect(page.locator('.studio-brand strong')).toHaveText('cc2cx')
  await expect(page.locator('.studio-brand span')).toBeVisible()
  await expect(page.locator('.studio-brand span')).toHaveText('AI3D生成')
})
