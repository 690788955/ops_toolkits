import {useCallback, useEffect, useMemo, useRef, useState} from 'react'
import {fetchUIPreferences} from './api.js'
import XTerminal from './XTerminal.jsx'
import {DEFAULT_LOG_FONT_SIZE, LOG_LEVELS, detectLogLevel, exportLogText, formatLogLine, normalizeLogFontSize} from './logUtils.js'

// 终端缓冲与日志列表保持一致；超出后靠 xterm 自身的 scrollback 顶掉最早的行。
const LOG_BUFFER_LIMIT = 50000
const TERMINAL_BATCH_LINES = 400
const TERMINAL_FLUSH_DELAY = 16

export default function LiveRunTerminal({runID, running = false, initialItems = []}) {
  const terminalRef = useRef(null)
  const filtersRef = useRef(new Set(LOG_LEVELS))
  const autoScrollRef = useRef(true)
  const seenLogsRef = useRef(new Set())
  const pendingTerminalRef = useRef({lines: [], timer: 0})
  const replayOnFiltersRef = useRef(false)
  const [logs, setLogs] = useState([])
  const [filters, setFilters] = useState(() => new Set(LOG_LEVELS))
  const [search, setSearch] = useState('')
  const [autoScroll, setAutoScroll] = useState(true)
  const [fontSize, setFontSize] = useState(DEFAULT_LOG_FONT_SIZE)
  // 终端要等挂载后才渲染，首屏日志在 onReady 里统一回放，避免写进尚未创建的终端被丢掉。
  const bootedRef = useRef(false)
  const initialItemsRef = useRef(initialItems)
  const runningRef = useRef(running)
  const runIDRef = useRef(runID)
  initialItemsRef.current = initialItems
  runningRef.current = running
  runIDRef.current = runID
  const counts = useMemo(() => {
    const next = Object.fromEntries(LOG_LEVELS.map(level => [level, 0]))
    logs.forEach(entry => { next[entry.level] = (next[entry.level] || 0) + 1 })
    return next
  }, [logs])

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

  const flushTerminal = useCallback(() => {
    const pending = pendingTerminalRef.current
    if (pending.timer) {
      window.clearTimeout(pending.timer)
      pending.timer = 0
    }
    if (!pending.lines.length) return
    const text = pending.lines.join('\r\n')
    const scroll = autoScrollRef.current
    pending.lines = []
    terminalRef.current?.write(text, () => {
      if (scroll) terminalRef.current?.scrollToBottom()
    })
  }, [])

  // 流式日志逐行写会让长任务越来越卡：积攒到批次大小或下一帧再写。
  // 打开运行快照时首屏会一次性灌入整份历史，这种批量直接整块写入。
  const queueTerminalLines = useCallback(lines => {
    if (!lines.length) return
    const pending = pendingTerminalRef.current
    if (lines.length >= TERMINAL_BATCH_LINES || pending.lines.length + lines.length >= TERMINAL_BATCH_LINES) {
      pending.lines.push(...lines)
      flushTerminal()
      return
    }
    pending.lines.push(...lines)
    if (!pending.timer) pending.timer = window.setTimeout(flushTerminal, TERMINAL_FLUSH_DELAY)
  }, [flushTerminal])

  useEffect(() => () => {
    const pending = pendingTerminalRef.current
    if (pending.timer) window.clearTimeout(pending.timer)
    pending.timer = 0
    pending.lines = []
  }, [])

  useEffect(() => {
    seenLogsRef.current = new Set()
    setLogs([])
    flushTerminal()
    terminalRef.current?.reset()
  }, [runID, flushTerminal])

  const appendEvents = useCallback((events, activeFilters = filtersRef.current) => {
    if (!events.length) return
    const entries = events.map(event => ({...event, text: event.text || '', level: detectLogLevel(event.text || '')}))
    const accepted = []
    for (const entry of entries) {
      const key = logEntryKey(entry)
      if (seenLogsRef.current.has(key)) continue
      seenLogsRef.current.add(key)
      accepted.push(entry)
    }
    if (!accepted.length) return
    setLogs(previous => [...previous, ...accepted].slice(-LOG_BUFFER_LIMIT))
    queueTerminalLines(
      accepted
        .filter(entry => activeFilters.has(entry.level))
        .map(entry => formatLogLine(entry.text, entry)),
    )
  }, [queueTerminalLines])

  const replayTerminal = useCallback((nextFilters = filters) => {
    if (pendingTerminalRef.current.timer) {
      window.clearTimeout(pendingTerminalRef.current.timer)
      pendingTerminalRef.current.timer = 0
    }
    pendingTerminalRef.current.lines = []
    terminalRef.current?.reset()
    const text = logs
      .filter(entry => nextFilters.has(entry.level))
      .map(entry => formatLogLine(entry.text, entry))
      .join('\r\n')
    if (!text) return
    terminalRef.current?.write(text, () => terminalRef.current?.scrollToBottom())
  }, [logs])

  useEffect(() => {
    if (!bootedRef.current) return
    appendEvents(flattenLogItems(initialItems, runID, running))
  }, [appendEvents, initialItems, runID, running])

  // 首屏快照和切换运行都重建终端 key，新终端就绪时只需重放当前日志，去重交给 seenLogsRef。
  const handleTerminalReady = useCallback(() => {
    bootedRef.current = true
    appendEvents(flattenLogItems(initialItemsRef.current, runIDRef.current, runningRef.current))
    replayTerminal(filtersRef.current)
  }, [appendEvents, replayTerminal])

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

  // 筛选变化后由 effect 统一重放：日志会在同一帧里继续追加，重放必须用最新快照。
  function toggleFilter(level) {
    setFilters(previous => {
      const next = new Set(previous)
      if (next.has(level)) next.delete(level)
      else next.add(level)
      filtersRef.current = next
      return next
    })
    replayOnFiltersRef.current = true
  }

  function showOnlyErrors() {
    const next = new Set(['fatal', 'error', 'warning'])
    filtersRef.current = next
    setFilters(next)
    replayOnFiltersRef.current = true
  }

  function toggleAll() {
    setFilters(previous => {
      const next = previous.size === LOG_LEVELS.length ? new Set() : new Set(LOG_LEVELS)
      filtersRef.current = next
      return next
    })
    replayOnFiltersRef.current = true
  }

  useEffect(() => {
    if (!replayOnFiltersRef.current) return
    replayOnFiltersRef.current = false
    replayTerminal(filters)
  }, [filters, replayTerminal])

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
            {level} {logs.length >= LOG_BUFFER_LIMIT ? `${counts[level] || 0}+` : counts[level] || 0}
          </button>
        ))}
      </div>
      <XTerminal key={`${runID}:${fontSize}`} ref={terminalRef} className="runXTerminal" options={{fontSize}} onReady={handleTerminalReady} />
    </div>
  )
}

function flattenLogItems(items, runID, running) {
  const events = []
  const visit = item => {
    if (!item) return
    const base = {
      run_id: runID,
      item_id: item.id,
      kind: item.kind,
      step_id: item.kind === 'loop_iteration' ? String(item.id || '').split('#')[0] : item.id,
      iteration: item.iteration || 0,
      status: item.status
    }
    for (const stream of ['stdout', 'stderr']) {
      const lines = String(item[stream] || '').split(/\r\n|\r|\n/)
      if (running) lines.pop()
      let line = 0
      for (const text of lines) {
        if (text) events.push({...base, stream, line: ++line, text})
      }
    }
    ;(item.children || []).forEach(visit)
  }
  items.forEach(visit)
  return events
}

function logEntryKey(entry) {
  return JSON.stringify([entry.run_id, entry.item_id, entry.stream, entry.iteration || 0, entry.line])
}
