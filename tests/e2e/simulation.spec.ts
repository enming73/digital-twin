import { expect, test } from '@playwright/test'

test('simulation controls start, pause and reset the simulated line', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByText('SIMULATED', { exact: true }).first()).toBeVisible()
  await page.getByRole('button', { name: 'Start simulation' }).click()
  await expect(page.getByTestId('created-count')).not.toHaveText('0')
  await page.getByRole('button', { name: 'Pause simulation' }).click()
  const time = await page.getByTestId('simulation-time').textContent()
  await page.waitForTimeout(400)
  await expect(page.getByTestId('simulation-time')).toHaveText(time!)
  await page.getByRole('button', { name: 'Reset simulation' }).click()
  await expect(page.getByTestId('created-count')).toHaveText('0')
  await expect(page.getByTestId('simulation-time')).toHaveText('00:00.0')
})
