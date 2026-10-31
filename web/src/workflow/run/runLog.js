// 运行模式与运行日志的纯函数集合。
// 画布、右栏节点详情、运行记录左栏共用同一套判定，避免三处各写一份导致状态漂移。
import {detectLogLevel} from '../../logUtils.js'
import {normalizeRunStatus} from '../nodes.jsx'

export const CANVAS_MODE_EDIT = 'edit'
export const CANVAS_MODE_RUN = 'run'

// 循环节点的迭代步骤 ID 形如 loop-1#2-target，去掉 # 之后的部分才能归回原节点。
export function baseStepID(id) {
  const value = String(id || '')
  const hash = value.indexOf('#')
  return hash > 0 ? value.slice(0, hash) : value
}

// 编辑模式 / 运行模式的锁：开始运行强制进运行模式，之后一直停在运行模式，
// 由用户点「编辑模式」显式解锁。运行结果不会因为运行结束就自动消失。
export function nextCanvasMode({current, running}) {
  if (running) return CANVAS_MODE_RUN
  return current === CANVAS_MODE_RUN ? CANVAS_MODE_RUN : CANVAS_MODE_EDIT
}

export function findRunLogItem(items, nodeID) {
  for (const item of items || []) {
    if (item?.id === nodeID) return item
    const child = findRunLogItem(item?.children || [], nodeID)
    if (child) return child
  }
  return null
}

// 一个节点可能对应多条日志项：循环节点每个迭代都是 loopID#n-target。
export function collectRunLogItems(items, nodeID) {
  const out = []
  const visit = item => {
    if (!item) return
    if (baseStepID(item.id) === nodeID) {
      out.push(item)
      return
    }
    ;(item.children || []).forEach(visit)
  }
  ;(items || []).forEach(visit)
  return out
}

// 把日志树压平成带归属节点的行：nodeID 为空时给整份运行日志，否则只给该节点（含循环迭代）。
export function flattenRunLogLines(items, {nodeID = '', limit = 5000} = {}) {
  const lines = []
  let cursor = 0
  const visit = (item, owner) => {
    if (!item || lines.length >= limit) return
    const own = owner || baseStepID(item.id)
    if (!nodeID || own === nodeID) {
      for (const stream of ['stdout', 'stderr']) {
        const text = String(item[stream] || '')
        if (!text) continue
        for (const raw of text.split(/\r\n|\r|\n/)) {
          if (!raw) continue
          // 上限按行算：只在进入节点时判断会让最后一个节点的输出整块超出去。
          if (lines.length >= limit) break
          lines.push({
            key: `${item.id}:${stream}:${item.line || cursor}`,
            index: cursor++,
            stepID: item.id,
            ownerID: own,
            stream,
            text: raw,
            status: item.status || '',
            // 级别判定与运行记录页的日志终端共用一套，筛选标签才对得上。
            level: detectLogLevel(raw)
          })
        }
      }
    }
    ;(item.children || []).forEach(child => visit(child, own))
  }
  ;(items || []).forEach(item => visit(item, baseStepID(item.id)))
  return lines
}

// 当前正在执行的节点：日志要聚焦到它的第一行。
export function runningRunNodeID(runState) {
  const nodes = runState?.nodes || {}
  for (const id of Object.keys(nodes)) {
    if (normalizeRunStatus(nodes[id]?.status) === 'running') return id
  }
  return ''
}

export function firstLineIndexOfNode(lines, nodeID) {
  if (!nodeID) return -1
  return (lines || []).findIndex(line => line.ownerID === nodeID || line.stepID === nodeID)
}

export function runDurationText(startedAt, endedAt) {
  const start = Date.parse(startedAt || '')
  const end = Date.parse(endedAt || '')
  if (Number.isNaN(start)) return ''
  const ms = (Number.isNaN(end) ? Date.now() : end) - start
  if (ms < 0) return ''
  if (ms < 1000) return `${ms}ms`
  const seconds = ms / 1000
  if (seconds < 60) return `${seconds.toFixed(1)}s`
  const wholeSeconds = Math.floor(seconds)
  const minutes = Math.floor(wholeSeconds / 60)
  const rest = wholeSeconds % 60
  if (minutes < 60) return `${minutes}m${String(rest).padStart(2, '0')}s`
  return `${Math.floor(minutes / 60)}h${String(minutes % 60).padStart(2, '0')}m`
}

export function stepDurationText(step) {
  return runDurationText(step?.started_at, step?.ended_at)
}

export function runCountsByStatus(runState) {
  const counts = {succeeded: 0, failed: 0, running: 0, skipped: 0, waiting: 0}
  Object.values(runState?.nodes || {}).forEach(node => {
    const status = normalizeRunStatus(node?.status)
    if (counts[status] === undefined) counts[status] = 0
    counts[status] += 1
  })
  return counts
}

// 导出当前视图里看到的日志行（已经过节点范围、级别和搜索过滤），文件名带上范围便于区分。
export function downloadRunLogText(lines, runID = 'run', nodeID = '') {
  const text = (lines || []).join('\n')
  if (!text || typeof document === 'undefined' || typeof URL === 'undefined' || typeof URL.createObjectURL !== 'function') return false
  const blob = new Blob([text], {type: 'text/plain;charset=utf-8'})
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  const stamp = new Date().toISOString().slice(0, 19).replace(/[:-]/g, '')
  link.href = url
  link.download = `run-${runID || 'run'}${nodeID ? `-${nodeID}` : ''}-${stamp}.log`
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
  return true
}
