import React, {useEffect, useMemo, useRef, useState} from 'react'
import {LOG_LEVELS, countLogsByLevel} from '../../logUtils.js'
import {downloadRunLogText, flattenRunLogLines} from './runLog.js'

export const LOG_LINE_LIMIT = 4000
const ERROR_LEVELS = ['fatal', 'error', 'warning']

// 统一日志列表：右栏日志页、运行记录页、工作流抽屉都用这一个组件。
// nodeID 为空看整份运行日志（按节点分组），非空看该节点（含循环迭代）；级别过滤与终端页共用一套判定。
// focusOwnerID 是“用户点的那一段”：滚动和整组高亮都跟着它走，没点则退回正在执行的节点。
export default function RunLogView({
  items = [],
  nodeID = '',
  nodeNames = {},
  runningOwnerID = '',
  focusOwnerID = '',
  runID = 'run',
  scopeToggle = true,
  lineLimit = LOG_LINE_LIMIT
}) {
  const [onlyNode, setOnlyNode] = useState(true)
  const [search, setSearch] = useState('')
  const [autoScroll, setAutoScroll] = useState(true)
  const [wrapLines, setWrapLines] = useState(true)
  const [levels, setLevels] = useState(() => new Set(LOG_LEVELS))
  // 命中的第几条：搜索之后可以上下跳，长日志不用自己找。
  const [hitCursor, setHitCursor] = useState(0)
  const [atBottom, setAtBottom] = useState(true)
  const logBoxRef = useRef(null)
  const ownerRefs = useRef({})
  const previousFocusRef = useRef('')
  const previousContentRef = useRef('')
  const focusedOwnerID = focusOwnerID || runningOwnerID

  const scoped = Boolean(nodeID)
  useEffect(() => {
    if (scoped) setOnlyNode(true)
  }, [scoped, nodeID])

  const showOnlyNode = scopeToggle && scoped && onlyNode
  const lines = useMemo(
    () => flattenRunLogLines(items, {nodeID: showOnlyNode ? nodeID : '', limit: Infinity}),
    [items, showOnlyNode, nodeID]
  )
  const levelCounts = useMemo(() => countLogsByLevel(lines), [lines])
  const activeLevels = useMemo(() => LOG_LEVELS.filter(level => Number(levelCounts[level]) > 0), [levelCounts])
  const keyword = search.trim()
  const matchingLines = useMemo(() => {
    const needle = keyword.toLowerCase()
    return lines.filter(line => levels.has(line.level) && (!needle || line.text.toLowerCase().includes(needle)))
  }, [lines, levels, keyword])
  const visibleLines = useMemo(() => matchingLines.slice(-lineLimit), [matchingLines, lineLimit])
  const allLevelsOn = activeLevels.length > 0 && activeLevels.every(level => levels.has(level))
  const truncated = matchingLines.length > visibleLines.length
  const hitCount = keyword ? visibleLines.length : 0
  const activeHit = hitCount > 0 ? Math.min(hitCursor, hitCount - 1) : 0
  // 搜索高亮只做在可见行上：被截断的旧行本来就不渲染，不必为它们建索引。
  const hitOrder = useMemo(() => {
    if (!keyword) return null
    const map = new Map()
    visibleLines.forEach((line, index) => map.set(`${line.key}:${line.index}`, index))
    return map
  }, [keyword, visibleLines])

  useEffect(() => {
    setHitCursor(0)
  }, [keyword, levels])

  function toggleLevel(level) {
    setLevels(current => {
      const next = new Set(current)
      if (next.has(level)) next.delete(level)
      else next.add(level)
      return next
    })
  }

  // 只滚日志容器，不让 scrollIntoView 把整个工作台拉离用户当前的位置。
  useEffect(() => {
    const box = logBoxRef.current
    if (!box) return
    const focusChanged = previousFocusRef.current !== focusedOwnerID
    previousFocusRef.current = focusedOwnerID
    const last = visibleLines[visibleLines.length - 1]
    const content = `${matchingLines.length}:${last?.key || ''}:${last?.text || ''}`
    const contentChanged = previousContentRef.current !== content
    previousContentRef.current = content
    const target = ownerRefs.current[focusedOwnerID]
    if (focusChanged && target && !showOnlyNode) {
      box.scrollTop += target.getBoundingClientRect().top - box.getBoundingClientRect().top
    } else if (autoScroll && !keyword && contentChanged && (!focusOwnerID || showOnlyNode)) {
      box.scrollTop = box.scrollHeight
    }
  }, [focusedOwnerID, focusOwnerID, showOnlyNode, autoScroll, keyword, visibleLines, matchingLines.length])

  // 搜索时把视图带到当前命中的那一行（声明在上面的自动滚动之后，保证这一下不被覆盖）。
  useEffect(() => {
    if (!keyword) return
    scrollToHit(activeHit)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [keyword, activeHit])

  const groups = useMemo(() => {
    const out = []
    visibleLines.forEach(line => {
      const last = out[out.length - 1]
      if (!last || last.ownerID !== line.ownerID) out.push({ownerID: line.ownerID, lines: [line]})
      else last.lines.push(line)
    })
    return out
  }, [visibleLines])

  function scrollToHit(index) {
    const box = logBoxRef.current
    if (!box) return
    const rows = box.querySelectorAll('.runLogLine')
    const target = rows[index]
    if (!target) return
    const offset = target.getBoundingClientRect().top - box.getBoundingClientRect().top
    box.scrollTop += offset - 8
  }

  function jumpToHit(delta) {
    if (!keyword || hitCount === 0) return
    const next = ((activeHit + delta) % hitCount + hitCount) % hitCount
    setHitCursor(next)
    scrollToHit(next)
  }

  function jumpToLatest() {
    const box = logBoxRef.current
    if (!box) return
    box.scrollTop = box.scrollHeight
    setAtBottom(true)
  }

  function handleLogScroll() {
    const box = logBoxRef.current
    if (!box) return
    const distance = box.scrollHeight - box.scrollTop - box.clientHeight
    const next = distance <= 24
    setAtBottom(current => (current === next ? current : next))
  }

  function handleSearchKeyDown(event) {
    if (event.key !== 'Enter') return
    event.preventDefault()
    jumpToHit(event.shiftKey ? -1 : 1)
  }

  function exportLogs() {
    downloadRunLogText(matchingLines.map(line => line.text), runID, showOnlyNode ? nodeID : '')
  }

  const emptyText = search || (lines.length > 0 && matchingLines.length === 0)
    ? '没有匹配的日志行。'
    : nodeID
      ? '当前节点暂无日志，节点开始执行后会显示。'
      : '这次运行还没有产生日志。'

  return (
    <div className="runLogView">
      <div className="runLogToolbar">
        {scopeToggle && scoped && (
          <label className="terminalToggle">
            <input type="checkbox" checked={onlyNode} onChange={event => setOnlyNode(event.target.checked)} />
            <span>只看此节点</span>
          </label>
        )}
        <input
          type="search"
          aria-label="搜索日志"
          value={search}
          placeholder="搜索全部日志（Enter 跳到下一处）"
          onChange={event => setSearch(event.target.value)}
          onKeyDown={handleSearchKeyDown}
        />
        {keyword && (
          <button type="button" className="secondary small" onClick={() => setSearch('')} title="清除搜索">清除</button>
        )}
        {keyword && (
          <span className="runLogHitNav" role="group" aria-label="搜索命中跳转">
            <button type="button" className="secondary small" disabled={hitCount === 0} onClick={() => jumpToHit(-1)} aria-label="上一个匹配">↑</button>
            <span className="runLogHitPosition">{hitCount === 0 ? '无匹配' : `${activeHit + 1} / ${hitCount}`}</span>
            <button type="button" className="secondary small" disabled={hitCount === 0} onClick={() => jumpToHit(1)} aria-label="下一个匹配">↓</button>
          </span>
        )}
        <label className="terminalToggle">
          <input type="checkbox" checked={autoScroll} onChange={event => setAutoScroll(event.target.checked)} />
          <span>自动滚动</span>
        </label>
        <label className="terminalToggle">
          <input type="checkbox" checked={wrapLines} onChange={event => setWrapLines(event.target.checked)} />
          <span>自动换行</span>
        </label>
        <button type="button" className="secondary small" onClick={() => setLevels(new Set(ERROR_LEVELS))}>只看错误</button>
        <button type="button" className="secondary small" disabled={matchingLines.length === 0} title="导出全部匹配日志，不受显示行数限制" onClick={exportLogs}>导出</button>
        <span className="runLogCount">{matchingLines.length !== lines.length ? `${matchingLines.length} / ${lines.length}` : `${lines.length} 行`}</span>
      </div>
      <div className="logFilterTags" role="group" aria-label="日志级别过滤">
        <button type="button" className={allLevelsOn ? 'tagChip active' : 'tagChip'} onClick={() => setLevels(allLevelsOn ? new Set() : new Set(LOG_LEVELS))}>
          {allLevelsOn ? '全部' : '恢复全部'}
        </button>
        {activeLevels.map(level => (
          <button key={level} type="button" className={levels.has(level) ? 'tagChip active' : 'tagChip'} onClick={() => toggleLevel(level)}>
            {level} {levelCounts[level] || 0}
          </button>
        ))}
      </div>
      <div className="runLogBoxWrap">
        <div
          className="runLogBox"
          ref={logBoxRef}
          data-only-node={showOnlyNode ? '1' : '0'}
          data-wrap={wrapLines ? 'on' : 'off'}
          onScroll={handleLogScroll}
        >
          {truncated && <div className="runLogTruncated">显示最近 {lineLimit} 条匹配日志，共 {matchingLines.length} 条。搜索、级别筛选和导出覆盖全部日志。</div>}
          {groups.map((group, index) => (
            <React.Fragment key={`${group.ownerID}:${index}`}>
              {!showOnlyNode && (
                <div
                  className={`runLogOwner${group.ownerID === focusedOwnerID ? ' focus' : ''}${group.ownerID === runningOwnerID ? ' running' : ''}`}
                  data-owner-id={group.ownerID}
                  ref={element => {
                    if (element) ownerRefs.current[group.ownerID] = element
                    else delete ownerRefs.current[group.ownerID]
                  }}
                >
                  <span className="runLogOwnerName">{nodeNames[group.ownerID] || group.ownerID}</span>
                  <span className="runLogOwnerMeta">{group.lines.length} 行</span>
                </div>
              )}
              {group.lines.map(line => {
                const lineIndex = hitOrder ? hitOrder.get(`${line.key}:${line.index}`) : -1
                const currentHit = keyword && lineIndex === activeHit
                return (
                  <div
                    key={`${line.key}:${line.index}`}
                    className={`runLogLine ${line.stream} level-${line.level}${line.ownerID === focusedOwnerID ? ' focus' : ''}${line.ownerID === runningOwnerID ? ' running' : ''}${currentHit ? ' current' : ''}`}
                    data-step-id={line.stepID}
                    data-level={line.level}
                  >
                    {highlightLogText(line.text, keyword)}
                  </div>
                )
              })}
            </React.Fragment>
          ))}
          {visibleLines.length === 0 && <div className="empty small">{emptyText}</div>}
        </div>
        {!atBottom && (
          <button type="button" className="runLogJumpLatest" onClick={jumpToLatest}>跳到最新 ↓</button>
        )}
      </div>
    </div>
  )
}

// 搜索命中就地高亮：只拆文本节点，不改行内容，复制/断言都还是原文。
export function highlightLogText(text, keyword) {
  const value = String(text ?? '')
  const needle = String(keyword || '').trim()
  if (!needle) return value
  const lowerValue = value.toLowerCase()
  const lowerNeedle = needle.toLowerCase()
  const parts = []
  let cursor = 0
  let found = lowerValue.indexOf(lowerNeedle)
  let key = 0
  while (found !== -1) {
    if (found > cursor) parts.push(value.slice(cursor, found))
    parts.push(<mark className="runLogHit" key={`hit-${key++}`}>{value.slice(found, found + needle.length)}</mark>)
    cursor = found + needle.length
    found = lowerValue.indexOf(lowerNeedle, cursor)
  }
  if (parts.length === 0) return value
  if (cursor < value.length) parts.push(value.slice(cursor))
  return parts
}
