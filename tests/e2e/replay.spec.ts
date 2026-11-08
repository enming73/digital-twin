import { expect, test } from '@playwright/test'

test('injects and clears a simulated fault and inspects read-only replay', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'Start simulation' }).click()
  await page.getByRole('button', { name: 'Inject mounter fault' }).click()
  await expect(page.getByRole('button', { name: 'Clear mounter fault' })).toBeVisible()
  await expect(page.locator('.station-item').filter({ hasText: 'Pick & Place' })).toContainText('FAULT')
  await page.getByRole('button', { name: 'Clear mounter fault' }).click()
  await page.getByRole('button', { name: 'Open replay' }).click()
  await expect(page.getByText('REPLAY · READ ONLY')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Inject mounter fault' })).toBeDisabled()
  await expect(page.getByText('FAULT_INJECTED', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Return to live' }).click()
  await expect(page.getByRole('button', { name: 'Inject mounter fault' })).toBeEnabled()
})
