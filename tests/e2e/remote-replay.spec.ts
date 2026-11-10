import { expect, test } from '@playwright/test'

test('remote historical seek is read-only and can return to live', async ({ page }) => {
  const snapshot = { runId: 'demo-run', epoch: 'live', revision: 0, lastSequence: 0, simTimeMs: 0, integrity: 'uncertain', machines: {}, boards: {}, workOrders: {} }
  await page.routeWebSocket('**/ws/runs/**', socket => socket.send(JSON.stringify({ type: 'snapshot', epoch: 'live', revision: 0, snapshot })))
  await page.route('**/api/runs/demo-run/replay?simTimeMs=5', route => route.fulfill({ json: { mode: 'historical', synthetic: true, requestedSimTimeMs: 5, snapshot: { ...snapshot, epoch: 'historical' } } }))
  await page.goto('/')
  await page.getByLabel('Data source', { exact: true }).selectOption('remote')
  await page.getByLabel('Historical simulated milliseconds').fill('5')
  await page.getByRole('button', { name: 'Seek remote history' }).click()
  await expect(page.getByText('HISTORICAL · SIMULATED · READ ONLY')).toBeVisible()
  await expect(page.getByRole('region', { name: 'Remote simulated MES' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Return to remote live' }).click()
  await expect(page.getByText('HISTORICAL · SIMULATED · READ ONLY')).toHaveCount(0)
  await expect(page.getByRole('region', { name: 'Remote simulated MES' })).toBeVisible()
})

test('source change cancels a pending historical request', async ({ page }) => {
  let release!: () => void
  let started!: () => void
  const blocked = new Promise<void>(resolve => { release = resolve })
  const seen = new Promise<void>(resolve => { started = resolve })
  const snapshot = { runId: 'demo-run', epoch: 'live', revision: 0, lastSequence: 0, simTimeMs: 0, integrity: 'uncertain', machines: {}, boards: {}, workOrders: {} }
  await page.routeWebSocket('**/ws/runs/**', socket => socket.send(JSON.stringify({ type: 'snapshot', epoch: 'live', revision: 0, snapshot })))
  await page.route('**/api/runs/demo-run/replay?simTimeMs=5', async route => {
    started(); await blocked
    await route.fulfill({ json: { mode: 'historical', synthetic: true, requestedSimTimeMs: 5, snapshot: { ...snapshot, epoch: 'historical' } } }).catch(() => {})
  })
  await page.goto('/')
  await page.getByLabel('Data source', { exact: true }).selectOption('remote')
  await page.getByLabel('Historical simulated milliseconds').fill('5')
  await page.getByRole('button', { name: 'Seek remote history' }).click()
  await seen
  await page.getByLabel('Data source', { exact: true }).selectOption('local')
  release()
  await expect(page.getByRole('button', { name: 'Start guided presentation' })).toBeVisible()
  await expect(page.getByText('HISTORICAL · SIMULATED · READ ONLY')).toHaveCount(0)
})

test('a superseded historical request cannot replace the newer result', async ({ page }) => {
  let release!: () => void
  let started!: () => void
  const blocked = new Promise<void>(resolve => { release = resolve })
  const seen = new Promise<void>(resolve => { started = resolve })
  const snapshot = { runId: 'demo-run', epoch: 'live', revision: 0, lastSequence: 0, simTimeMs: 0, integrity: 'uncertain', machines: {}, boards: {}, workOrders: {} }
  await page.routeWebSocket('**/ws/runs/**', socket => socket.send(JSON.stringify({ type: 'snapshot', epoch: 'live', revision: 0, snapshot })))
  await page.route('**/api/runs/demo-run/replay?simTimeMs=*', async route => {
    const at = Number(new URL(route.request().url()).searchParams.get('simTimeMs'))
    if (at === 5) { started(); await blocked }
    await route.fulfill({ json: { mode: 'historical', synthetic: true, requestedSimTimeMs: at, snapshot: { ...snapshot, epoch: 'historical', revision: at === 10 ? 2 : 1, lastSequence: at === 10 ? 2 : 1 } } }).catch(() => {})
  })
  await page.goto('/')
  await page.getByLabel('Data source', { exact: true }).selectOption('remote')
  await page.getByLabel('Historical simulated milliseconds').fill('5')
  await page.getByRole('button', { name: 'Seek remote history' }).click()
  await seen
  await page.getByLabel('Historical simulated milliseconds').fill('10')
  await page.getByRole('button', { name: 'Seek remote history' }).click()
  await expect(page.getByText('Sequence 2 · integrity uncertain')).toBeVisible()
  release()
  await expect(page.getByText('Sequence 2 · integrity uncertain')).toBeVisible()
})
