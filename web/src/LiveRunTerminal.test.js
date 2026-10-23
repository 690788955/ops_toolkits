// @vitest-environment jsdom
import React, {act} from 'react'
import {createRoot} from 'react-dom/client'
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import LiveRunTerminal from './LiveRunTerminal.jsx'
import {normalizeLogFontSize} from './logUtils.js'

// 组件把整批日志合并成一次 write 写入，这里按行拆开记到 writeln 上，断言仍然按行计数。
const terminal = vi.hoisted(() => ({
  writeln: vi.fn(),
  write: vi.fn(function (data, callback) {
    String(data).split('\r\n').forEach(line => terminal.writeln(line))
    callback?.()
  }),
  reset: vi.fn(),
  scrollToBottom: vi.fn()
}))

vi.mock('./api.js', () => ({fetchUIPreferences: vi.fn(async () => ({data: {log_font_size: 14}}))}))
vi.mock('./XTerminal.jsx', async () => {
  const {forwardRef, useEffect, useImperativeHandle} = await import('react')
  return {
    default: forwardRef(function MockTerminal({onReady}, ref) {
      useImperativeHandle(ref, () => terminal, [])
      useEffect(() => { onReady?.() }, [])
      return null
    })
  }
})

globalThis.IS_REACT_ACT_ENVIRONMENT = true

let host
let root
let sources

beforeEach(() => {
  vi.clearAllMocks()
  sources = []
  vi.stubGlobal('EventSource', class {
    listeners = new Map()
    close = vi.fn()
    constructor(url) {
      this.url = url
      sources.push(this)
    }
    addEventListener(type, listener) {
      this.listeners.set(type, listener)
    }
    log(event) {
      this.listeners.get('log')({data: JSON.stringify(event)})
    }
  })
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})

afterEach(async () => {
  await act(async () => root.unmount())
  host.remove()
  vi.unstubAllGlobals()
})

async function render(props) {
  await act(async () => root.render(React.createElement(LiveRunTerminal, props)))
  // 流式日志按帧攒批后整块写入，渲染结束后把待刷新的定时器跑完再断言。
  await act(async () => { await new Promise(resolve => setTimeout(resolve, 20)) })
}

function logEvent(line, text, overrides = {}) {
  return {run_id: 'run-1', item_id: 'run-1', stream: 'stdout', line, text, ...overrides}
}

function count(level, value) {
  expect([...host.querySelectorAll('button')].some(button => button.textContent === `${level} ${value}`)).toBe(true)
}

describe('LiveRunTerminal logs', () => {
  it('does not replay streamed logs when the completed snapshot arrives', async () => {
    await render({runID: 'run-1', running: true})
    await act(async () => {
      sources[0].log(logEvent(1, 'command started'))
      sources[0].log(logEvent(2, 'hello'))
      sources[0].log(logEvent(3, 'completed'))
    })
    await render({runID: 'run-1', initialItems: [{id: 'run-1', stdout: 'command started\nhello\ncompleted\n'}]})

    expect(terminal.writeln).toHaveBeenCalledTimes(3)
    count('log', 2)
    count('success', 1)
  })

  it('preserves identical lines at different positions and ignores stream replay', async () => {
    await render({runID: 'run-1', running: true, initialItems: [{id: 'run-1', stdout: 'same\nsame\n'}]})
    await act(async () => {
      sources[0].log(logEvent(1, 'same'))
      sources[0].log(logEvent(2, 'same'))
      sources[0].log(logEvent(3, 'different'))
      sources[0].log(logEvent(4, 'same'))
      sources[0].log(logEvent(1, 'same'))
    })
    await render({runID: 'run-1', initialItems: [{id: 'run-1', stdout: 'same\nsame\ndifferent\nsame\n'}]})

    expect(terminal.writeln).toHaveBeenCalledTimes(4)
    expect(terminal.writeln.mock.calls.filter(([text]) => text.endsWith(' same'))).toHaveLength(3)
    count('log', 4)
  })

  it('tracks stdout, stderr and workflow steps independently as snapshots grow', async () => {
    const items = [{id: 'first', stdout: 'same\n', stderr: 'same\n'}, {id: 'second', stdout: 'same\n'}]
    await render({runID: 'run-1', running: true, initialItems: items})
    await render({runID: 'run-1', running: true, initialItems: [{...items[0], stdout: 'same\nnew output\n'}, items[1]]})

    expect(terminal.writeln).toHaveBeenCalledTimes(4)
    expect(terminal.writeln.mock.calls.at(-1)[0]).toContain('new output')
    count('log', 4)
  })

  it('resets the log identities when switching runs with the same step snapshot', async () => {
    const initialItems = [{id: 'shared-step', stdout: 'same\n'}]
    await render({runID: 'run-1', initialItems})
    // 终端按运行重建，重置只发生在切换运行后的新终端上，同一运行不会因快照到达再清屏。
    terminal.reset.mockClear()
    await render({runID: 'run-2', initialItems})

    // 切换运行会重建终端，只在新终端里重放当前运行的日志。
    expect(terminal.reset.mock.calls.length).toBeLessThanOrEqual(2)
    expect(terminal.writeln.mock.calls.slice(-1)[0][0]).toContain(' same')
    expect([...host.querySelectorAll('button')].some(button => button.textContent === 'log 1')).toBe(true)
  })

  it('waits for an incomplete line before merging the final output', async () => {
    await render({runID: 'run-1', running: true, initialItems: [{id: 'run-1', stdout: 'first\npar'}]})
    count('log', 1)
    await act(async () => {
      sources[0].log(logEvent(1, 'first'))
      sources[0].log(logEvent(2, 'partial'))
    })
    await render({runID: 'run-1', initialItems: [{id: 'run-1', stdout: 'first\npartial'}]})

    expect(terminal.writeln).toHaveBeenCalledTimes(2)
    expect(terminal.writeln.mock.calls.at(-1)[0]).toContain('partial')
    count('log', 2)
  })
})

describe('LiveRunTerminal bulk snapshot', () => {
  it('writes a large backlog in one block instead of one write per line', async () => {
    const stdout = Array.from({length: 2000}, (_, index) => `line ${index}`).join('\n') + '\n'
    await render({runID: 'run-1', initialItems: [{id: 'run-1', stdout}]})
    expect(terminal.write.mock.calls.length).toBeLessThan(10)

    const beforeSwitch = terminal.write.mock.calls.length
    await render({runID: 'run-2', running: true, initialItems: [{id: 'run-2', stdout: 'fresh\n'}]})
    // 切换运行只重放当前运行的日志，历史 2000 行不会在之后快照到达时被重新灌入。
    expect(terminal.write.mock.calls.length - beforeSwitch).toBeLessThan(10)
    expect(terminal.writeln.mock.calls.slice(-1)[0][0]).toContain(' fresh')
    expect([...host.querySelectorAll('button')].some(button => button.textContent.startsWith('log '))).toBe(true)
  })
})

describe('LiveRunTerminal preferences', () => {
  it('normalizes log font size from UI preferences', () => {
    expect(normalizeLogFontSize(undefined)).toBe(14)
    expect(normalizeLogFontSize('abc')).toBe(14)
    expect(normalizeLogFontSize(11)).toBe(14)
    expect(normalizeLogFontSize(21)).toBe(14)
    expect(normalizeLogFontSize('16')).toBe(16)
    expect(normalizeLogFontSize(16.4)).toBe(16)
  })
})
