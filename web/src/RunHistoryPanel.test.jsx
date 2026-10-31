// @vitest-environment jsdom

import React from 'react'
import {act} from 'react'
import {createRoot} from 'react-dom/client'
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {RunHistoryPanel} from './main.jsx'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

vi.mock('@xyflow/react', () => ({
  Background: () => <div data-testid="flow-background" />,
  Controls: () => <div data-testid="flow-controls" />,
  Handle: () => <span data-testid="flow-handle" />,
  MiniMap: () => <div data-testid="flow-minimap" />,
  Position: {Top: 'top', Right: 'right', Bottom: 'bottom', Left: 'left'},
  ReactFlow: ({nodes = [], children, onNodeClick}) => (
    <div className="react-flow">
      {nodes.map(node => (
        <button
          key={node.id}
          type="button"
          className="react-flow__node"
          onClick={event => onNodeClick?.(event, node)}
        >
          {node.data?.name || node.id}
        </button>
      ))}
      {children}
    </div>
  )
}))

vi.mock('./LiveRunTerminal.jsx', () => ({
  default: ({runID}) => <div className="liveTerminal">实时日志 {runID}</div>
}))

const catalog = {
  tools: [
    {id: 'demo.inspect', name: '巡检工具', parameters: []},
    {id: 'demo.notify', name: '通知工具', parameters: []}
  ]
}

function jsonResponse(data) {
  return Promise.resolve({ok: true, json: () => Promise.resolve(data)})
}

async function renderPanel(props = {}) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  await act(async () => {
    root.render(<RunHistoryPanel catalog={catalog} {...props} />)
  })
  return {host, root}
}

function click(element) {
  act(() => {
    element.dispatchEvent(new MouseEvent('click', {bubbles: true}))
  })
}

describe('RunHistoryPanel cleanup preview', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn((url, options = {}) => {
      const path = String(url)
      if (path === '/api/runs/cleanup') {
        const payload = JSON.parse(options.body || '{}')
        return jsonResponse({
          data: {
            keep: payload.keep,
            dry_run: payload.dry_run,
            total: 2,
            deleted: 1,
            deleted_ids: ['run-old'],
            kept_ids: ['run-new'],
            candidates: [{id: 'run-old', kind: 'tool', target: 'demo.old', status: 'succeeded', started_at: '2026-06-14T09:00:00Z'}]
          }
        })
      }
      if (path === '/api/runs/' || path.startsWith('/api/runs/?')) {
        return jsonResponse({data: {runs: [], total: 0}})
      }
      return jsonResponse({data: {}})
    }))
  })

  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('lists preview candidates and runs cleanup from the preview', async () => {
    const {host, root} = await renderPanel()
    click([...host.querySelectorAll('button')].find(button => button.textContent === '清理'))
    await vi.waitFor(() => expect(host.querySelector('.runCleanupModal')).not.toBeNull())

    const keepInput = host.querySelector('.runCleanupModal input[type="number"]')
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
      setter.call(keepInput, '1')
      keepInput.dispatchEvent(new Event('input', {bubbles: true}))
    })
    click([...host.querySelectorAll('.runCleanupModal button')].find(button => button.textContent === '开始预演'))
    await vi.waitFor(() => expect(host.querySelector('.runCleanupCandidateList')).not.toBeNull())
    expect(host.querySelector('.runCleanupCandidateList').textContent).toContain('demo.old')
    expect(host.querySelector('.runCleanupCandidateList').textContent).toContain('run-old')

    click([...host.querySelectorAll('.runCleanupPreview button')].find(button => button.textContent === '确认清理以上记录'))
    await vi.waitFor(() => {
      const cleanupCalls = fetch.mock.calls.filter(call => String(call[0]) === '/api/runs/cleanup')
      expect(cleanupCalls).toHaveLength(2)
      expect(JSON.parse(cleanupCalls[1][1].body)).toEqual({keep: 1, dry_run: false})
    })
    await vi.waitFor(() => expect(host.textContent).toContain('已清理 1 条运行记录'))

    await act(async () => root.unmount())
  })
})

describe('RunHistoryPanel canvas view', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn(url => {
      const path = String(url)
      if (path.startsWith('/api/runs/?awaiting=')) {
        return jsonResponse({data: {runs: [], total: 0}})
      }
      if (path === '/api/runs/' || path.startsWith('/api/runs/?')) {
        return jsonResponse({
          data: {
            runs: [
              {id: 'workflow-run-1', kind: 'workflow', target: 'demo.workflow', status: 'succeeded', started_at: '2026-06-18T10:00:00Z'},
              {id: 'tool-run-1', kind: 'tool', target: 'demo.inspect', status: 'succeeded', started_at: '2026-06-18T09:00:00Z'}
            ]
          }
        })
      }
      if (path === '/api/runs/workflow-run-1') {
        return jsonResponse({
          data: {
            record: {
              id: 'workflow-run-1',
              kind: 'workflow',
              target: 'demo.workflow',
              status: 'succeeded',
              steps: [
                {id: 'inspect', type: 'tool', tool: 'demo.inspect', status: 'succeeded'},
                {id: 'notify', type: 'tool', tool: 'demo.notify', status: 'succeeded'}
              ]
            },
            logs: {
              items: [
                {id: 'inspect', title: '巡检', status: 'succeeded', stdout: 'inspect ok', stderr: ''},
                {id: 'notify', title: '通知', status: 'succeeded', stdout: 'notify ok', stderr: ''}
              ]
            }
          }
        })
      }
      if (path === '/api/workflows/demo.workflow') {
        return jsonResponse({
          data: {
            config: {
              id: 'demo.workflow',
              name: '演示流程',
              nodes: [
                {id: 'inspect', tool: 'demo.inspect', name: '巡检'},
                {id: 'notify', tool: 'demo.notify', name: '通知', params: {input: '{{ .steps.inspect.stdout }}'}}
              ],
              edges: [{from: 'inspect', to: 'notify'}]
            }
          }
        })
      }
      if (path === '/api/runs/tool-run-1') {
        return jsonResponse({
          data: {
            record: {id: 'tool-run-1', kind: 'tool', target: 'demo.inspect', status: 'succeeded'},
            logs: {stdout: 'tool ok', stderr: '', items: []}
          }
        })
      }
      return jsonResponse({data: {}})
    }))
  })

  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('详情直接使用按节点分组的统一日志视图，不再有画布与终端切换', async () => {
    const {host, root} = await renderPanel()
    await vi.waitFor(() => expect(host.textContent).toContain('workflow-run-1'))

    const workflowItem = [...host.querySelectorAll('.runHistoryItem')]
      .find(item => item.textContent.includes('workflow-run-1'))
    expect([...workflowItem.querySelectorAll('button')].some(button => button.textContent === '画布')).toBe(false)
    click(workflowItem.querySelector('.runHistorySelectButton'))

    // 和工作流工作台同一套 RunLogView：进入详情直接就是按节点分组的日志列表。
    await vi.waitFor(() => expect(host.querySelector('.runLogView')).not.toBeNull())
    expect(host.querySelector('.runRecordCanvas')).toBeNull()
    // 画布视图和原始终端视图都去掉了，所以这两个切换控件也该消失。
    expect(host.querySelector('[aria-label="运行详情视图"]')).toBeNull()
    expect(host.querySelector('[aria-label="日志视图"]')).toBeNull()
    expect(host.querySelector('.liveTerminal')).toBeNull()

    expect([...host.querySelectorAll('.runLogOwnerName')].map(node => node.textContent)).toEqual(['巡检', '通知'])
    expect(host.textContent).toContain('inspect ok')
    expect(host.querySelector('[aria-label="日志级别过滤"]')).not.toBeNull()

    await act(async () => root.unmount())
  })
})

// ===== 待审批：列表可见性、详情审批卡片、确认/驳回、筛选、focusRun =====

const AWAITING_RUN_ID = 'workflow-run-await'
const AWAITING_NODE_ID = 'approval_2'
const AWAITING_MESSAGE = '请修改主机清单！！！！'

function awaitingRunListItem(overrides = {}) {
  return {
    id: AWAITING_RUN_ID,
    kind: 'workflow',
    target: 'example.system-check',
    status: 'running',
    started_at: '2026-10-30T12:00:00Z',
    awaiting_approval: true,
    approval: {
      node_id: AWAITING_NODE_ID,
      message: AWAITING_MESSAGE,
      // 用相对现在的时间，避免“已等待 X 分钟”这类文案受机器时钟影响。
      since: new Date(Date.now() - 120000).toISOString()
    },
    ...overrides
  }
}

// 详情的 approvals 数组才是审批卡片的真实数据源；批准后不再有 waiting 条目。
function awaitingRecord(waiting = true) {
  return {
    id: AWAITING_RUN_ID,
    kind: 'workflow',
    target: 'example.system-check',
    status: 'running',
    started_at: '2026-10-30T12:00:00Z',
    approvals: waiting
      ? [{node_id: AWAITING_NODE_ID, status: 'waiting', message: AWAITING_MESSAGE, since: new Date(Date.now() - 120000).toISOString()}]
      : [{node_id: AWAITING_NODE_ID, status: 'approved', message: AWAITING_MESSAGE, note: '已改好'}]
  }
}

function awaitingDetailBody(waiting = true) {
  return {record: awaitingRecord(waiting), logs: {items: []}}
}

// 审批用例共用的 fetch 桩：列表、全量计数、运行详情、工作流配置、审批接口。
function stubAwaitingFetch({onDetail, onApprove, onReject} = {}) {
  const calls = {approve: [], reject: []}
  vi.stubGlobal('fetch', vi.fn((url, options = {}) => {
    const path = String(url)
    const method = String(options.method || 'GET').toUpperCase()
    if (path.startsWith('/api/runs/?awaiting=')) {
      // limit=0 只要总数；限 5 的横幅接口顺带返回第一条摘要。
      const limit = new URLSearchParams(path.slice(path.indexOf('?') + 1)).get('limit')
      return jsonResponse({data: {runs: limit === '0' ? [] : [awaitingRunListItem()], total: 1}})
    }
    if (method === 'GET' && path.startsWith('/api/runs/?')) {
      return jsonResponse({data: {runs: [awaitingRunListItem()], total: 1}})
    }
    if (method === 'POST' && path === `/api/runs/${AWAITING_RUN_ID}/approvals/${AWAITING_NODE_ID}/approve`) {
      calls.approve.push(options)
      return onApprove ? onApprove() : jsonResponse({id: AWAITING_RUN_ID, status: 'running', data: awaitingDetailBody(false)})
    }
    if (method === 'POST' && path === `/api/runs/${AWAITING_RUN_ID}/approvals/${AWAITING_NODE_ID}/reject`) {
      calls.reject.push(options)
      return onReject ? onReject() : jsonResponse({id: AWAITING_RUN_ID, status: 'failed', data: awaitingDetailBody(false)})
    }
    if (method === 'GET' && path === `/api/runs/${AWAITING_RUN_ID}`) {
      return onDetail ? onDetail() : jsonResponse({data: awaitingDetailBody(true)})
    }
    if (path === '/api/workflows/example.system-check') {
      return jsonResponse({data: {config: {id: 'example.system-check', name: '系统巡检', nodes: [{id: AWAITING_NODE_ID, name: '人工审批'}]}}})
    }
    return jsonResponse({data: {}})
  }))
  return calls
}

describe('RunHistoryPanel 待审批', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('列表里显示待审批与审批消息，点「去确认」后详情出现审批卡片', async () => {
    stubAwaitingFetch()
    const {host, root} = await renderPanel()

    await vi.waitFor(() => expect(host.querySelector('.runHistoryItem.awaiting')).not.toBeNull())
    const item = host.querySelector('.runHistoryItem.awaiting')
    expect(item.textContent).toContain('待审批')
    expect(item.textContent).toContain('运行中')
    expect(item.textContent).toContain(AWAITING_MESSAGE)
    // 列表头的全量计数（不是当前页条数）
    expect(host.textContent).toContain('待审批 1')

    click([...item.querySelectorAll('button')].find(button => button.textContent === '去确认'))
    await vi.waitFor(() => expect(host.querySelector('.runApprovalCard')).not.toBeNull())
    const card = host.querySelector('.runApprovalCard')
    expect(card.textContent).toContain(AWAITING_MESSAGE)
    expect(card.textContent).toContain(AWAITING_NODE_ID)
    expect(card.querySelector('input[aria-label="审批备注"]')).not.toBeNull()
    expect(card.textContent).toContain('确认继续')
    expect(card.textContent).toContain('驳回')

    await act(async () => root.unmount())
  })

  // CLI 等其它进程启动的运行：列表不能用会报错的「去确认」，详情卡片要解释清楚。
  it('由其他进程启动的待审批运行只给说明，确认按钮不可用', async () => {
    const item = awaitingRunListItem({approval_actionable: false})
    vi.stubGlobal('fetch', vi.fn((url, options = {}) => {
      const path = String(url)
      const method = String(options.method || 'GET').toUpperCase()
      if (path.startsWith('/api/runs/?awaiting=')) return jsonResponse({data: {runs: [item], total: 1}})
      if (method === 'GET' && path.startsWith('/api/runs/?')) return jsonResponse({data: {runs: [item], total: 1}})
      if (method === 'GET' && path === `/api/runs/${AWAITING_RUN_ID}`) {
        return jsonResponse({data: {record: awaitingRecord(true), logs: {items: []}, actionable: false}})
      }
      if (path === '/api/workflows/example.system-check') {
        return jsonResponse({data: {config: {id: 'example.system-check', nodes: [{id: AWAITING_NODE_ID, name: '人工审批'}]}}})
      }
      return jsonResponse({data: {}})
    }))
    const {host, root} = await renderPanel()

    await vi.waitFor(() => expect(host.querySelector('.runHistoryItem.awaiting')).not.toBeNull())
    expect(host.querySelector('.runHistoryAwaitingButton')).toBeNull()
    expect(host.textContent).toContain('由其他进程启动')

    click(host.querySelector('.runHistorySelectButton'))
    await vi.waitFor(() => expect(host.querySelector('.runApprovalCard')).not.toBeNull())
    const card = host.querySelector('.runApprovalCard')
    expect(card.className).toContain('locked')
    expect(card.textContent).toContain('其他进程')
    expect([...card.querySelectorAll('button')].find(button => button.textContent === '确认继续').disabled).toBe(true)
    expect(card.querySelector('input[aria-label="审批备注"]').disabled).toBe(true)

    await act(async () => root.unmount())
  })

  it('审批卡片「确认继续」带备注提交、刷新详情并通知 App 更新计数', async () => {
    let approved = false
    let detailCalls = 0
    const calls = stubAwaitingFetch({
      onDetail: () => {
        detailCalls += 1
        return jsonResponse({data: awaitingDetailBody(!approved)})
      },
      onApprove: () => {
        approved = true
        return jsonResponse({id: AWAITING_RUN_ID, status: 'running', data: awaitingDetailBody(false)})
      }
    })
    const onAwaitingChanged = vi.fn()
    const {host, root} = await renderPanel({onAwaitingChanged})

    await vi.waitFor(() => expect(host.querySelector('.runHistoryItem.awaiting')).not.toBeNull())
    click(host.querySelector('.runHistorySelectButton'))
    await vi.waitFor(() => expect(host.querySelector('.runApprovalCard')).not.toBeNull())
    const detailCallsBeforeApprove = detailCalls

    const noteInput = host.querySelector('.runApprovalCard input[aria-label="审批备注"]')
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
      setter.call(noteInput, '已经改好主机清单')
      noteInput.dispatchEvent(new Event('input', {bubbles: true}))
    })

    click([...host.querySelectorAll('.runApprovalCard button')].find(button => button.textContent === '确认继续'))
    await vi.waitFor(() => expect(calls.approve).toHaveLength(1))
    expect(JSON.parse(calls.approve[0].body)).toEqual({note: '已经改好主机清单'})
    await vi.waitFor(() => expect(host.textContent).toContain('已确认继续'))
    expect(detailCalls).toBeGreaterThan(detailCallsBeforeApprove)
    expect(onAwaitingChanged).toHaveBeenCalled()
    await vi.waitFor(() => expect(host.querySelector('.runApprovalCard')).toBeNull())

    await act(async () => root.unmount())
  })

  it('驳回要二次确认：取消不发请求，确认后才调用 reject', async () => {
    const calls = stubAwaitingFetch()
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const {host, root} = await renderPanel({onAwaitingChanged: vi.fn()})

    await vi.waitFor(() => expect(host.querySelector('.runHistoryItem.awaiting')).not.toBeNull())
    click(host.querySelector('.runHistorySelectButton'))
    await vi.waitFor(() => expect(host.querySelector('.runApprovalCard')).not.toBeNull())

    const rejectButton = () => [...host.querySelectorAll('.runApprovalCard button')].find(button => button.textContent === '驳回')
    click(rejectButton())
    await act(async () => {})
    expect(confirmSpy).toHaveBeenCalled()
    expect(calls.reject).toHaveLength(0)

    confirmSpy.mockReturnValue(true)
    click(rejectButton())
    await vi.waitFor(() => expect(calls.reject).toHaveLength(1))
    expect(confirmSpy).toHaveBeenCalledTimes(2)

    await act(async () => root.unmount())
  })

  it('「待审批」横幅打开后列表请求带上 awaiting=1', async () => {
    stubAwaitingFetch()
    const {host, root} = await renderPanel()

    await vi.waitFor(() => expect(host.querySelector('.runHistoryAwaitingBanner')).not.toBeNull())
    const firstListCall = fetch.mock.calls.find(call => String(call[0]).includes('limit=20'))
    expect(firstListCall).toBeDefined()
    expect(String(firstListCall[0])).not.toContain('awaiting=1')

    click(host.querySelector('.runHistoryAwaitingBanner'))
    await vi.waitFor(() => {
      const withAwaiting = fetch.mock.calls.filter(call => String(call[0]).includes('awaiting=1') && String(call[0]).includes('limit=20'))
      expect(withAwaiting.length).toBeGreaterThan(0)
    })
    expect(host.querySelector('.runHistoryAwaitingBanner').className).toContain('active')

    await act(async () => root.unmount())
  })

  it('focusRun 传入后自动选中该运行并打开待审批筛选', async () => {
    stubAwaitingFetch()
    const {host, root} = await renderPanel({focusRun: {runID: AWAITING_RUN_ID, token: 1}})

    await vi.waitFor(() => expect(host.querySelector('.runApprovalCard')).not.toBeNull())
    expect(host.querySelector('.runApprovalCard').textContent).toContain(AWAITING_MESSAGE)
    expect(host.querySelector('.runHistoryAwaitingBanner').className).toContain('active')
    const awaitingListCalls = fetch.mock.calls.filter(call => String(call[0]).includes('awaiting=1') && String(call[0]).includes('limit=20'))
    expect(awaitingListCalls.length).toBeGreaterThan(0)

    await act(async () => root.unmount())
  })

  // 横幅要能把「谁在等、等什么、等了多久」一次说清：它替代了原来挤在状态 chip 里的计数。
  it('顶部横幅显示最早一条待审批的节点与已等待时长，点击可切换筛选', async () => {
    const items = [
      awaitingRunListItem(),
      awaitingRunListItem({id: 'workflow-run-await-2', target: 'example.second-check'})
    ]
    vi.stubGlobal('fetch', vi.fn(url => {
      const path = String(url)
      if (path.startsWith('/api/runs/?awaiting=')) return jsonResponse({data: {runs: items, total: 2}})
      if (path.startsWith('/api/runs/?')) return jsonResponse({data: {runs: items, total: 2}})
      return jsonResponse({data: {}})
    }))
    const {host, root} = await renderPanel()
    const banner = () => host.querySelector('.runHistoryAwaitingBanner')

    await vi.waitFor(() => expect(banner()).not.toBeNull())
    expect(banner().textContent).toContain('待审批 2')
    expect(banner().textContent).toContain(`审批节点 ${AWAITING_NODE_ID}`)
    expect(banner().textContent).toContain('已等待 2 分钟')
    expect(banner().textContent).toContain('只看待审批')

    click(banner())
    await vi.waitFor(() => expect(banner().className).toContain('active'))
    expect(banner().textContent).toContain('返回全部运行')

    await act(async () => root.unmount())
  })

  // 点下去必须有回执：按钮换文案，右侧也不能先闪「选择一条运行记录」的空态。
  it('点「去确认」立刻显示「打开中…」，右侧不会先闪空态', async () => {
    let releaseDetail
    const detailGate = new Promise(resolve => { releaseDetail = resolve })
    vi.stubGlobal('fetch', vi.fn((url, options = {}) => {
      const path = String(url)
      const method = String(options.method || 'GET').toUpperCase()
      if (path.startsWith('/api/runs/?awaiting=')) return jsonResponse({data: {runs: [awaitingRunListItem()], total: 1}})
      if (method === 'GET' && path.startsWith('/api/runs/?')) return jsonResponse({data: {runs: [awaitingRunListItem()], total: 1}})
      if (method === 'GET' && path === `/api/runs/${AWAITING_RUN_ID}`) {
        return detailGate.then(() => jsonResponse({data: awaitingDetailBody(true)}))
      }
      return jsonResponse({data: {}})
    }))
    const {host, root} = await renderPanel()

    await vi.waitFor(() => expect(host.querySelector('.runHistoryAwaitingButton')).not.toBeNull())
    click(host.querySelector('.runHistoryAwaitingButton'))

    await vi.waitFor(() => expect(host.querySelector('.runHistoryAwaitingButton').textContent).toBe('打开中…'))
    expect(host.querySelector('.runHistoryAwaitingButton').disabled).toBe(true)
    expect(host.querySelector('.runHistoryDetailLoading')).not.toBeNull()
    expect(host.textContent).not.toContain('选择一条运行记录查看日志和支持包')

    await act(async () => { releaseDetail() })
    await vi.waitFor(() => expect(host.querySelector('.runApprovalCard')).not.toBeNull())
    expect(host.querySelector('.runHistoryAwaitingButton').textContent).toBe('去确认')

    await act(async () => root.unmount())
  })

  // 审批是「有人等着」的事：分页和时间倒序都不能把它压到看不见。
  it('待审批条目置顶，其余运行按开始时间倒序', async () => {
    const waiting = awaitingRunListItem({started_at: '2026-09-01T10:00:00Z'})
    const newer = {id: 'run-newer', kind: 'tool', target: 'demo.new', status: 'failed', started_at: '2026-10-20T10:00:00Z'}
    const older = {id: 'run-older', kind: 'tool', target: 'demo.old', status: 'succeeded', started_at: '2026-10-01T10:00:00Z'}
    vi.stubGlobal('fetch', vi.fn(url => {
      const path = String(url)
      if (path.startsWith('/api/runs/?awaiting=')) return jsonResponse({data: {runs: [waiting], total: 1}})
      if (path.startsWith('/api/runs/?')) return jsonResponse({data: {runs: [newer, older, waiting], total: 3}})
      return jsonResponse({data: {}})
    }))
    const {host, root} = await renderPanel()

    await vi.waitFor(() => expect(host.querySelectorAll('.runHistoryItem')).toHaveLength(3))
    const ids = [...host.querySelectorAll('.runHistoryItem .runHistoryItemID')].map(node => node.textContent)
    expect(ids).toEqual([AWAITING_RUN_ID, 'run-newer', 'run-older'])

    await act(async () => root.unmount())
  })
})

describe('RunHistoryPanel 分栏拖动', () => {
  it('运行记录列表和右侧日志详情之间可以拖宽度，双击回到默认', async () => {
    vi.stubGlobal('fetch', vi.fn(() => jsonResponse({data: {runs: [], total: 0}})))
    try {
      const {host, root} = await renderPanel()
      const layout = host.querySelector('.runHistoryLayout')
      const handle = layout.querySelector('.resizeHandle')
      expect(handle).not.toBeNull()
      expect(handle.getAttribute('aria-label')).toBe('调整运行记录列表宽度')
      expect(layout.style.getPropertyValue('--run-history-list-width')).toBe('')

      await act(async () => {
        handle.dispatchEvent(new MouseEvent('pointerdown', {bubbles: true, cancelable: true, clientX: 300}))
        handle.dispatchEvent(new MouseEvent('pointermove', {bubbles: true, cancelable: true, clientX: 340}))
      })
      expect(layout.style.getPropertyValue('--run-history-list-width')).toBe('440px')

      await act(async () => {
        layout.querySelector('.resizeHandle').dispatchEvent(new MouseEvent('dblclick', {bubbles: true, cancelable: true}))
      })
      expect(host.querySelector('.runHistoryLayout').style.getPropertyValue('--run-history-list-width')).toBe('')

      await act(async () => root.unmount())
    } finally {
      vi.unstubAllGlobals()
    }
  })
})
