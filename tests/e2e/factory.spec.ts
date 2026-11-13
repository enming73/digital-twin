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

test('a drag during the push-in cancels the ease without breaking the scene', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
  await page.goto('/')
  await expect(page.locator('canvas')).toBeVisible()
  await page.getByRole('button', { name: /Pick & Place/ }).click()
  const box = await page.locator('canvas').boundingBox()
  if (!box) throw new Error('canvas has no box')
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
  await page.mouse.down()
  await page.mouse.move(box.x + box.width / 2 + 160, box.y + box.height / 2 + 40, { steps: 8 })
  await page.mouse.up()
  await page.waitForTimeout(1500)
  await expect(page.getByRole('heading', { name: 'Pick & Place' })).toBeVisible()
  await page.getByRole('button', { name: 'Reset camera to factory overview' }).click()
  await expect(page.getByRole('heading', { name: 'Select a station' })).toBeVisible()
  expect(errors).toEqual([])
})

test('renders the scene without console errors or cross-origin requests', async ({ page }) => {
  const errors: string[] = []
  const external: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
  page.on('request', request => {
    const url = new URL(request.url())
    if (!url.protocol.startsWith('http')) return
    if (url.hostname !== '127.0.0.1' && url.hostname !== 'localhost') external.push(request.url())
  })
  await page.goto('/')
  await expect(page.locator('canvas')).toBeVisible()
  await page.waitForTimeout(1500)
  await page.screenshot({ path: 'test-results/v12-overview.png', animations: 'disabled' })
  await page.getByRole('button', { name: /Pick & Place/ }).click()
  await page.waitForTimeout(1200)
  await page.screenshot({ path: 'test-results/v12-closeup.png', animations: 'disabled' })
  expect(external).toEqual([])
  expect(errors).toEqual([])
})
