import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { applyMessage, parseSnapshot } from '../../src/twin/projection'
import { stations } from '../../src/data/stations'

describe.skipIf(!process.env.W4_PARITY)('W3 offline projection parity', () => {
  it('matches W3 after the complete W2 event stream', () => {
    const expected = JSON.parse(readFileSync('/tmp/opencode/w4-snapshot.json', 'utf8'))
    const events = readFileSync('/tmp/opencode/w4-events.ndjson', 'utf8').trim().split('\n').map(line => JSON.parse(line))
    const machines = Object.fromEntries(stations.map(s => [s.id, { observedState: '', flowState: '', reason: '', alarms: {} }]))
    let state = parseSnapshot({ runId: 'demo-run', epoch: 'fixture', revision: 0, lastSequence: 0, simTimeMs: 0, integrity: 'uncertain', machines, boards: {}, workOrders: {} }, 'demo-run')!
    for (const event of events) {
      const result = applyMessage(state, { type: 'delta', epoch: 'fixture', revision: state.revision + 1, integrity: 'complete', event })
      if (result.kind === 'resync') throw new Error(`sequence ${event.sequence} ${event.eventType}: ${result.reason}`)
      state = result.projection
    }
    expect(state.machines).toEqual(expected.machines)
    expect(state.boards).toEqual(expected.boards)
    expect(state.workOrders).toEqual(expected.workOrders)
    expect([state.revision, state.lastSequence, state.simTimeMs]).toEqual([expected.revision, expected.lastSequence, expected.simTimeMs])
  })
})
