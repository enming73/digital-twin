import { expect, test } from '@playwright/test'

test('machine close-up identifies process motion as illustrative', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByText('Illustrative animation · Not measured machine coordinates')).toBeVisible()
  await page.getByRole('button', { name: /Solder Paste Printer/ }).click()
  await expect(page.getByRole('button', { name: 'Refocus selected machine' })).toBeVisible()
  await page.getByRole('button', { name: 'Refocus selected machine' }).click()
  await expect(page.getByRole('heading', { name: 'Solder Paste Printer' })).toBeVisible()
})
