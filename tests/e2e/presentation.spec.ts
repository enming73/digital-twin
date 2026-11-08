import { expect, test } from '@playwright/test'

test('guided presentation starts, pauses, restarts and returns to manual mode', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'Start guided presentation' }).click()
  await expect(page.getByRole('heading', { name: 'A line in context' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Inject mounter fault' })).toBeDisabled()
  await page.getByRole('button', { name: 'Pause presentation' }).click()
  const time = await page.getByTestId('simulation-time').textContent()
  await page.waitForTimeout(400)
  await expect(page.getByTestId('simulation-time')).toHaveText(time!)
  await page.getByRole('button', { name: 'Restart presentation' }).click()
  await expect(page.getByRole('heading', { name: 'A line in context' })).toBeVisible()
  await page.getByRole('button', { name: 'Exit presentation' }).click()
  await expect(page.getByRole('button', { name: 'Inject mounter fault' })).toBeEnabled()
})

test('cold start and presentation do not require external services', async ({ page }) => {
  const external: string[] = []
  await page.route('**/*', route => {
    const url = new URL(route.request().url())
    if (url.hostname !== '127.0.0.1' && url.hostname !== 'localhost') { external.push(url.href); return route.abort() }
    return route.continue()
  })
  await page.goto('/')
  await expect(page.locator('canvas')).toBeVisible()
  await page.getByRole('button', { name: 'Start guided presentation' }).click()
  await expect(page.getByRole('heading', { name: 'A line in context' })).toBeVisible()
  await page.waitForLoadState('networkidle')
  expect(external).toEqual([])
  await page.screenshot({ path: 'test-results/presentation.png', animations: 'disabled' })
})
