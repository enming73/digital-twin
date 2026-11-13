import { expect, test } from '@playwright/test'

test('installs the recovered snapshot with a new epoch without reconnecting', async ({ page }) => {
  let connections = 0
  const stationIds = ['loader', 'printer', 'spi', 'mounter', 'reflow', 'aoi', 'unloader']
  const machines = Object.fromEntries(stationIds.map(id => [id, { observedState: 'IDLE', flowState: 'NORMAL', reason: 'idle', alarms: {} }]))
  const runningBoard = { id: 'board-1', workOrderId: 'work-order-1', machineId: 'loader', status: 'at_station', processed: false, inspections: {} }
  const completedBoards = Object.fromEntries(Array.from({ length: 100 }, (_, index) => {
    const id = `board-${index + 1}`
    return [id, { id, workOrderId: 'work-order-1', machineId: 'unloader', status: 'completed', processed: true, inspections: {} }]
  }))

  let pushRecovered = () => {}
  await page.routeWebSocket('**/ws/runs/**', socket => {
    connections++
    socket.send(JSON.stringify({
      type: 'snapshot',
      epoch: 'epoch-live',
      revision: 1,
      snapshot: { runId: 'demo-run', epoch: 'epoch-live', revision: 1, lastSequence: 1, simTimeMs: 10, integrity: 'gap', reason: 'expected sequence 2, got 400', machines, boards: { 'board-1': runningBoard }, workOrders: { 'work-order-1': { id: 'work-order-1', quantity: 100, passed: 0, failed: 0, status: 'running' } } },
    }))
    // Pushed explicitly below: the replacement must not race the first-snapshot assertion.
    pushRecovered = () => socket.send(JSON.stringify({
      type: 'snapshot',
      epoch: 'epoch-live-r3411',
      revision: 3411,
      integrity: 'complete',
      snapshot: {
        runId: 'demo-run',
        epoch: 'epoch-live-r3411',
        revision: 3411,
        lastSequence: 3411,
        simTimeMs: 20440,
        integrity: 'complete',
        terminalVerified: true,
        machines,
        boards: completedBoards,
        workOrders: { 'work-order-1': { id: 'work-order-1', quantity: 100, passed: 98, failed: 2, status: 'completed' } },
      },
    }))
    const statusAfterRecovery = () => socket.send(JSON.stringify({ type: 'status', epoch: 'epoch-live-r3411', integrity: 'complete' }))
    const pushSnapshot = pushRecovered
    pushRecovered = () => { pushSnapshot(); statusAfterRecovery() }
  })

  await page.goto('/')
  await page.getByLabel('Data source', { exact: true }).selectOption('remote')
  await expect(page.getByText('gap', { exact: true })).toBeVisible()
  await expect(page.locator('.scene')).toHaveAttribute('data-visible-boards', '1')
  pushRecovered()

  await expect(page.getByText('epoch-live-r3411')).toBeVisible({ timeout: 10000 })
  await expect(page.getByText('complete', { exact: true })).toBeVisible()
  await expect(page.locator('.scene')).toHaveAttribute('data-visible-boards', '0', { timeout: 10000 })
  expect(connections).toBe(1)
})
