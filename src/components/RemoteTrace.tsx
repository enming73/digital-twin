import { useEffect, useRef, useState } from 'react'
import { getBoardTrace, getWorkOrder, type BoardTrace, type WorkOrderDetail } from '../twin/mesClient'

type Props = { runId: string; epoch: string | undefined; selectedBoard: string | null }
export default function RemoteTrace({ runId, epoch, selectedBoard }: Props) {
  const [boardId, setBoardId] = useState('')
  const [orderId, setOrderId] = useState('work-order-1')
  const [board, setBoard] = useState<BoardTrace | null>(null)
  const [order, setOrder] = useState<WorkOrderDetail | null>(null)
  const [status, setStatus] = useState('Enter an ID to query the simulated MES.')
  const controller = useRef<AbortController | null>(null)
  const generation = useRef(0)
  useEffect(() => { generation.current++; controller.current?.abort(); setBoard(null); setOrder(null); setStatus('Run or epoch changed; search again.'); return () => { generation.current++; controller.current?.abort() } }, [runId, epoch])
  useEffect(() => { if (selectedBoard) setBoardId(selectedBoard) }, [selectedBoard])
  const search = async (kind: 'board' | 'order') => {
    controller.current?.abort()
    const request = new AbortController(); controller.current = request
    const version = ++generation.current
    setBoard(null); setOrder(null); setStatus('Loading simulated MES…')
    try {
      if (!epoch) throw new Error('Wait for a remote snapshot')
      const result = kind === 'board' ? await getBoardTrace(runId, boardId, request.signal) : await getWorkOrder(runId, orderId, request.signal)
      if (version !== generation.current) return
      if (result.epoch !== epoch) throw new Error('Epoch changed; refresh after reconnect')
      if ('entries' in result) setBoard(result); else setOrder(result)
      setStatus('Point-in-time result; search again to refresh.')
    } catch (error) { if (version === generation.current && !request.signal.aborted) setStatus(error instanceof Error ? error.message : 'Request failed') }
  }
  return <section className="detail" aria-label="Remote simulated MES"><div className="eyebrow">TRACEABILITY / SIMULATED</div><h3>Mock MES · read only</h3>
    <label>Board ID <input aria-label="Remote board ID" value={boardId} onChange={e => setBoardId(e.target.value)} /></label><button onClick={() => void search('board')} aria-label="Find remote board" disabled={!boardId}>Find board</button>
    <label>Work order ID <input aria-label="Remote work order ID" value={orderId} onChange={e => setOrderId(e.target.value)} /></label><button onClick={() => void search('order')} aria-label="Find remote work order" disabled={!orderId}>Find work order</button>
    <p aria-live="polite">{status}</p>
    {board && <div><p>SIMULATED · revision {board.revision} · integrity {board.integrity} {board.reason}</p><p>{board.board.id} / {board.board.workOrderId} · {board.board.status}</p><p>Route: {board.route.join(' → ')}</p><ol>{board.entries.map(e => <li key={e.eventId}><span>{e.sequence} · {e.simTimeMs}ms · {e.machineId}</span><br /><strong>{e.eventType}</strong>{e.passed !== undefined && <> · <b>{e.passed ? 'PASS' : 'FAIL'}</b></>}</li>)}</ol></div>}
    {order && <div><p>SIMULATED · revision {order.revision} · integrity {order.integrity} {order.reason}</p><p>{order.order.id} · {order.order.status}</p><p>Route: {order.route.join(' → ')}</p><p>Planned {order.progress.planned} · Created {order.progress.created} · In progress {order.progress.inProgress} · Completed {order.progress.completed}</p><p>Pass {order.progress.passed} · Fail {order.progress.failed} · Uninspected {order.progress.uninspected}</p><p>Reconciliation: {order.reconciliation}. Failures are not physical scrap.</p></div>}
  </section>
}
