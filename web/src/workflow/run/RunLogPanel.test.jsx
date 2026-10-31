// @vitest-environment jsdom
import {afterEach, describe, expect, it, vi} from 'vitest'
import React from 'react'
import {createRoot} from 'react-dom/client'
import {act} from 'react'
import RunLogPanel from './RunLogPanel.jsx'

let mounted = []

afterEach(() => {
  for (const {host, root} of mounted) {
    act(() => root.unmount())
    host.remove()
  }
  mounted = []
})

const runState = {status: 'running', nodes: {first: {status: 'succeeded', label: '成功'}, second: {status: 'running', label: '运行中'}}}
const record = {
  id: 'run-1',
  status: 'running',
  started_at: '2026-10-30T12:00:00Z',
  steps: [
    {id: 'first', type: 'tool', tool: 'demo.hello', status: 'succeeded', started_at: '2026-10-30T12:00:00Z', ended_at: '2026-10-30T12:00:01.500Z'},
    {id: 'second', type: 'tool', tool: 'demo.deploy', status: 'running', started_at: '2026-10-30T12:00:01Z', error: '连接超时'}
  ]
}
const logItems = [
  {id: 'first', title: '第一步', status: 'succeeded', stdout: 'first-output'},
  {id: 'second', title: '第二步', status: 'running', stdout: 'second-output\nsecond-line', stderr: 'second-error'}
]

function baseProps(overrides = {}) {
  return {
    nodeOrder: ['first', 'second'],
    runNodeNames: {first: '第一步', second: '第二步'},
    runState,
    record,
    logItems,
    activeRunID: 'run-1',
    isRunning: true,
    machineLabel: 'B 机',
    runningNodeID: 'second',
    onRerun: vi.fn(),
    onClose: vi.fn(),
    ...overrides
  }
}

async function mountPanel(props) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  mounted.push({host, root})
  await act(async () => { root.render(<RunLogPanel {...props} />) })
  return host
}

function clickByText(host, text, scope = host) {
  const button = Array.from(scope.querySelectorAll('button')).find(item => item.textContent === text)
  if (!button) throw new Error(`未找到按钮：${text}`)
  return act(async () => { button.dispatchEvent(new MouseEvent('click', {bubbles: true})) })
}

describe('RunLogPanel 就是运行日志页', () => {
  it('默认直接给整份运行日志，没有 tab 和节点列表', async () => {
    const host = await mountPanel(baseProps())
    const box = host.querySelector('.runLogBox')
    expect(box.textContent).toContain('first-output')
    expect(box.textContent).toContain('second-output')
    expect(box.dataset.onlyNode).toBe('0')
    // 旧的 tab / 节点列表 / 只看此节点勾选都不再出现。
    expect(host.querySelector('.segmentedControl')).toBeNull()
    expect(host.querySelector('.workflowRunDockNodes')).toBeNull()
    expect(host.textContent).not.toContain('只看此节点')
  })

  it('点节点只聚焦日志，不换视图', async () => {
    const host = await mountPanel(baseProps({focusNodeID: 'second', focusNodeName: '第二步'}))
    expect(host.querySelector('.runLogOwner.focus .runLogOwnerName').textContent).toBe('第二步')
    expect(host.querySelector('.workflowRunFocusChip').textContent).toContain('聚焦：第二步')
    expect(host.querySelector('.runLogBox').textContent).toContain('first-output')
  })

  it('没有点节点时聚焦到正在执行的节点', async () => {
    const host = await mountPanel(baseProps({focusNodeID: ''}))
    expect(host.querySelector('.runLogOwner.focus .runLogOwnerName').textContent).toBe('第二步')
    expect(host.querySelector('.runLogOwner.running')).toBeTruthy()
    expect(host.querySelector('.workflowRunFocusChip')).toBeNull()
  })
})

describe('RunLogPanel 顶部折叠区', () => {
  it('概览默认收起，打开后给该节点的结论、参数与重试', async () => {
    const host = await mountPanel(baseProps({focusNodeID: 'second', focusNodeName: '第二步', node: {data: {params: {target: '/srv'}}}}))
    expect(host.querySelector('.nodeRunOverview')).toBeNull()

    await clickByText(host, '概览')
    const text = host.querySelector('.nodeRunOverview').textContent
    expect(text).toContain('demo.deploy')
    expect(text).toContain('连接超时')
    expect(text).toContain('target')
    expect(text).toContain('/srv')
    // 打开概览不影响日志仍然在下面。
    expect(host.querySelector('.runLogBox').textContent).toContain('second-output')
  })

  it('没点节点时概览给运行统计', async () => {
    const host = await mountPanel(baseProps())
    await clickByText(host, '概览')
    const text = host.querySelector('.nodeRunOverview').textContent
    expect(text).toContain('run-1')
    expect(text).toContain('1 / 0')
  })

  it('重试按钮在给出原因时禁用', async () => {
    const host = await mountPanel(baseProps({focusNodeID: 'second', isRunning: false, rerunDisabledReason: '运行进行中，结束后才能重试单个节点。'}))
    await clickByText(host, '概览')
    const rerun = Array.from(host.querySelectorAll('button')).find(button => button.textContent === '重试该节点')
    expect(rerun.disabled).toBe(true)
    expect(host.querySelector('.nodeRunOverview').textContent).toContain('结束后才能重试')
  })

  it('有恢复数据时才出现恢复开关，打开才去读取', async () => {
    const onRequestRecovery = vi.fn()
    const host = await mountPanel(baseProps({recoveryAvailable: false}))
    expect(Array.from(host.querySelectorAll('.workflowRunDockActions button')).map(button => button.textContent)).toEqual(['概览'])
    expect(Array.from(host.querySelectorAll('.workflowRunDockWindowActions button')).map(button => button.textContent)).toEqual(['收起'])
    expect(onRequestRecovery).not.toHaveBeenCalled()

    const withRecovery = await mountPanel(baseProps({recoveryAvailable: true, onRequestRecovery}))
    expect(host.querySelector('.nodeRunRecovery')).toBeNull()
    await clickByText(withRecovery, '恢复')
    expect(onRequestRecovery).toHaveBeenCalledTimes(1)
    expect(withRecovery.querySelector('.nodeRunRecovery').textContent).toContain('正在读取待处理运行')
  })

  it('换运行记录时折叠区自动收起', async () => {
    const host = await mountPanel(baseProps({focusNodeID: 'second', recoveryAvailable: true, recoveryPanel: React.createElement('div', null, '恢复操作区')}))
    await clickByText(host, '概览')
    await clickByText(host, '恢复')
    expect(host.querySelector('.nodeRunOverview')).toBeTruthy()
    expect(host.querySelector('.nodeRunRecovery')).toBeTruthy()

    await act(async () => {
      mounted[0].root.render(React.createElement(RunLogPanel, baseProps({focusNodeID: 'second', activeRunID: 'run-2', recoveryAvailable: true, recoveryPanel: React.createElement('div', null, '恢复操作区')})))
    })
    expect(host.querySelector('.nodeRunOverview')).toBeNull()
    expect(host.querySelector('.nodeRunRecovery')).toBeNull()
  })
})
