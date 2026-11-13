import { expect, test } from '@playwright/test'
import { artifactNames } from '../../src/data/anchors'

test('loads every station model and retains machine selection', async ({ page }) => {
  const loads: string[] = []
  page.on('response', response => { if (response.url().includes('/models/') && response.status() === 200) loads.push(response.url()) })
  await page.goto('/')
  await expect.poll(() => artifactNames.every(name => loads.some(url => url.endsWith(`/${name}.glb`)))).toBe(true)
  await page.getByRole('button', { name: /Solder Paste Printer/ }).click()
  await expect(page.getByRole('heading', { name: 'Solder Paste Printer' })).toBeVisible()
  await page.getByRole('button', { name: /Pick & Place/ }).click()
  await expect(page.getByRole('heading', { name: 'Pick & Place' })).toBeVisible()
  await page.getByRole('button', { name: /Solder Paste Inspection/ }).click()
  await expect(page.getByRole('heading', { name: 'Solder Paste Inspection' })).toBeVisible()
  await page.getByRole('button', { name: /Automated Optical Inspection/ }).click()
  await expect(page.getByRole('heading', { name: 'Automated Optical Inspection' })).toBeVisible()
})

for (const [kind, label] of [
  ['printer', 'Solder Paste Printer'],
  ['mounter', 'Pick & Place'],
  ['spi', 'Solder Paste Inspection'],
  ['aoi', 'Automated Optical Inspection'],
]) {
  test(`keeps the factory interactive if ${kind} fails to load`, async ({ page }) => {
    let intercepted = false
    await page.route(`**/models/${kind}.glb`, route => { intercepted = true; return route.fulfill({ status: 200, contentType: 'model/gltf-binary', body: 'invalid model' }) })
    await page.goto('/')
    await expect.poll(() => intercepted).toBe(true)
    await expect(page.locator('canvas')).toBeVisible()
    await page.getByRole('button', { name: new RegExp(label) }).click()
    await expect(page.getByRole('heading', { name: label })).toBeVisible()
    await page.getByRole('button', { name: 'Start simulation' }).click()
    await expect(page.getByTestId('created-count')).not.toHaveText('0')
  })
}

test('falls back to the procedural shell for a newly modelled station', async ({ page }) => {
  let intercepted = false
  await page.route('**/models/loader.glb', route => { intercepted = true; return route.fulfill({ status: 200, contentType: 'model/gltf-binary', body: 'invalid model' }) })
  await page.goto('/')
  await expect.poll(() => intercepted).toBe(true)
  await expect(page.locator('canvas')).toBeVisible()
  await page.getByRole('button', { name: /PCB Loader/ }).click()
  await expect(page.getByRole('heading', { name: 'PCB Loader' })).toBeVisible()
})
