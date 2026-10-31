// @vitest-environment jsdom
import {afterEach, describe, expect, it, vi} from 'vitest'
import React from 'react'
import {createRoot} from 'react-dom/client'
import {act} from 'react'
import RunLogWindow from './RunLogWindow.jsx'

let mounted = []

afterEach(() => {
  for (const {host, root} of mounted) {
    act(() => root.unmount())
    host.remove()
  }
  mounted = []
})

const record = {id: 'run-1', status: 'succeeded', steps: [{id: 'first', type: 'tool', status: 'succeeded'}]}
const logItems = [{id: 'first', title: '第一步', status: 'succeeded', stdout: 'first-output'}]

function props(overrides = {}) {
  return {
    open: true,
    onClose: vi.fn(),
    title: '运行日志 · 第一步',
    runNodeNames: {first: '第一步'},
    runState: {nodes: {first: {status: 'succeeded'}}},
    record,
    logItems,
    activeRunID: 'run-1',
    ...overrides
  }
}

async function mountWindow(windowProps) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  mounted.push({host, root})
  await act(async () => { root.render(<RunLogWindow {...windowProps} />) })
  return host
}

describe('RunLogWindow', () => {
  it('未打开时不渲染任何内容', async () => {
    const host = await mountWindow(props({open: false}))
    expect(host.textContent).toBe('')
  })

  it('打开后是一份完整运行日志，不是又一套 tab', async () => {
    const host = await mountWindow(props())
    const window = host.querySelector('.workflowLogWindow')
    expect(window.getAttribute('aria-label')).toBe('运行日志 · 第一步')
    expect(window.querySelector('.runLogBox').textContent).toContain('first-output')
    expect(window.querySelector('.segmentedControl')).toBeNull()
    // 窗口自己有关闭按钮，日志面板里不再重复一个「收起」。
    expect(Array.from(window.querySelectorAll('button')).map(button => button.textContent)).not.toContain('收起')
  })

  it('× / Esc / 点窗口外都能关闭', async () => {
    const onClose = vi.fn()
    const host = await mountWindow(props({onClose}))

    await act(async () => {
      host.querySelector('button[aria-label="关闭日志窗口"]').dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })
    expect(onClose).toHaveBeenCalledTimes(1)

    await act(async () => {
      document.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true}))
    })
    expect(onClose).toHaveBeenCalledTimes(2)

    await act(async () => {
      host.querySelector('.workflowLogWindowBackdrop').dispatchEvent(new MouseEvent('mousedown', {bubbles: true}))
    })
    expect(onClose).toHaveBeenCalledTimes(3)
  })

  it('点窗口内部不会关闭', async () => {
    const onClose = vi.fn()
    const host = await mountWindow(props({onClose}))
    await act(async () => {
      host.querySelector('.workflowLogWindow').dispatchEvent(new MouseEvent('mousedown', {bubbles: true}))
    })
    expect(onClose).not.toHaveBeenCalled()
  })
})