import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react'
import {fetchJSON} from '../../api.js'
import {readableAPIError} from '../../utils.js'
import {normalizeRunStatus, runStatusLabel} from '../nodes.jsx'
import {runDurationText} from './runLog.js'

const RAIL_PAGE_SIZE = 20
const RAIL_POLL_MS = 3000

// 工作流工作台左栏：只列**当前工作流**的运行（kind=workflow + target 精确过滤）。
// 别的运行/工具运行都在全局“运行记录”页，这里只给一个跳转入口，不复制那套列表。
export default function RunHistoryRail({workflowID = '', activeRunID = '', activeRunStatus = '', selectionLocked = false, refreshToken = 0, onSelectRun, onOpenAllRuns}) {
  const [items, setItems] = useState([])
  const [total, setTotal] = useState(0)
  const [loadedWorkflowID, setLoadedWorkflowID] = useState('')
  const [loading, setLoading] = useState(false)
  const [message, setMessage] = useState('')
  const requestRef = useRef(0)

  const load = useCallback(async ({silent = false} = {}) => {
    const requestID = ++requestRef.current
    if (!workflowID) {
      setItems([])
      setTotal(0)
      setLoadedWorkflowID('')
      setLoading(false)
      setMessage('当前画布还没有对应的已保存工作流，运行记录请到“完整运行记录”里看。')
      return
    }
    if (!silent) setLoading(true)
    try {
      const params = new URLSearchParams()
      params.set('kind', 'workflow')
      params.set('target', workflowID)
      params.set('limit', String(RAIL_PAGE_SIZE))
      const body = await fetchJSON(`/api/runs/?${params.toString()}`)
      if (requestID !== requestRef.current) return
      const runs = Array.isArray(body?.data?.runs) ? body.data.runs : []
      setItems(runs)
      setTotal(Number(body?.data?.total ?? runs.length) || 0)
      setLoadedWorkflowID(workflowID)
      setMessage('')
    } catch (err) {
      if (requestID !== requestRef.current) return
      if (!silent) setMessage(readableAPIError(err, '加载运行记录失败。'))
    } finally {
      if (requestID === requestRef.current) setLoading(false)
    }
  }, [workflowID])

  useEffect(() => {
    setMessage('')
    load()
    return () => { requestRef.current += 1 }
  }, [load, refreshToken])

  const visibleItems = loadedWorkflowID === workflowID ? items : []
  const visibleTotal = loadedWorkflowID === workflowID ? total : 0
  const hasRunning = useMemo(() => loadedWorkflowID === workflowID && items.some(item => normalizeRunStatus(item.status) === 'running'), [items, loadedWorkflowID, workflowID])
  const activeRunRunning = normalizeRunStatus(activeRunStatus) === 'running'
  useEffect(() => {
    if (!hasRunning && !activeRunRunning) return undefined
    const timer = window.setInterval(() => { load({silent: true}) }, RAIL_POLL_MS)
    return () => window.clearInterval(timer)
  }, [hasRunning, activeRunRunning, load])

  return (
    <aside className="workflowRunRail" aria-label="运行记录">
      <header className="workflowRunRailHeader">
        <div>
          <strong>运行记录</strong>
          <span title={workflowID || undefined}>{workflowID || '未保存工作流'}</span>
        </div>
        <button type="button" className="secondary small" onClick={() => load()} disabled={loading} title="刷新运行记录">{loading ? '刷新中' : '刷新'}</button>
      </header>
      <div className="workflowRunRailList">
        {visibleItems.map(item => {
          const status = normalizeRunStatus(item.status)
          const active = item.id === activeRunID
          return (
            <button
              key={item.id}
              type="button"
              className={`workflowRunRailItem ${status}${active ? ' active' : ''}`}
              onClick={() => onSelectRun?.(item.id, item)}
              disabled={selectionLocked && !active}
              title={selectionLocked && !active ? '当前运行完成或取消后可切换记录' : `${item.id} · ${runStatusLabel(status)}`}
            >
              <span className={`workflowRunStatusDot ${status}`} aria-hidden="true" />
              <span className="workflowRunRailItemMain">
                <strong>{item.target || item.id}</strong>
                <small>{formatRailTime(item.started_at)}{item.ended_at ? ` · ${runDurationText(item.started_at, item.ended_at)}` : ''}</small>
              </span>
              <em className={`runStatusText ${status}`}>{runStatusLabel(status)}</em>
            </button>
          )
        })}
        {visibleItems.length === 0 && !loading && !message && (
          <div className="empty small">这个工作流还没有运行记录。点“运行”开始第一次执行。</div>
        )}
        {message && <div className="workflowRunRailMessage" role="status">{message}</div>}
        {selectionLocked && <div className="workflowRunRailMessage">当前运行完成或取消后可切换记录。</div>}
      </div>
      <footer className="workflowRunRailFooter">
        <span>{visibleTotal > visibleItems.length ? `显示 ${visibleItems.length} / ${visibleTotal} 条` : `共 ${visibleTotal} 条`}</span>
        <button type="button" className="secondary small" onClick={() => onOpenAllRuns?.()}>完整运行记录</button>
      </footer>
    </aside>
  )
}

function formatRailTime(value) {
  const date = new Date(value || '')
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString(undefined, {month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'})
}
