import { expect, test } from '@playwright/test'

test('dashboard locates a simulated board and shows its recorded route', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'Start simulation' }).click()
  await page.getByRole('button', { name: 'Pause simulation' }).click()
  await page.getByLabel('Board ID').fill('board-1')
  await page.getByRole('button', { name: 'Find board' }).click()
  await expect(page.getByRole('heading', { name: 'board-1', exact: true })).toBeVisible()
  await expect(page.getByText('BOARD_CREATED', { exact: true })).toBeVisible()
  await expect(page.locator('.board-result').getByText('work-order-1', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Reset simulation' }).click()
  await expect(page.getByRole('heading', { name: 'board-1', exact: true })).toHaveCount(0)
})
