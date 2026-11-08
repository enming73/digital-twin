import { useState } from 'react'
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { alarmEvents, boardHistory, getMetrics, throughputSeries } from '../domain/analytics'
import type { ProductionEvent, Snapshot } from '../simulation/engine'
import { stations } from '../data/stations'

function time(ms: number) { const seconds = Math.floor(ms / 1000); return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}` }
function number(value: number | null, digits = 1) { return value === null ? '—' : value.toFixed(digits) }

export default function Dashboard({ snapshot, events, selectedBoard, onSelectBoard, onSelectStation }: { snapshot: Snapshot; events: ProductionEvent[]; selectedBoard: string | null; onSelectBoard: (id: string | null) => void; onSelectStation: (id: string) => void }) {
  const [query, setQuery] = useState('')
  const [searched, setSearched] = useState(false)
  const [windowMs, setWindowMs] = useState(300000)
  const metrics = getMetrics(snapshot, events, windowMs)
  const board = snapshot.boards.find(b => b.id === selectedBoard)
  const history = board ? boardHistory(events, board.id) : []
  const alarms = alarmEvents(events).filter(e => e.simTimeMs >= Math.max(0, snapshot.simTimeMs - windowMs))
  const series = throughputSeries(events, snapshot.simTimeMs, windowMs).map(p => ({ ...p, label: time(p.timeMs) }))
  const find = () => { setSearched(true); onSelectBoard(snapshot.boards.find(b => b.id.toLowerCase() === query.trim().toLowerCase())?.id ?? null) }
  return <section className="dashboard" aria-label="Simulated analytics dashboard">
    <div className="dash-head"><div><div className="eyebrow">LINE 01 / MOCK MES</div><h2>Production insights</h2></div><span>SIMULATED</span></div>
    <div className="work-order"><span>WORK ORDER</span><strong>work-order-1</strong><small>{snapshot.completed} / {snapshot.quantity} completed</small><div className="progress-track"><div style={{ width: `${100 * snapshot.completed / snapshot.quantity}%` }} /></div></div>
    <label className="window-label">Time window <select aria-label="Analytics time window" value={windowMs} onChange={e => setWindowMs(Number(e.target.value))}><option value={60000}>1 min</option><option value={300000}>5 min</option><option value={1800000}>30 min</option></select></label>
    <div className="kpi-grid"><div><small>THROUGHPUT / H</small><strong>{number(metrics.throughputPerHour)}</strong></div><div><small>WIP / NOW</small><strong>{metrics.wip}</strong></div><div><small>AVG CYCLE / S</small><strong>{number(metrics.avgCycleMs === null ? null : metrics.avgCycleMs / 1000)}</strong></div><div><small>BOARD PASS RATE</small><strong>{metrics.qualityPassRate === null ? '—' : `${number(metrics.qualityPassRate * 100)}%`}</strong></div></div>
    <div className="chart-title">COMPLETIONS / WINDOW <span>SIMULATED</span></div><div className="chart" aria-label="Simulated completion chart"><ResponsiveContainer width="100%" height={130}><BarChart data={series} margin={{ top: 5, right: 4, left: -25, bottom: 0 }}><CartesianGrid vertical={false} stroke="#d7e0d6" /><XAxis dataKey="label" tick={{ fontSize: 8, fill: '#798b7d' }} minTickGap={20} /><YAxis allowDecimals={false} tick={{ fontSize: 8, fill: '#798b7d' }} /><Tooltip /><Bar dataKey="count" fill="#638670" name="Completed boards" /></BarChart></ResponsiveContainer></div>
    <div className="trace"><div className="chart-title">BOARD TRACE <span>SIMULATED</span></div><form onSubmit={e => { e.preventDefault(); find() }}><label htmlFor="board-id">Board ID</label><div><input id="board-id" value={query} onChange={e => { setQuery(e.target.value); setSearched(false) }} placeholder="e.g. board-1" /><button type="submit">Find board</button></div></form>
      {searched && !board && <p className="trace-note">Board not found in this run.</p>}
      {board && <div className="board-result"><h3>{board.id}</h3><p>Work order: <b>{board.workOrderId}</b> · {board.status}</p><p>Current station: {board.stationId ? <button className="station-link" onClick={() => onSelectStation(board.stationId!)}>{stations.find(s => s.id === board.stationId)?.name ?? board.stationId}</button> : 'Completed / off line'}</p><ol>{history.map(e => <li key={e.id}><time>{time(e.simTimeMs)}</time><span>{e.type}</span><small>{String(e.payload.stationId ?? (e.payload.to ? `${e.payload.from} → ${e.payload.to}` : ''))}{e.type === 'INSPECTION_RECORDED' ? (e.payload.passed ? ' / PASS' : ' / FAIL') : ''}</small></li>)}</ol></div>}
    </div>
    <div className="alarms"><div className="chart-title">SIMULATED ALERTS <span>{alarms.length} / WINDOW</span></div>{alarms.length === 0 ? <p>No simulated alerts in this window.</p> : alarms.slice(-5).reverse().map(e => <p key={e.id}>{time(e.simTimeMs)} · {e.type === 'FAULT_INJECTED' ? `fault at ${String(e.payload.stationId)}` : `${String(e.payload.boardId)} · ${String(e.payload.stationId)} failed`}</p>)}<small>Simulated inspection failures and injected faults only.</small></div>
  </section>
}
