// @vitest-environment jsdom

import React from 'react'
import {act} from 'react'
import {createRoot} from 'react-dom/client'
import {afterEach, describe, expect, it, vi} from 'vitest'
import {AwaitingApprovalBanner, runIDFromHash, shouldShowAwaitingBanner} from './main.jsx'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

// 只测横幅这个纯组件：不挂 App、不 mock 接口。main.jsx 底部的挂载逻辑在 jsdom 里因为
// 没有 #root 元素不会执行，所以直接 import 是安全的。
function renderBanner(props) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  act(() => {
    root.render(<AwaitingApprovalBanner {...props} />)
  })
  return {host, root}
}

function click(element) {
  act(() => {
    element.dispatchEvent(new MouseEvent('click', {bubbles: true}))
  })
}

function sampleRun(overrides = {}) {
  return {
    id: 'workflow-run-1',
    kind: 'workflow',
    target: 'example.system-check',
    status: 'running',
    awaiting_approval: true,
    approval: {node_id: 'approval_2', message: '请修改主机清单！！！！', since: '2026-10-30T12:27:31.123Z'},
    ...overrides
  }
}

describe('AwaitingApprovalBanner', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('没有待审批运行时不渲染任何内容', () => {
    const {host, root} = renderBanner({items: [], total: 0, onGo: vi.fn()})
    expect(host.querySelector('.runAwaitingBanner')).toBeNull()
    expect(host.textContent).toBe('')
    act(() => root.unmount())
  })

  it('显示总数与第一条的目标、审批消息，并支持点击「去确认」', () => {
    const onGo = vi.fn()
    const items = [sampleRun(), sampleRun({id: 'workflow-run-2', target: 'example.backup'})]
    const {host, root} = renderBanner({items, total: 2, onGo})

    expect(host.textContent).toContain('2 个运行等待人工确认')
    expect(host.textContent).toContain('example.system-check')
    expect(host.textContent).toContain('请修改主机清单')
    expect(host.textContent).toContain('还有 1 个等待确认的运行')

    click(host.querySelector('.runAwaitingBannerButton'))
    expect(onGo).toHaveBeenCalledTimes(1)
    expect(onGo.mock.calls[0][0]).toMatchObject({id: 'workflow-run-1'})

    act(() => root.unmount())
  })

  it('过长的审批消息会截断，完整内容留在 title 上', () => {
    const longMessage = '请'.repeat(80)
    const {host, root} = renderBanner({
      items: [sampleRun({approval: {node_id: 'approval_2', message: longMessage, since: ''}})],
      total: 1,
      onGo: vi.fn()
    })

    const item = host.querySelector('.runAwaitingBannerItem')
    expect(item.textContent).toContain('…')
    // 横幅里除了消息还有「工作流 · 目标」前缀，所以按“是否还包含整段消息”判断截断。
    expect(item.textContent).not.toContain(longMessage)
    expect(item.textContent.trim().endsWith('…')).toBe(true)
    expect(item.getAttribute('title')).toContain(longMessage)

    act(() => root.unmount())
  })
})

describe('runIDFromHash', () => {
  it('从工作流深链里取出运行 ID', () => {
    expect(runIDFromHash('#workflows?run=run-1')).toBe('run-1')
    expect(runIDFromHash('#workflows?run=run%2D2&other=1')).toBe('run-2')
  })

  it('普通 tab 深链没有运行 ID', () => {
    expect(runIDFromHash('#workflows')).toBe('')
    expect(runIDFromHash('#runs')).toBe('')
    expect(runIDFromHash('')).toBe('')
  })
})

describe('shouldShowAwaitingBanner', () => {
  const items = [{id: 'run-1', target: 'demo.flow'}]

  it('没有待审批运行时不提示', () => {
    expect(shouldShowAwaitingBanner({activeTab: 'tools', total: 0, items: []})).toBe(false)
  })

  it('工具与配置页需要提示', () => {
    expect(shouldShowAwaitingBanner({activeTab: 'tools', total: 1, items})).toBe(true)
    expect(shouldShowAwaitingBanner({activeTab: 'config', total: 1, items})).toBe(true)
  })

  it('运行记录页自己有审批卡片，不再提示', () => {
    expect(shouldShowAwaitingBanner({activeTab: 'runs', total: 1, items})).toBe(false)
  })

  it('工作流页正在看的就是那条待审批运行时不再提示', () => {
    expect(shouldShowAwaitingBanner({activeTab: 'workflows', total: 1, items, linkedRunID: 'run-1'})).toBe(false)
  })

  it('工作流页看的是别的运行时仍然提示', () => {
    expect(shouldShowAwaitingBanner({activeTab: 'workflows', total: 1, items, linkedRunID: 'run-2'})).toBe(true)
    expect(shouldShowAwaitingBanner({activeTab: 'workflows', total: 1, items, linkedRunID: ''})).toBe(true)
  })
})