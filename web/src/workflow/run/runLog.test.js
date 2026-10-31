// @vitest-environment jsdom
import {describe, expect, it, vi} from 'vitest'
import {
  baseStepID,
  CANVAS_MODE_EDIT,
  CANVAS_MODE_RUN,
  collectRunLogItems,
  downloadRunLogText,
  findRunLogItem,
  firstLineIndexOfNode,
  flattenRunLogLines,
  nextCanvasMode,
  runCountsByStatus,
  runDurationText,
  runningRunNodeID
} from './runLog.js'

const logItems = [
  {id: 'first', status: 'succeeded', stdout: 'first-output\nline-2'},
  {
    id: 'loop-1',
    status: 'running',
    stdio: '',
    children: [
      {id: 'loop-1#1-do', status: 'succeeded', stdout: 'iteration-1'},
      {id: 'loop-1#2-do', status: 'running', stdout: 'iteration-2', stderr: 'boom'}
    ]
  }
]

describe('canvas run mode lock', () => {
  it('开始运行时锁进运行模式', () => {
    expect(nextCanvasMode({current: CANVAS_MODE_EDIT, running: true})).toBe(CANVAS_MODE_RUN)
  })

  it('运行结束后停在运行模式，等用户显式切回编辑', () => {
    expect(nextCanvasMode({current: CANVAS_MODE_RUN, running: false})).toBe(CANVAS_MODE_RUN)
  })

  it('空闲时保持用户手动选择的模式', () => {
    expect(nextCanvasMode({current: CANVAS_MODE_RUN, running: false})).toBe(CANVAS_MODE_RUN)
    expect(nextCanvasMode({current: CANVAS_MODE_EDIT, running: false})).toBe(CANVAS_MODE_EDIT)
  })
})

describe('run log scoping', () => {
  it('循环迭代步骤归回原节点', () => {
    expect(baseStepID('loop-1#2-do')).toBe('loop-1')
    expect(baseStepID('plain')).toBe('plain')
  })

  it('findRunLogItem 只认精确 ID', () => {
    expect(findRunLogItem(logItems, 'loop-1#2-do')?.stdout).toBe('iteration-2')
    expect(findRunLogItem(logItems, 'loop-1')?.id).toBe('loop-1')
    expect(findRunLogItem(logItems, 'missing')).toBeNull()
  })

  it('collectRunLogItems 收全循环迭代', () => {
    expect(collectRunLogItems(logItems, 'loop-1').map(item => item.id)).toEqual(['loop-1'])
    expect(collectRunLogItems([{id: 'loop-1#1-do', stdout: 'a'}, {id: 'loop-1#2-do', stdout: 'b'}], 'loop-1').map(item => item.id)).toEqual(['loop-1#1-do', 'loop-1#2-do'])
  })

  it('只看此节点时只压平该节点的行', () => {
    const lines = flattenRunLogLines(logItems, {nodeID: 'loop-1'})
    expect(lines.map(line => line.text)).toEqual(['iteration-1', 'iteration-2', 'boom'])
    expect(lines.every(line => line.ownerID === 'loop-1')).toBe(true)
    expect(lines.find(line => line.text === 'boom').stream).toBe('stderr')
  })

  it('全局流保留所有节点并标出归属', () => {
    const lines = flattenRunLogLines(logItems)
    expect(lines.map(line => line.text)).toEqual(['first-output', 'line-2', 'iteration-1', 'iteration-2', 'boom'])
    expect(firstLineIndexOfNode(lines, 'loop-1')).toBe(2)
    expect(firstLineIndexOfNode(lines, 'first')).toBe(0)
    expect(firstLineIndexOfNode(lines, 'missing')).toBe(-1)
  })

  it('空行不产出日志行', () => {
    expect(flattenRunLogLines([{id: 'a', stdout: 'x\n\n\ny'}]).map(line => line.text)).toEqual(['x', 'y'])
  })

  it('行数上限按行严格生效', () => {
    const lines = flattenRunLogLines([
      {id: 'a', stdout: 'a1\na2'},
      {id: 'b', stdout: 'b1\nb2'}
    ], {limit: 3})
    expect(lines.map(line => line.text)).toEqual(['a1', 'a2', 'b1'])
  })

  it('每行带上与运行记录页一致的日志级别', () => {
    const lines = flattenRunLogLines([{id: 'a', stdout: 'ERROR 连接失败\n普通输出', stderr: 'warning 重试中'}])
    expect(lines.map(line => line.level)).toEqual(['error', 'log', 'warning'])
  })

  it('导出会生成下载链接并释放 blob', () => {
    const originalCreate = URL.createObjectURL
    const originalRevoke = URL.revokeObjectURL
    const create = vi.fn(() => 'blob:fake')
    const revoke = vi.fn()
    URL.createObjectURL = create
    URL.revokeObjectURL = revoke
    try {
      expect(downloadRunLogText(['a', 'b'], 'run-9', 'node-1')).toBe(true)
      expect(create).toHaveBeenCalledTimes(1)
      expect(revoke).toHaveBeenCalledWith('blob:fake')
    } finally {
      URL.createObjectURL = originalCreate
      URL.revokeObjectURL = originalRevoke
    }
  })

  it('导出在当前环境不可用时返回 false，不抛错', () => {
    const originalCreate = URL.createObjectURL
    URL.createObjectURL = undefined
    try {
      expect(downloadRunLogText(['line'], 'run-1')).toBe(false)
    } finally {
      URL.createObjectURL = originalCreate
    }
    expect(downloadRunLogText([], 'run-1')).toBe(false)
  })
})

describe('run status helpers', () => {
  it('runningRunNodeID 找到正在执行的节点', () => {
    expect(runningRunNodeID({nodes: {a: {status: 'succeeded'}, b: {status: 'running'}}})).toBe('b')
    expect(runningRunNodeID({nodes: {a: {status: 'succeeded'}}})).toBe('')
    expect(runningRunNodeID(null)).toBe('')
  })

  it('runCountsByStatus 汇总节点状态', () => {
    expect(runCountsByStatus({nodes: {a: {status: 'succeeded'}, b: {status: 'failed'}, c: {status: 'running'}}})).toMatchObject({succeeded: 1, failed: 1, running: 1})
  })

  it('runDurationText 覆盖毫秒到小时', () => {
    expect(runDurationText('2026-10-30T10:00:00Z', '2026-10-30T10:00:00.400Z')).toBe('400ms')
    expect(runDurationText('2026-10-30T10:00:00Z', '2026-10-30T10:00:12Z')).toBe('12.0s')
    expect(runDurationText('2026-10-30T10:00:00Z', '2026-10-30T10:03:04Z')).toBe('3m04s')
    expect(runDurationText('', '')).toBe('')
  })
})