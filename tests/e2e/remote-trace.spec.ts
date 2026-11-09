import { expect, test } from '@playwright/test'

test('remote MES lookup shows simulated board trace', async ({ page }) => {
  await page.routeWebSocket('**/ws/runs/**', socket => socket.send(JSON.stringify({ type: 'snapshot', epoch: 'epoch', revision: 0, snapshot: { runId: 'demo-run', epoch: 'epoch', revision: 0, lastSequence: 0, simTimeMs: 0, integrity: 'uncertain', machines: {}, boards: {}, workOrders: {} } })))
  await page.route('**/api/runs/demo-run/boards/board-1/trace', route => route.fulfill({ json: { runId: 'demo-run', epoch: 'epoch', revision: 4, integrity: 'complete', synthetic: true, route: ['spi'], board: { id: 'board-1', workOrderId: 'work-order-1', status: 'completed', processed: true, inspections: { spi: false } }, entries: [{ sequence: 4, eventId: 'e4', eventType: 'pcb.inspection.completed', machineId: 'spi', simTimeMs: 100, emittedAt: '2026-01-01T00:00:00Z', ingestedAt: '2026-01-01T00:00:01Z', passed: false }] } }))
  await page.route('**/api/runs/demo-run/workorders/work-order-1', route => route.fulfill({ json: { runId: 'demo-run', epoch: 'epoch', revision: 5, integrity: 'gap', synthetic: true, route: ['spi'], order: { id: 'work-order-1', quantity: 4, passed: 0, failed: 0, status: 'running' }, metadata: null, progress: { planned: 4, created: 4, inProgress: 1, completed: 3, passed: 1, failed: 1, uninspected: 1 }, reconciliation: 'pending' } }))
  await page.goto('/')
  await page.getByLabel('Data source', { exact: true }).selectOption('remote')
  await page.getByLabel('Remote board ID').fill('board-1')
  await page.getByRole('button', { name: 'Find remote board' }).click()
  await expect(page.getByText('pcb.inspection.completed')).toBeVisible()
  await expect(page.getByText('FAIL', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Find remote work order' }).click()
  await expect(page.getByText('Planned 4 · Created 4 · In progress 1 · Completed 3')).toBeVisible()
  await expect(page.getByText('Pass 1 · Fail 1 · Uninspected 1')).toBeVisible()
  await expect(page.getByText('SIMULATED · revision 5 · integrity gap')).toBeVisible()
})

test('rejects stale epoch and reports unknown board', async ({ page }) => {
  await page.routeWebSocket('**/ws/runs/**', socket => socket.send(JSON.stringify({ type: 'snapshot', epoch: 'current', revision: 0, snapshot: { runId: 'demo-run', epoch: 'current', revision: 0, lastSequence: 0, simTimeMs: 0, integrity: 'uncertain', machines: {}, boards: {}, workOrders: {} } })))
  await page.route('**/api/runs/demo-run/boards/**/trace', route => {
    if (route.request().url().includes('/missing/')) return route.fulfill({ status: 404 })
    return route.fulfill({ json: { runId: 'demo-run', epoch: 'old', revision: 1, integrity: 'complete', synthetic: true, route: ['spi'], board: { id: 'board-1', workOrderId: 'work-order-1', status: 'completed', processed: true, inspections: {} }, entries: [] } })
  })
  await page.goto('/')
  await page.getByLabel('Data source', { exact: true }).selectOption('remote')
  await expect(page.getByText('Epoch:')).toContainText('current')
  await page.getByLabel('Remote board ID').fill('board-1')
  await page.getByRole('button', { name: 'Find remote board' }).click()
  await expect(page.getByText('Epoch changed; refresh after reconnect')).toBeVisible()
  await page.getByLabel('Remote board ID').fill('missing')
  await page.getByRole('button', { name: 'Find remote board' }).click()
  await expect(page.getByText('HTTP 404')).toBeVisible()
})

test('a slower superseded lookup cannot replace a newer result', async ({ page }) => {
  let release!: () => void
  const blocked = new Promise<void>(resolve => { release = resolve })
  let firstStarted!: () => void
  const firstSeen = new Promise<void>(resolve => { firstStarted = resolve })
  await page.routeWebSocket('**/ws/runs/**', socket => socket.send(JSON.stringify({ type: 'snapshot', epoch: 'epoch', revision: 0, snapshot: { runId: 'demo-run', epoch: 'epoch', revision: 0, lastSequence: 0, simTimeMs: 0, integrity: 'uncertain', machines: {}, boards: {}, workOrders: {} } })))
  await page.route('**/api/runs/demo-run/boards/**/trace', async route => {
    const id = route.request().url().includes('/slow/') ? 'slow' : 'fast'
    if (id === 'slow') { firstStarted(); await blocked }
    await route.fulfill({ json: { runId: 'demo-run', epoch: 'epoch', revision: 1, integrity: 'complete', synthetic: true, route: ['spi'], board: { id, workOrderId: 'wo', status: 'completed', processed: true, inspections: {} }, entries: [] } }).catch(() => {})
  })
  await page.goto('/')
  await page.getByLabel('Data source', { exact: true }).selectOption('remote')
  await expect(page.getByText('Epoch:')).toContainText('epoch')
  await page.getByLabel('Remote board ID').fill('slow')
  await page.getByRole('button', { name: 'Find remote board' }).click()
  await firstSeen
  await page.getByLabel('Remote board ID').fill('fast')
  await page.getByRole('button', { name: 'Find remote board' }).click()
  await expect(page.getByText('fast / wo · completed')).toBeVisible()
  release()
  await expect(page.getByText('fast / wo · completed')).toBeVisible()
  await expect(page.getByText('slow / wo · completed')).toHaveCount(0)
})

test('switching to local cancels an in-flight remote lookup', async ({ page }) => {
  let release!: () => void
  const blocked = new Promise<void>(resolve => { release = resolve })
  let started!: () => void
  const seen = new Promise<void>(resolve => { started = resolve })
  await page.routeWebSocket('**/ws/runs/**', socket => socket.send(JSON.stringify({ type: 'snapshot', epoch: 'epoch', revision: 0, snapshot: { runId: 'demo-run', epoch: 'epoch', revision: 0, lastSequence: 0, simTimeMs: 0, integrity: 'uncertain', machines: {}, boards: {}, workOrders: {} } })))
  await page.route('**/api/runs/demo-run/boards/**/trace', async route => {
    started(); await blocked
    await route.fulfill({ json: { runId: 'demo-run', epoch: 'epoch', revision: 1, integrity: 'complete', synthetic: true, route: [], board: { id: 'board-1', workOrderId: 'wo', status: 'completed', processed: true, inspections: {} }, entries: [] } }).catch(() => {})
  })
  await page.goto('/')
  await page.getByLabel('Data source', { exact: true }).selectOption('remote')
  await expect(page.getByText('Epoch:')).toContainText('epoch')
  await page.getByLabel('Remote board ID').fill('board-1')
  await page.getByRole('button', { name: 'Find remote board' }).click()
  await seen
  await page.getByLabel('Data source', { exact: true }).selectOption('local')
  release()
  await expect(page.getByRole('region', { name: 'Remote simulated MES' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Start guided presentation' })).toBeVisible()
})

test('changing the run ID invalidates an in-flight lookup', async ({ page }) => {
  let release!: () => void
  const blocked = new Promise<void>(resolve => { release = resolve })
  let started!: () => void
  const seen = new Promise<void>(resolve => { started = resolve })
  await page.routeWebSocket('**/ws/runs/**', socket => {
    const runId = socket.url().endsWith('/new-run') ? 'new-run' : 'demo-run'
    socket.send(JSON.stringify({ type: 'snapshot', epoch: runId, revision: 0, snapshot: { runId, epoch: runId, revision: 0, lastSequence: 0, simTimeMs: 0, integrity: 'uncertain', machines: {}, boards: {}, workOrders: {} } }))
  })
  await page.route('**/api/runs/demo-run/boards/**/trace', async route => {
    started(); await blocked
    await route.fulfill({ json: { runId: 'demo-run', epoch: 'demo-run', revision: 1, integrity: 'complete', synthetic: true, route: [], board: { id: 'board-1', workOrderId: 'wo', status: 'completed', processed: true, inspections: {} }, entries: [] } }).catch(() => {})
  })
  await page.goto('/')
  await page.getByLabel('Data source', { exact: true }).selectOption('remote')
  await expect(page.getByText('Epoch:')).toContainText('demo-run')
  await page.getByLabel('Remote board ID').fill('board-1')
  await page.getByRole('button', { name: 'Find remote board' }).click()
  await seen
  await page.getByLabel('Run ID').fill('new-run')
  await expect(page.getByText('Epoch:')).toContainText('new-run')
  release()
  await expect(page.getByText('board-1 / wo · completed')).toHaveCount(0)
})
