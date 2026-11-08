import { test, expect } from '@playwright/test'

test('factory scene supports station selection and reset without browser errors', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
  await page.goto('/')
  await expect(page.getByText('Concept demo · Synthetic data · Not affiliated with ASMPT')).toBeVisible()
  await expect(page.locator('canvas')).toBeVisible()
  const buttons = page.locator('.station-item')
  await expect(buttons).toHaveCount(7)
  for (let i = 0; i < 7; i++) {
    await buttons.nth(i).click()
    await expect(buttons.nth(i)).toHaveAttribute('aria-pressed', 'true')
  }
  await page.getByRole('button', { name: 'Reset camera to factory overview' }).click()
  await expect(page.getByRole('heading', { name: 'Select a station' })).toBeVisible()
  await expect(page.locator('.station-item[aria-pressed="true"]')).toHaveCount(0)
  expect(errors).toEqual([])
  await page.screenshot({ path: 'test-results/factory-overview.png', animations: 'disabled' })
})
