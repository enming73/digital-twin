import type { Simulation } from '../simulation/engine'

export const chapters = [
  { id: 'overview', atMs: 0, station: null, title: 'A line in context', caption: 'Seven conceptual stations form a single simulated SMT line. All data and motion are illustrative.' },
  { id: 'printer', atMs: 20000, station: 'printer', title: 'Print and inspect', caption: 'The conceptual printer deposits paste before inspection; geometry is independently modeled.' },
  { id: 'mounter', atMs: 45000, station: 'mounter', title: 'Place components', caption: 'The simulated mounter processes boards using bounded FIFO buffers.' },
  { id: 'fault', atMs: 70000, station: 'mounter', title: 'A simulated interruption', caption: 'A scripted mounter fault holds its board. Upstream blocks and downstream starves only as buffers fill or drain.' },
  { id: 'recovery', atMs: 145000, station: 'mounter', title: 'Recover the line', caption: 'The fault clears and the held board resumes its remaining simulated process time.' },
  { id: 'summary', atMs: 190000, station: null, title: 'Review the evidence', caption: 'The dashboard summarizes simulated work in progress, throughput and inspection results. Use replay to revisit the incident.' },
] as const

export class Presentation {
  private active = false
  private done = false
  constructor(private simulation: Simulation) {}
  get finished() { return this.done }
  get running() { return this.active }
  get chapter() { const time = this.simulation.snapshot().simTimeMs; return [...chapters].reverse().find(c => c.atMs <= time) ?? chapters[0] }
  start() { this.simulation.reset(); this.simulation.start(); this.active = true; this.done = false }
  pause() { if (this.active) { this.active = false; this.simulation.pause() } }
  resume() { if (!this.active && !this.done) { this.simulation.start(); this.active = true } }
  advance(ms: number) {
    if (!Number.isFinite(ms) || ms < 0) throw new Error('Invalid presentation advance')
    if (!this.active || this.done) return
    const target = Math.min(210000, this.simulation.snapshot().simTimeMs + ms)
    const boundaries = [70000, 145000, 210000]
    for (const boundary of boundaries) {
      const now = this.simulation.snapshot().simTimeMs
      if (now < boundary && target >= boundary) {
        this.simulation.advance(boundary - now)
        if (boundary === 70000) this.simulation.injectFault('mounter')
        if (boundary === 145000) this.simulation.clearFault('mounter')
        if (boundary === 210000) { this.simulation.pause(); this.active = false; this.done = true; return }
      }
    }
    this.simulation.advance(target - this.simulation.snapshot().simTimeMs)
  }
}
