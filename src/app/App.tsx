import { Component, Suspense, useEffect, useRef, useState, type ReactNode } from 'react'
import Factory from '../scene/Factory'
import { stations, type Station } from '../data/stations'
import { defaultScenario } from '../data/scenario'
import { Simulation } from '../simulation/engine'
import Dashboard from '../components/Dashboard'
import RemoteTrace from '../components/RemoteTrace'
import { replayAt } from '../domain/replay'
import { Presentation } from '../presentation/presentation'
import { getMetrics } from '../domain/analytics'
import { localScene } from '../twin/localAdapter'
import { remoteScene } from '../twin/remoteAdapter'
import { createRemoteClient, type RemoteClientState } from '../twin/remoteClient'
import type { SourceMode } from '../twin/types'
import { nextRemoteVisual, resetRemoteVisual, type RemoteVisualState } from '../scene/remoteMotion'

class SceneError extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }
  static getDerivedStateFromError() { return { failed: true } }
  render() { return this.state.failed ? <div className="canvas-fallback">Unable to load the 3D scene. Check your browser's WebGL support and reload.</div> : this.props.children }
}

export default function App() {
  const [selected, setSelected] = useState<Station | null>(null)
  const [resetKey, setResetKey] = useState(0)
  const [simulation] = useState(() => new Simulation(defaultScenario))
  const [presentation] = useState(() => new Presentation(simulation))
  const [tourActive, setTourActive] = useState(false)
  const [liveSnapshot, setSnapshot] = useState(() => simulation.snapshot())
  const [speed, setSpeed] = useState(1)
  const [selectedBoard, setSelectedBoard] = useState<string | null>(null)
  const [replayTime, setReplayTime] = useState<number | null>(null)
  const [replayPlaying, setReplayPlaying] = useState(false)
  const [source, setSource] = useState<SourceMode>('local')
  const [runId, setRunId] = useState('demo-run')
  const [remote, setRemote] = useState<RemoteClientState | null>(null)
  const [visual, setVisual] = useState<RemoteVisualState | null>(null)
  const visualRef = useRef<RemoteVisualState | null>(null)
  const snapshotVersion = useRef(0)
  useEffect(() => {
    if (source !== 'remote' || !runId) return
    visualRef.current = null; snapshotVersion.current = 0
    const client = createRemoteClient({ runId, onState: state => {
      if (state.projection) {
        const fresh = state.snapshotVersion !== snapshotVersion.current || !visualRef.current
        visualRef.current = fresh ? resetRemoteVisual(state.projection) : nextRemoteVisual(visualRef.current!, state.projection, Date.now())
        snapshotVersion.current = state.snapshotVersion
        setVisual(visualRef.current)
        setSelectedBoard(id => id && (!state.projection!.boards[id] || state.projection!.boards[id].status === 'completed') ? null : id)
      }
      setRemote(state)
    } })
    client.start()
    return () => client.stop()
  }, [source, runId])
  const events = simulation.events()
  const snapshot = replayTime === null ? liveSnapshot : replayAt(defaultScenario, events, replayTime)
  const visibleEvents = replayTime === null ? events : events.filter(event => event.simTimeMs <= replayTime)
  useEffect(() => {
    if (!liveSnapshot.running || replayTime !== null) return
    let last = performance.now()
    const timer = window.setInterval(() => { const now = performance.now(); const delta = now - last; if (tourActive) presentation.advance(delta * speed); else simulation.advanceWall(delta, speed); last = now; setSnapshot(simulation.snapshot()) }, 100)
    return () => window.clearInterval(timer)
  }, [simulation, presentation, liveSnapshot.running, speed, replayTime, tourActive])
  useEffect(() => {
    if (!replayPlaying || replayTime === null) return
    let last = performance.now()
    const timer = window.setInterval(() => { const now = performance.now(); const delta = (now - last) * speed; last = now; setReplayTime(current => current === null ? null : Math.min(liveSnapshot.simTimeMs, current + delta)) }, 100)
    return () => window.clearInterval(timer)
  }, [replayPlaying, replayTime === null, speed, liveSnapshot.simTimeMs])
  useEffect(() => { if (replayTime !== null && replayTime >= liveSnapshot.simTimeMs) setReplayPlaying(false) }, [replayTime, liveSnapshot.simTimeMs])
  const toggle = () => { if (liveSnapshot.running) simulation.pause(); else simulation.start(); setSnapshot(simulation.snapshot()) }
  const resetSimulation = () => { simulation.reset(); setSnapshot(simulation.snapshot()); setSelectedBoard(null); setReplayTime(null); setReplayPlaying(false) }
  const openReplay = () => { simulation.pause(); const state = simulation.snapshot(); setSnapshot(state); setReplayTime(state.simTimeMs); setReplayPlaying(false); setSelectedBoard(null) }
  const faulted = liveSnapshot.stations.find(s => s.id === 'mounter')?.state === 'FAULT'
  const toggleFault = () => { if (faulted) simulation.clearFault('mounter'); else simulation.injectFault('mounter'); setSnapshot(simulation.snapshot()) }
  const startTour = () => { presentation.start(); setReplayTime(null); setReplayPlaying(false); setSpeed(1); setSelectedBoard(null); setTourActive(true); setSnapshot(simulation.snapshot()); setSelected(null); setResetKey(n => n + 1) }
  const pauseTour = () => { if (presentation.running) presentation.pause(); else presentation.resume(); setSnapshot(simulation.snapshot()) }
  const exitTour = () => { presentation.pause(); setSnapshot(simulation.snapshot()); setTourActive(false) }
  const chapter = presentation.chapter
  useEffect(() => { if (tourActive) { setSelected(stations.find(station => station.id === chapter.station) ?? null); setResetKey(n => n + 1) } }, [tourActive, chapter.id, chapter.station])
  const tourMetrics = getMetrics(snapshot, visibleEvents, 300000)
  const chooseBoard = (id: string | null) => { setSelectedBoard(id); const stationId = source === 'local' ? snapshot.boards.find(b => b.id === id)?.stationId : id ? remote?.projection?.boards[id]?.machineId : null; if (stationId) setSelected(stations.find(s => s.id === stationId) ?? null) }
  const seconds = Math.floor(snapshot.simTimeMs / 1000)
  const displayTime = `${String(Math.floor(seconds / 60)).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}.${Math.floor(snapshot.simTimeMs % 1000 / 100)}`
  const reset = () => { setSelected(null); setResetKey(n => n + 1) }
  const scene = source === 'local' ? localScene(snapshot) : remote?.projection && visual ? remoteScene(remote.projection, visual) : { simTimeMs: 0, stations: {}, boards: [] }
  const switchSource = (mode: SourceMode) => {
    if (mode === 'remote') { if (tourActive) presentation.pause(); else simulation.pause(); setSnapshot(simulation.snapshot()); setTourActive(false); setReplayTime(null); setReplayPlaying(false) }
    setSelectedBoard(null); setRemote(null); setVisual(null); setSource(mode)
  }
  return <div className="app-shell">
    <header className="topbar"><div className="brand-mark">S<span> / </span>T</div><div className="brand"><strong>SMT <span>STUDIO</span></strong><small>DIGITAL TWIN / CONCEPT</small></div><div className="header-right"><span className="header-dot" /> INDEPENDENT DEMONSTRATION <span className="header-version">V 1.0</span></div></header>
    <main className="workspace">
      <section className="viewport" aria-label="3D factory view">
        <div className="viewport-heading"><div><div className="eyebrow"><span className="live-dot" /> FACTORY VIEW <span className="separator">/</span> LINE 01</div><h1>Production line<span>.</span></h1><p>A closer look at the electronics assembly process.</p></div><div className="scene-counter"><strong>07</strong><span>CONCEPTUAL<br />STATIONS</span></div></div>
        <div className="scene" data-visible-boards={scene.boards.length}><SceneError><Suspense fallback={<div className="canvas-fallback">Loading factory scene…</div>}><Factory selected={selected} onSelect={setSelected} resetKey={resetKey} scene={scene} selectedBoard={selectedBoard} onSelectBoard={chooseBoard} /></Suspense></SceneError></div>
        <div className="view-actions"><div className="view-hint"><span>↖</span> DRAG TO ORBIT <i /> SCROLL TO ZOOM <i /> SELECT A STATION</div><button className="reset" onClick={reset} aria-label="Reset camera to factory overview"><span>↗</span> Reset view</button></div>
        <div className="viewport-footer"><span><b /> Illustrative animation · Not measured machine coordinates</span><span>SIMULATED FLOW <span className="separator">/</span> NO LIVE DATA</span></div>
      </section>
      <aside className="sidebar" aria-label="Line overview and station details"><div className="sidebar-heading"><div className="eyebrow">LINE 01 <span className="separator">/</span> OVERVIEW</div><h2>Assembly sequence</h2><p>Select a station to explore the line.</p></div>
        <section className="source-panel" aria-label="Data source selection"><div className="eyebrow">DATA SOURCE / SIMULATED</div><label>Data source <select aria-label="Data source" value={source} onChange={event => switchSource(event.target.value as SourceMode)}><option value="local">Local demo</option><option value="remote">Remote W3 · read only</option></select></label>{source === 'remote' && <><label>Run ID <input aria-label="Run ID" value={runId} onChange={event => { setRunId(event.target.value); setRemote(null); setVisual(null); setSelectedBoard(null) }} /></label><div aria-live="polite"><p>SIMULATED · {remote?.connection ?? 'connecting'}{remote?.stale ? ' · activity stale' : ''}</p><p>Integrity: <strong>{remote?.projection?.integrity ?? 'unknown'}</strong> {remote?.projection?.reason}</p><p>Epoch: <span>{remote?.projection?.epoch ?? '—'}</span> · revision {remote?.projection?.revision ?? '—'}</p><p>Last message: {remote?.lastReceivedAt ? new Date(remote.lastReceivedAt).toLocaleTimeString() : '—'}</p>{remote?.reason && <p>{remote.reason}</p>}<p>Read only · local presentation, controls, history and KPIs are unavailable. Activity can be stale during a quiet completed run.</p></div></>}</section>
        {source === 'local' && <>
        <section className="presentation-panel" aria-label="Guided presentation"><div className="eyebrow">PRESENTATION / SIMULATED</div>{!tourActive ? <><h2>Guided demonstration</h2><p>A 3½-minute virtual-time tour of the line, fault and recovery.</p><button onClick={startTour} aria-label="Start guided presentation">▶ Start guided presentation</button></> : <><div className="tour-progress">{Math.min(100, snapshot.simTimeMs / 2100).toFixed(0)}% / 03:30 AT 1×</div><h2>{chapter.title}</h2><p>{chapter.caption}</p>{chapter.id === 'summary' && <p>SIMULATED · Completed {snapshot.completed}/{snapshot.quantity} · WIP {tourMetrics.wip} · Throughput {tourMetrics.throughputPerHour === null ? '—' : tourMetrics.throughputPerHour.toFixed(1)}/h</p>}<div className="tour-buttons"><button onClick={pauseTour} disabled={presentation.finished} aria-label={presentation.running ? 'Pause presentation' : 'Resume presentation'}>{presentation.running ? 'Pause' : 'Resume'}</button><button onClick={startTour} aria-label="Restart presentation">Restart</button><button onClick={exitTour} aria-label="Exit presentation">Exit</button></div>{presentation.finished && <p>Presentation complete. Exit to inspect the simulated event replay.</p>}</>}</section>
        <fieldset disabled={tourActive} className="manual-controls">
        <section className="simulation-panel" aria-label="Simulation controls"><div className="sim-heading"><span>{replayTime === null ? 'SIMULATION' : 'REPLAY · READ ONLY'}</span><b>SIMULATED</b></div><div className="sim-metrics"><div><small>VIRTUAL TIME</small><strong data-testid="simulation-time">{displayTime}</strong></div><div><small>CREATED</small><strong data-testid="created-count">{snapshot.created}</strong></div><div><small>COMPLETED</small><strong>{snapshot.completed} / {snapshot.quantity}</strong></div></div><div className="sim-controls"><button onClick={toggle} disabled={replayTime !== null || snapshot.completed === snapshot.quantity} aria-label={liveSnapshot.running ? 'Pause simulation' : 'Start simulation'}>{liveSnapshot.running ? 'Pause' : liveSnapshot.created ? 'Resume' : 'Start'}</button><button onClick={resetSimulation} disabled={replayTime !== null} aria-label="Reset simulation">Reset</button><label>Speed <select value={speed} onChange={e => setSpeed(Number(e.target.value))} aria-label="Simulation speed"><option value={1}>1×</option><option value={2}>2×</option><option value={5}>5×</option></select></label></div><div className="sim-controls replay-actions"><button onClick={toggleFault} disabled={replayTime !== null || liveSnapshot.completed === liveSnapshot.quantity} aria-label={faulted ? 'Clear mounter fault' : 'Inject mounter fault'}>{faulted ? 'Clear fault' : 'Inject fault'}</button>{replayTime === null ? <button onClick={openReplay} aria-label="Open replay">Open replay</button> : <button onClick={() => { setReplayTime(null); setReplayPlaying(false); setSelectedBoard(null) }} aria-label="Return to live">Return to live</button>}</div>{replayTime !== null && <div className="replay-controls"><label>Seek virtual time <input type="range" aria-label="Replay time" min={0} max={liveSnapshot.simTimeMs} step="1" value={replayTime} onChange={e => { setReplayTime(Number(e.target.value)); setReplayPlaying(false) }} /></label><button onClick={() => { if (replayTime >= liveSnapshot.simTimeMs) setReplayTime(0); setReplayPlaying(!replayPlaying) }} aria-label={replayPlaying ? 'Pause replay' : 'Play replay'}>{replayPlaying ? 'Pause replay' : 'Play replay'}</button><ol aria-label="Simulated event timeline">{events.filter(e => ['FAULT_INJECTED', 'FAULT_CLEARED', 'WORK_ORDER_COMPLETED'].includes(e.type)).map(e => <li key={e.id}><button onClick={() => { setReplayTime(e.simTimeMs); setReplayPlaying(false) }}>{(e.simTimeMs / 1000).toFixed(1)}s · <span>{e.type}</span></button></li>)}</ol></div>}</section>
        </fieldset>
        </>}
        <div className="station-list">{stations.map((station, index) => <button key={station.id} className={'station-item' + (selected?.id === station.id ? ' selected' : '')} onClick={() => setSelected(station)} aria-pressed={selected?.id === station.id}><span className="station-number">{String(index + 1).padStart(2, '0')}</span><span className="station-name"><strong>{station.name}</strong><small>{station.short} <span>·</span> {scene.stations[station.id]?.state ?? 'UNKNOWN'} / SIMULATED</small></span><span className="station-arrow">↗</span></button>)}</div>
        <div className="detail" aria-live="polite"><div className="eyebrow">{selected ? 'STATION DETAILS' : 'EXPLORE THE LINE'}</div><h3>{selected ? selected.name : 'Select a station'}</h3><p>{selected ? selected.description : 'Choose a station above or select a machine in the factory view to focus the camera.'}</p>{selected && <><button className="refocus" onClick={() => setResetKey(n => n + 1)} aria-label="Refocus selected machine">↗ Refocus close-up</button>{source === 'local' ? snapshot.stations.filter(s => s.id === selected.id).map(machine => <p key={machine.id}>SIMULATED · {machine.state} · input {machine.queue.length}/{machine.inputCapacity} · cycle {machine.processTimeMs / 1000}s · board {machine.boardId ?? '—'}</p>) : <p>SIMULATED · observed {scene.stations[selected.id]?.observedState || 'unknown'} · flow {scene.stations[selected.id]?.flowState || 'unknown'} · {scene.stations[selected.id]?.reason || 'no reason'}</p>}</>}<div className="detail-meta"><span>MODEL <b>CONCEPTUAL</b></span><span>DATA <b>SIMULATED</b></span></div><p>{source === 'local' ? 'Amber tower is decorative; station states are simulated.' : 'Indicators reflect reported simulated state, not equipment telemetry.'}</p></div>
        {source === 'remote' && selected && remote?.projection?.machines[selected.id] && <div className="detail"><strong>SIMULATED ALARMS</strong>{Object.entries(remote.projection.machines[selected.id].alarms).length ? Object.entries(remote.projection.machines[selected.id].alarms).map(([id, reason]) => <p key={id}>{id} · {reason}</p>) : <p>None reported</p>}</div>}
        {source === 'local' && <Dashboard snapshot={snapshot} events={visibleEvents} selectedBoard={selectedBoard} onSelectBoard={chooseBoard} onSelectStation={id => setSelected(stations.find(s => s.id === id) ?? null)} />}
        {source === 'remote' && <RemoteTrace runId={runId} epoch={remote?.projection?.epoch} selectedBoard={selectedBoard} />}
      </aside>
    </main>
    <footer className="disclaimer"><span>●</span> Concept demo · Synthetic data · Not affiliated with ASMPT <span className="footer-end">INDEPENDENTLY MODELED / V1.0</span></footer>
  </div>
}
