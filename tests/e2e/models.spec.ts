import { expect, test } from '@playwright/test'

test('loads the two local machine assets and retains machine selection', async ({ page }) => {
  const loads: string[] = []
  page.on('response', response => { if (response.url().includes('/models/') && response.status() === 200) loads.push(response.url()) })
  await page.goto('/')
  await expect.poll(() => loads.some(url => url.endsWith('/printer.glb')) && loads.some(url => url.endsWith('/mounter.glb'))).toBe(true)
  await page.getByRole('button', { name: /Solder Paste Printer/ }).click()
  await expect(page.getByRole('heading', { name: 'Solder Paste Printer' })).toBeVisible()
})

test('keeps the factory interactive if a model fails to load', async ({ page }) => {
  let intercepted = false
  await page.route('**/models/printer.glb', route => { intercepted = true; return route.fulfill({ status: 200, contentType: 'model/gltf-binary', body: 'invalid model' }) })
  await page.goto('/')
  await expect.poll(() => intercepted).toBe(true)
  await expect(page.locator('canvas')).toBeVisible()
  await page.getByRole('button', { name: /Solder Paste Printer/ }).click()
  await expect(page.getByRole('heading', { name: 'Solder Paste Printer' })).toBeVisible()
  await page.getByRole('button', { name: 'Start simulation' }).click()
  await expect(page.getByTestId('created-count')).not.toHaveText('0')
})
