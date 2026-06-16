import {useCallback, useEffect, useMemo, useRef, useState} from 'react'
import {fetchUIPreferences} from './api.js'
import XTerminal from './XTerminal.jsx'
import {DEFAULT_LOG_FONT_SIZE, LOG_LEVELS, countLogsByLevel, detectLogLevel, exportLogText, formatLogLine, normalizeLogFontSize} from './logUtils.js'

export default function LiveRunTerminal({runID, running = false, initialItems = []}) {
  const terminalRef = useRef(null)
  const lastIndexRef = useRef(0)
  const filtersRef = useRef(new Set(LOG_LEVELS))
  const autoScrollRef = useRef(true)
  const seenLogsRef = useRef(new Set())
  const lastLogRef = useRef(null)
  const [logs, setLogs] = useState([])
  const [filters, setFilters] = useState(() => new Set(LOG_LEVELS))
  const [search, setSearch] = useState('')
  const [autoScroll, setAutoScroll] = useState(true)
  const [fontSize, setFontSize] = useState(DEFAULT_LOG_FONT_SIZE)
  const counts = useMemo(() => countLogsByLevel(logs), [logs])

  useEffect(() => {
    let cancelled = false
    async function loadPreferences() {
      try {
        const body = await fetchUIPreferences()
        const nextFontSize = normalizeLogFontSize(body.data?.log_font_size)
        if (!cancelled) setFontSize(nextFontSize)
      } catch {
        if (!cancelled) setFontSize(DEFAULT_LOG_FONT_SIZE)
      }
    }
    loadPreferences()
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    filtersRef.current = filters
  }, [filters])

  useEffect(() => {
    autoScrollRef.current = autoScroll
  }, [autoScroll])

  useEffect(() => {
    lastIndexRef.current = 0
    seenLogsRef.current = new Set()
    lastLogRef.current = null
    setLogs([])
    terminalRef.current?.reset()
  }, [runID])

  const appendEvents = useCallback((events, activeFilters = filtersRef.current) => {
    if (!events.length) return
    const entries = events.map(event => ({...event, text: event.text || '', level: detectLogLevel(event.text || '')}))
    const accepted = []
    for (const entry of entries) {
      const key = logEntryKey(entry)
      if (seenLogsRef.current.has(key)) continue
      const last = lastLogRef.current
      if (last && last.text === entry.text && last.item_id === entry.item_id && last.stream === entry.stream) continue
      seenLogsRef.current.add(key)
      lastLogRef.current = entry
      accepted.push(entry)
    }
    if (!accepted.length) return
    setLogs(previous => [...previous, ...accepted].slice(-50000))
    accepted.forEach(entry => {
      if (activeFilters.has(entry.level)) terminalRef.current?.writeln(formatLogLine(entry.text, entry))
    })
    if (accepted.length && autoScrollRef.current) terminalRef.current?.scrollToBottom()
  }, [])

  const replayTerminal = useCallback((nextFilters = filters) => {
    terminalRef.current?.reset()
    logs.forEach(entry => {
      if (nextFilters.has(entry.level)) terminalRef.current?.writeln(formatLogLine(entry.text, entry))
    })
    terminalRef.current?.scrollToBottom()
  }, [filters, logs])

  useEffect(() => {
    const events = flattenLogItems(initialItems)
    const newEvents = events.slice(lastIndexRef.current)
    lastIndexRef.current = events.length
    appendEvents(newEvents)
  }, [appendEvents, initialItems])

  useEffect(() => {
    if (!runID || !running) return undefined
    const source = new EventSource(`/api/runs/${encodeURIComponent(runID)}/events`)
    source.addEventListener('log', event => {
      try {
        appendEvents([JSON.parse(event.data)])
      } catch {
        // Ignore malformed stream frames; the detail JSON remains available.
      }
    })
    source.addEventListener('complete', () => source.close())
    source.onerror = () => {
      if (!running) source.close()
    }
    return () => source.close()
  }, [appendEvents, runID, running])

  function toggleFilter(level) {
    setFilters(previous => {
      const next = new Set(previous)
      if (next.has(level)) next.delete(level)
      else next.add(level)
      filtersRef.current = next
      replayTerminal(next)
      return next
    })
  }

  function showOnlyErrors() {
    const next = new Set(['fatal', 'error', 'warning'])
    filtersRef.current = next
    setFilters(next)
    replayTerminal(next)
  }

  function toggleAll() {
    setFilters(previous => {
      const next = previous.size === LOG_LEVELS.length ? new Set() : new Set(LOG_LEVELS)
      filtersRef.current = next
      replayTerminal(next)
      return next
    })
  }

  function exportLogs() {
    const text = exportLogText(logs, filters)
    if (!text) return
    const blob = new Blob([text], {type: 'text/plain;charset=utf-8'})
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `run-${runID || 'logs'}-${new Date().toISOString().slice(0, 19).replace(/[:-]/g, '')}.log`
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    URL.revokeObjectURL(url)
  }

  function findNext() {
    if (search) terminalRef.current?.findNext(search, {caseSensitive: false})
  }

  function findPrevious() {
    if (search) terminalRef.current?.findPrevious(search, {caseSensitive: false})
  }

  return (
    <div className="liveTerminal">
      <div className="liveTerminalToolbar">
        <input value={search} placeholder="搜索日志" onChange={event => setSearch(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') findNext() }} />
        <button type="button" className="secondary" onClick={findPrevious}>上一个</button>
        <button type="button" className="secondary" onClick={findNext}>下一个</button>
        <button type="button" className="secondary" onClick={showOnlyErrors}>只看错误</button>
        <label className="terminalToggle">
          <input type="checkbox" checked={autoScroll} onChange={event => setAutoScroll(event.target.checked)} />
          <span>自动滚动</span>
        </label>
        <button type="button" className="secondary" onClick={exportLogs}>导出</button>
      </div>
      <div className="logFilterTags">
        <button type="button" className={filters.size === LOG_LEVELS.length ? 'tagChip active' : 'tagChip'} onClick={toggleAll}>
          {filters.size === LOG_LEVELS.length ? '全部' : '恢复全部'}
        </button>
        {LOG_LEVELS.map(level => (
          <button key={level} type="button" className={filters.has(level) ? 'tagChip active' : 'tagChip'} onClick={() => toggleFilter(level)}>
            {level} {counts[level] || 0}
          </button>
        ))}
      </div>
      <XTerminal key={fontSize} ref={terminalRef} className="runXTerminal" options={{fontSize}} onReady={() => replayTerminal(filters)} />
    </div>
  )
}

function flattenLogItems(items = []) {
  const events = []
  const visit = item => {
    if (!item) return
    const base = {
      item_id: item.id,
      kind: item.kind,
      step_id: item.kind === 'loop_iteration' ? String(item.id || '').split('#')[0] : item.id,
      iteration: item.iteration || 0,
      status: item.status
    }
    String(item.stdout || '').split(/\r?\n/).filter(Boolean).forEach(text => events.push({...base, stream: 'stdout', text}))
    String(item.stderr || '').split(/\r?\n/).filter(Boolean).forEach(text => events.push({...base, stream: 'stderr', text}))
    ;(item.children || []).forEach(visit)
  }
  items.forEach(visit)
  return events
}

function logEntryKey(entry) {
  return [
    entry.run_id || '',
    entry.item_id || '',
    entry.stream || '',
    entry.iteration || 0,
    entry.text || ''
  ].join('|')
}
