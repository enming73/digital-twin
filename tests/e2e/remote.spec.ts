import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'

test('switches explicitly to a simulated W3 snapshot and back to paused local mode', async ({ page }) => {
  let connections = 0
  await page.routeWebSocket('**/ws/runs/**', socket => {
    connections++
    socket.send(JSON.stringify({ type: 'snapshot', epoch: 'test-epoch', revision: 0, snapshot: { runId: 'demo-run', epoch: 'test-epoch', revision: 0, lastSequence: 0, simTimeMs: 0, integrity: 'uncertain', reason: 'awaiting first event', machines: { loader: { observedState: 'FAULT', flowState: 'BLOCKED', reason: 'simulated fault', alarms: {} } }, boards: {}, workOrders: {} } }))
  })
  await page.goto('/')
  expect(connections).toBe(0)
  await page.getByRole('button', { name: 'Start simulation' }).click()
  await page.getByLabel('Data source', { exact: true }).selectOption('remote')
  await expect(page.getByText('test-epoch')).toBeVisible()
  await expect(page.getByText('uncertain', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Inject mounter fault' })).toHaveCount(0)
  expect(connections).toBe(1)
  await page.getByLabel('Data source', { exact: true }).selectOption('local')
  await expect(page.getByRole('button', { name: 'Start simulation' })).toHaveText('Resume')
})

test('replays the 100-board W2 stream through the remote UI', async ({ page }) => {
  test.skip(!process.env.W4_PARITY, 'requires generated /tmp/opencode/w4-events.ndjson')
  test.setTimeout(90000)
  const events = readFileSync('/tmp/opencode/w4-events.ndjson', 'utf8').trim().split('\n').map(line => JSON.parse(line))
  await page.routeWebSocket('**/ws/runs/**', socket => {
    const machines = Object.fromEntries(['loader', 'printer', 'spi', 'mounter', 'reflow', 'aoi', 'unloader'].map(id => [id, { observedState: '', flowState: '', reason: '', alarms: {} }]))
    const snapshot = { runId: 'demo-run', epoch: 'fixture', revision: 0, lastSequence: 0, simTimeMs: 0, integrity: 'uncertain', machines, boards: {}, workOrders: {} }
    socket.send(JSON.stringify({ type: 'snapshot', epoch: 'fixture', revision: 0, snapshot }))
    for (const event of events) socket.send(JSON.stringify({ type: 'delta', epoch: 'fixture', revision: event.sequence, integrity: 'complete', event }))
  })
  await page.goto('/')
  await page.getByLabel('Data source', { exact: true }).selectOption('remote')
  await expect(page.getByText(`Epoch: fixture · revision ${events.length}`)).toBeVisible({ timeout: 60000 })
  await expect(page.locator('.scene')).toHaveAttribute('data-visible-boards', '0')
  await expect(page.getByText('complete', { exact: true })).toBeVisible()
})

test('replaces the snapshot and removes stale boards after reconnect', async ({ page }) => {
  let connection = 0
  await page.routeWebSocket('**/ws/runs/**', socket => {
    connection++
    const epoch = connection === 1 ? 'first-epoch' : 'new-epoch'
    const board = { id: 'b1', workOrderId: 'wo', machineId: 'loader', status: 'at_station', processed: false, inspections: {} }
    const loader = { observedState: 'RUNNING', flowState: 'NORMAL', reason: '', alarms: {} }
    socket.send(JSON.stringify({ type: 'snapshot', epoch, revision: 0, snapshot: { runId: 'demo-run', epoch, revision: 0, lastSequence: 0, simTimeMs: 0, integrity: 'uncertain', machines: { loader }, boards: connection === 1 ? { b1: board } : {}, workOrders: {} } }))
    if (connection === 1) {
      socket.send(JSON.stringify({ type: 'status', epoch, integrity: 'gap', reason: 'missing event' }))
      setTimeout(() => socket.close(), 700)
    }
  })
  await page.goto('/')
  await page.getByLabel('Data source', { exact: true }).selectOption('remote')
  await expect(page.locator('.scene')).toHaveAttribute('data-visible-boards', '1')
  await expect(page.getByText('gap', { exact: true })).toBeVisible()
  await expect(page.getByText('new-epoch')).toBeVisible({ timeout: 10000 })
  await expect(page.locator('.scene')).toHaveAttribute('data-visible-boards', '0')
  await expect(page.getByText('uncertain', { exact: true })).toBeVisible()
  expect(connection).toBeGreaterThanOrEqual(2)
})
