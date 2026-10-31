// @vitest-environment jsdom

import {beforeEach, describe, expect, it, vi} from 'vitest'
import React from 'react'
import {createRoot} from 'react-dom/client'
import {renderToStaticMarkup} from 'react-dom/server'
import {act} from 'react'
import {ReactFlow} from '@xyflow/react'
import {defaultCondition} from './model.js'
import WorkflowEditor, {
  RecoveryPanel,
  RerunParamsModal,
  buildCanvasRunStateFromDetail,
  buildRunningCanvasRunState,
  buildRerunCanvasRunState,
  describeRunMachines,
  legacyLoopTargetMap,
  pendingRunApproval,
  pruneWorkflowRecovery,
  remapLegacyLoopEdges,
  shouldClearCanvasRunOverlayForNodeChanges,
  splitNodeIDList,
  workflowNodeToFlowNode,
  workflowNotificationSummary
} from './WorkflowEditor.jsx'
import {nodeTypes, normalizeRunStatus, runStatusLabel} from './nodes.jsx'

async function selectWorkflow(host, id) {
  const trigger = host.querySelector('.workflowNameTrigger')
  if (!trigger) throw new Error('未找到工作流名称入口')
  await act(async () => {
    trigger.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
  })
  const item = Array.from(host.querySelectorAll('[role="dialog"][aria-label="切换工作流"] .workflowLoaderItem'))
    .find(button => button.dataset.workflowId === id)
  if (!item) throw new Error('未找到工作流选择器')
  await act(async () => {
    item.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
  })
  await act(async () => {})
}

async function clickByText(container, text) {
  const button = Array.from(container.querySelectorAll('button')).find(item => item.textContent === text)
  if (!button) throw new Error(`未找到按钮：${text}`)
  await act(async () => {
    button.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
  })
}

describe('workflow node run status', () => {
  beforeEach(() => {
    if (!globalThis.ResizeObserver) {
      globalThis.ResizeObserver = class ResizeObserver {
        observe() {}
        unobserve() {}
        disconnect() {}
      }
    }
  })

  it('normalizes pending and rolled back run statuses', () => {
    expect(normalizeRunStatus('pending')).toBe('pending')
    expect(runStatusLabel('pending')).toBe('待处理')
    expect(normalizeRunStatus('rolled_back')).toBe('rolled_back')
    expect(runStatusLabel('rolled_back')).toBe('已回滚')
    expect(normalizeRunStatus('waiting')).toBe('waiting')
    expect(runStatusLabel('waiting')).toBe('等待中')
  })

  it('marks serial successors in later stages as affected by a rerun', () => {
    const stages = [{id: 'prepare'}, {id: 'verify'}]
    const nodes = [
      {id: 'third', data: {stageId: 'verify', order: 10}},
      {id: 'first', data: {stageId: 'prepare', order: 10}},
      {id: 'second', data: {stageId: 'prepare', order: 20}}
    ]
    const state = buildRerunCanvasRunState(nodes, [], {nodes: {first: {status: 'succeeded'}}}, 'second', stages)
    expect(state.nodes.first.status).toBe('succeeded')
    expect(state.nodes.second.status).toBe('running')
    expect(state.nodes.third.status).toBe('waiting')
    const initialState = buildRunningCanvasRunState(nodes, [], stages)
    expect(initialState.nodes.first.status).toBe('running')
    expect(initialState.nodes.second.status).toBe('waiting')
    expect(initialState.nodes.third.status).toBe('waiting')
  })

  it('parallel rerun preserves unrelated siblings and includes dependency successors and later stages', () => {
    const stages = [{id: 'fanout', execution_mode: 'parallel'}, {id: 'verify'}]
    const nodes = [
      {id: 'child', data: {stageId: 'fanout', order: 10}},
      {id: 'target', data: {stageId: 'fanout', order: 20}},
      {id: 'sibling', data: {stageId: 'fanout', order: 30}},
      {id: 'later', data: {stageId: 'verify', order: 10}}
    ]
    const state = buildRerunCanvasRunState(nodes, [{source: 'target', target: 'child'}], {nodes: {sibling: {status: 'succeeded'}, child: {status: 'failed'}}}, 'target', stages)
    expect(state.nodes.target.status).toBe('running')
    expect(state.nodes.child.status).toBe('waiting')
    expect(state.nodes.later.status).toBe('waiting')
    expect(state.nodes.sibling.status).toBe('succeeded')
  })

  it('parallel rerun includes same-host mutation successors using the actual snapshot targets', () => {
    const stages = [{id: 'change', execution_mode: 'parallel'}]
    const nodes = ['deploy', 'service', 'sibling'].map((id, index) => ({id, type: 'controlNode', data: {controlType: 'shell_script', stageId: 'change', order: (index + 1) * 10, host: `host-${id}`}}))
    const state = buildRerunCanvasRunState(nodes, [], {nodes: {sibling: {status: 'succeeded'}}}, 'deploy', stages, {change_node_ids: ['deploy'], service_node_ids: ['service']}, [], [{id: 'deploy', host: 'actual'}, {id: 'service', host: 'actual'}])
    expect(state.nodes.deploy.status).toBe('running')
    expect(state.nodes.service.status).toBe('waiting')
    expect(state.nodes.sibling.status).toBe('succeeded')
  })

  it('shows execution hosts from step records instead of the workflow target ID', () => {
    const record = {target: 'demo.workflow', steps: [
      {id: 'first', type: 'tool', status: 'succeeded', host_id: 'host-b', host_name: 'B 机'},
      {id: 'check', type: 'http_request', status: 'succeeded'}
    ]}
    expect(describeRunMachines(record, [], [], '跟随节点配置')).toBe('B 机、本机')
    expect(describeRunMachines(null, [{type: 'toolNode', data: {host: 'host-b'}}], [{id: 'host-b', name: 'B 机'}], '本机')).toBe('B 机')
  })

  it('renders condition nodes with visible branches', () => {
    const markup = renderToStaticMarkup(React.createElement(ReactFlow, {
      nodes: [{
        id: 'route',
        type: 'conditionNode',
        position: {x: 0, y: 0},
        data: {
          name: '按结果分支',
          condition: {
            ...defaultCondition(),
            input: '{{ .steps.inspect.stdout }}',
            cases: [{id: 'ok', name: '正常', operator: 'contains', values: ['OK']}]
          },
          onRemove: () => {}
        }
      }],
      edges: [],
      nodeTypes,
      fitView: true
    }))

    expect(markup).toContain('按结果分支')
    expect(markup).toContain('正常')
    expect(markup).toContain('default')
  })

  it('人工审批节点显示审批摘要，真正在等待时标签是「等待审批」', () => {
    const markup = renderToStaticMarkup(React.createElement(ReactFlow, {
      nodes: [
        {
          id: 'gate',
          type: 'controlNode',
          position: {x: 0, y: 0},
          data: {
            name: '人工确认',
            controlType: 'approval',
            approval: {message: '确认配置已改好后继续', timeout: '30m'},
            run: {status: 'waiting', type: 'approval'}
          }
        },
        {
          id: 'empty-gate',
          type: 'controlNode',
          position: {x: 0, y: 180},
          data: {name: '未填提示', controlType: 'approval', run: {status: 'waiting'}}
        }
      ],
      edges: [],
      nodeTypes,
      fitView: true
    }))

    expect(markup).toContain('确认配置已改好后继续')
    expect(markup).toContain('超时 30m')
    expect(markup).toContain('未填写审批提示')
    expect(markup).toContain('等待审批')
    // 只有审批步骤真的进入 waiting 才换成审批文案，其它节点仍是普通的「等待中」。
    expect(markup).toContain('等待中')
  })

  it('等待审批的 step 不会被降级成未运行', () => {
    const nodes = [{id: 'gate', type: 'controlNode', data: {controlType: 'approval'}}]
    const detail = {
      record: {id: 'run-1', status: 'running', steps: [{id: 'gate', type: 'approval', status: 'waiting'}]},
      logs: {items: []}
    }
    const state = buildCanvasRunStateFromDetail(detail, nodes)
    expect(state.status).toBe('running')
    // waiting 必须原样保留：降级成 idle 会让审批节点在画布上没有任何状态提示。
    expect(state.nodes.gate).toMatchObject({status: 'waiting', type: 'approval', label: '等待中'})
  })

  it('只把最后一条仍在等待的审批当作待处理项', () => {
    expect(pendingRunApproval(null)).toBeNull()
    expect(pendingRunApproval({approvals: []})).toBeNull()
    expect(pendingRunApproval({approvals: [{node_id: 'a', status: 'approved'}, {node_id: 'b', status: 'rejected'}]})).toBeNull()
    const record = {
      approvals: [
        {node_id: 'a', status: 'approved'},
        {node_id: 'b', status: 'waiting', message: '旧审批'},
        {node_id: 'c', status: 'waiting', message: '新审批'},
        {node_id: '', status: 'waiting'}
      ]
    }
    expect(pendingRunApproval(record)).toMatchObject({node_id: 'c', message: '新审批'})
  })

  it('keeps run overlay for canvas view and selection updates', () => {
    expect(shouldClearCanvasRunOverlayForNodeChanges([{type: 'select', id: 'step', selected: true}])).toBe(false)
    expect(shouldClearCanvasRunOverlayForNodeChanges([{type: 'position', id: 'step', dragging: false, position: {x: 80, y: 120}}])).toBe(false)
    expect(shouldClearCanvasRunOverlayForNodeChanges([{type: 'dimensions', id: 'step'}])).toBe(false)
    expect(shouldClearCanvasRunOverlayForNodeChanges([{type: 'remove', id: 'step'}])).toBe(true)
  })

  it('calls rerun handler from a node-local rerun button', async () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    const onRerun = vi.fn()
    await act(async () => {
      root.render(React.createElement(ReactFlow, {
        nodes: [{
          id: 'step',
          type: 'toolNode',
          position: {x: 0, y: 0},
          data: {
            name: '执行步骤',
            tool: 'demo.tool',
            run: {status: 'failed', stderr: 'failed'},
            onRerun
          }
        }],
        edges: [],
        nodeTypes,
        fitView: true
      }))
    })

    const button = host.querySelector('.nodeRerun')
    expect(button).toBeTruthy()
    await act(async () => {
      button.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
    })

    expect(onRerun).toHaveBeenCalledWith('step')
    root.unmount()
    host.remove()
  })

  it('submits edited rerun parameters', async () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    const onSubmit = vi.fn()
    await act(async () => {
      root.render(React.createElement(RerunParamsModal, {
        nodeID: 'step',
        params: {name: 'old'},
        confirmRequired: false,
        onClose: () => {},
        onSubmit
      }))
    })

    const input = host.querySelector('textarea')
    expect(input).toBeTruthy()
    const setValue = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set
    await act(async () => {
      setValue.call(input, '{"name":"new"}')
      input.dispatchEvent(new InputEvent('input', {bubbles: true, inputType: 'insertText', data: 'new'}))
    })
    const button = Array.from(host.querySelectorAll('button')).find(item => item.textContent === '开始重跑')
    await act(async () => {
      button.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
    })

    expect(onSubmit).toHaveBeenCalledWith({name: 'new'})
    root.unmount()
    host.remove()
  })

  it('opens the run drawer without validating upload files until the run starts', async () => {
    const workflow = {
      id: 'upload-flow',
      name: '上传工作流',
      category: 'global',
      parameters: [],
      nodes: [{id: 'upload_1', type: 'upload', name: '上传文件'}],
      edges: []
    }
    const originalFetch = globalThis.fetch
    globalThis.fetch = vi.fn(async path => {
      if (path === '/api/workflows/upload-flow') {
        return new Response(JSON.stringify({data: {config: workflow}}), {status: 200, headers: {'Content-Type': 'application/json'}})
      }
      return new Response(JSON.stringify({error: `unexpected path ${path}`}), {status: 404, headers: {'Content-Type': 'application/json'}})
    })
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    const setResult = vi.fn()
    try {
      await act(async () => {
        root.render(React.createElement(WorkflowEditor, {
          catalog: {tools: [], workflows: [workflow]},
          setResult,
          refreshCatalog: vi.fn()
        }))
      })

      await selectWorkflow(host, 'upload-flow')

      const runButton = host.querySelector('button[aria-label="运行工作流"]')
      expect(runButton).toBeTruthy()
      await act(async () => {
        runButton.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
      })

      const drawer = host.querySelector('[role="dialog"][aria-label="运行准备"]')
      expect(drawer).toBeTruthy()
      expect(drawer.textContent).toContain('上传文件')
      expect(drawer.textContent).toContain('上传文件：未选择')
      expect(drawer.textContent).toContain('执行机器')
      expect(drawer.querySelector('.validationSummary')).toBeNull()

      await clickByText(drawer, '开始运行')

      expect(host.querySelector('[role="dialog"][aria-label="运行准备"]')).toBeTruthy()
      expect(drawer.querySelector('.validationSummary')).toBeTruthy()
      expect(drawer.textContent).toContain('上传节点 upload_1 请选择要上传的文件或目录。')
    } finally {
      root.unmount()
      host.remove()
      globalThis.fetch = originalFetch
    }
  })

  it('opens the run setup drawer from the top bar action', async () => {
    const workflow = {
      id: 'sidebar-run-flow',
      name: '侧栏运行工作流',
      category: 'global',
      parameters: [],
      nodes: [{id: 'upload_1', type: 'upload', name: '上传文件'}],
      edges: []
    }
    const originalFetch = globalThis.fetch
    globalThis.fetch = vi.fn(async path => {
      if (path === '/api/workflows/sidebar-run-flow') {
        return new Response(JSON.stringify({data: {config: workflow}}), {status: 200, headers: {'Content-Type': 'application/json'}})
      }
      return new Response(JSON.stringify({error: `unexpected path ${path}`}), {status: 404, headers: {'Content-Type': 'application/json'}})
    })
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    try {
      await act(async () => {
        root.render(React.createElement(WorkflowEditor, {
          catalog: {tools: [], workflows: [workflow]},
          setResult: vi.fn(),
          refreshCatalog: vi.fn()
        }))
      })

      await selectWorkflow(host, 'sidebar-run-flow')

      const runButton = host.querySelector('button[aria-label="运行工作流"]')
      expect(runButton).toBeTruthy()
      await act(async () => {
        runButton.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
      })

      const drawer = host.querySelector('[role="dialog"][aria-label="运行准备"]')
      expect(drawer).toBeTruthy()
      expect(drawer.textContent).toContain('工作流参数')
      expect(drawer.textContent).toContain('上传文件')
      expect(drawer.textContent).toContain('执行机器')
      expect(drawer.textContent).toContain('运行摘要')
      expect(drawer.querySelector('button[aria-label="开始运行工作流"]')).toBeTruthy()
    } finally {
      root.unmount()
      host.remove()
      globalThis.fetch = originalFetch
    }
  })

  it('does not duplicate tool node parameters in the run drawer', async () => {
    const workflow = {
      id: 'tool-param-flow',
      name: '工具参数工作流',
      category: 'global',
      parameters: [],
      nodes: [{id: 'merge_1', type: 'tool', name: '分片包 MD5 校验并合并', tool: 'docker.exec.verify_md5_merge', params: {}}],
      edges: []
    }
    const tool = {
      id: 'docker.exec.verify_md5_merge',
      name: 'Docker 容器执行',
      parameters: [
        {name: 'package_name', description: '分片包文件名', type: 'string', required: true},
        {name: 'extract_dir', description: '解压目录', type: 'string'}
      ]
    }
    const originalFetch = globalThis.fetch
    globalThis.fetch = vi.fn(async path => {
      if (path === '/api/workflows/tool-param-flow') {
        return new Response(JSON.stringify({data: {config: workflow}}), {status: 200, headers: {'Content-Type': 'application/json'}})
      }
      return new Response(JSON.stringify({error: `unexpected path ${path}`}), {status: 404, headers: {'Content-Type': 'application/json'}})
    })
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    try {
      await act(async () => {
        root.render(React.createElement(WorkflowEditor, {
          catalog: {tools: [tool], workflows: [workflow]},
          setResult: vi.fn(),
          refreshCatalog: vi.fn()
        }))
      })

      await selectWorkflow(host, 'tool-param-flow')

      const runButton = host.querySelector('button[aria-label="运行工作流"]')
      expect(runButton).toBeTruthy()
      await act(async () => {
        runButton.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
      })

      const drawer = host.querySelector('[role="dialog"][aria-label="运行准备"]')
      expect(drawer).toBeTruthy()
      expect(drawer.textContent).toContain('节点参数请在画布中打开对应节点配置')
      expect(drawer.textContent).not.toContain('分片包文件名')

      await clickByText(drawer, '开始运行')

      expect(drawer.textContent).toContain('节点 merge_1（Docker 容器执行）缺少必填参数：package_name')
    } finally {
      root.unmount()
      host.remove()
      globalThis.fetch = originalFetch
    }
  })

  it('keeps the canvas mounted and opens run history in a drawer', async () => {
    const workflow = {
      id: 'view-switch-flow',
      name: '视图切换工作流',
      category: 'global',
      parameters: [],
      nodes: [{id: 'inspect', type: 'tool', name: '巡检', tool: 'demo.inspect'}],
      edges: []
    }
    const originalFetch = globalThis.fetch
    globalThis.fetch = vi.fn(async path => {
      if (path === '/api/workflows/view-switch-flow') {
        return new Response(JSON.stringify({data: {config: workflow}}), {status: 200, headers: {'Content-Type': 'application/json'}})
      }
      return new Response(JSON.stringify({error: `unexpected path ${path}`}), {status: 404, headers: {'Content-Type': 'application/json'}})
    })
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    try {
      await act(async () => {
        root.render(React.createElement(WorkflowEditor, {
          catalog: {tools: [{id: 'demo.inspect', name: '巡检工具'}], workflows: [workflow]},
          setResult: vi.fn(),
          refreshCatalog: vi.fn(),
          resultPanel: React.createElement('section', {className: 'card resultCard workflowResultCard'}, '执行日志面板')
        }))
      })

      await selectWorkflow(host, 'view-switch-flow')

      expect(host.querySelector('.canvasCard')).toBeTruthy()
      expect(host.textContent).not.toContain('执行日志面板')

      const moreButton = Array.from(host.querySelectorAll('.workflowTopBar button')).find(button => button.textContent === '更多')
      expect(moreButton).toBeTruthy()
      await act(async () => {
        moreButton.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
      })
      const historyItem = Array.from(host.querySelectorAll('[role="menuitem"]')).find(item => item.textContent === '运行记录')
      await act(async () => {
        historyItem.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
      })

      const drawer = host.querySelector('[role="dialog"][aria-label="运行日志"]')
      expect(drawer).toBeTruthy()
      expect(drawer.textContent).toContain('运行结果为只读快照')
      expect(drawer.textContent).toContain('执行日志面板')
      expect(host.querySelector('.canvasCard')).toBeTruthy()
    } finally {
      root.unmount()
      host.remove()
      globalThis.fetch = originalFetch
    }
  })

  it('remaps legacy loop target edges onto the loop node', () => {
    const nodes = [
      {id: 'loop_1', type: 'loop', loop: {target: 'step'}},
      {id: 'step', type: 'tool', tool: 'demo.step'},
      {id: 'next', type: 'tool', tool: 'demo.next'}
    ]
    const targetMap = legacyLoopTargetMap(nodes)
    expect(targetMap.get('step')).toBe('loop_1')

    const remapped = remapLegacyLoopEdges([
      {from: 'step', to: 'next'},
      {from: 'step', to: 'next'},
      {from: 'loop_1', to: 'step'}
    ], targetMap)
    expect(remapped).toEqual([{from: 'loop_1', to: 'next'}])
  })

  it('loads extract_config workflow nodes as control nodes', () => {
    const node = workflowNodeToFlowNode({
      id: 'extract_config_3',
      type: 'extract_config',
      name: '解析配置',
      extract_config: {
        file_name: '{{ .steps.merge.outputs.output_file }}',
        target_path: 'everisk-deployment/config.yaml',
        replace: true
      }
    }, 0, () => {})

    expect(node.type).toBe('controlNode')
    expect(node.data.controlType).toBe('extract_config')
    expect(node.data.extract_config.target_path).toBe('everisk-deployment/config.yaml')
  })

  it('loads http_request workflow nodes as control nodes', () => {
    const node = workflowNodeToFlowNode({
      id: 'call_api',
      type: 'http_request',
      name: '调用接口',
      http: {
        method: 'post',
        url: 'https://api.example.com/deploy',
        headers: {'X-Token': 'abc'},
        body_type: 'json',
        body: '{"env":"prod"}',
        expect_status: [201]
      }
    }, 0, () => {})

    expect(node.type).toBe('controlNode')
    expect(node.data.controlType).toBe('http_request')
    expect(node.data.http.method).toBe('POST')
    expect(node.data.http.headers).toEqual([{name: 'X-Token', value: 'abc'}])
    expect(node.data.http.expect_status).toEqual([201])
  })

  it('filters the workflow loader by the active category', async () => {
    const workflows = [
      {id: 'docker-flow', name: 'Docker 巡检', category: 'docker', parameters: [], nodes: [], edges: []},
      {id: 'system-flow', name: '系统巡检', category: 'system', parameters: [], nodes: [], edges: []}
    ]
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    try {
      await act(async () => {
        root.render(React.createElement(WorkflowEditor, {
          catalog: {
            categories: [{id: 'docker', name: 'Docker'}, {id: 'system', name: '系统管理'}],
            tools: [],
            workflows
          },
          activeCategory: 'docker',
          setResult: vi.fn(),
          refreshCatalog: vi.fn()
        }))
      })

      const trigger = host.querySelector('.workflowNameTrigger')
      await act(async () => {
        trigger.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
      })
      const categorySelect = host.querySelector('[role="dialog"][aria-label="切换工作流"] select[aria-label="按分类筛选工作流"]')
      expect(categorySelect.value).toBe('docker')

      let itemNames = [...host.querySelectorAll('[role="dialog"][aria-label="切换工作流"] .workflowLoaderItem strong')].map(item => item.textContent)
      expect(itemNames).toContain('Docker 巡检')
      expect(itemNames).not.toContain('系统巡检')
      expect(host.querySelector('.workflowLoaderGroupLabel').textContent).toContain('Docker')

      const setSelectValue = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set
      await act(async () => {
        setSelectValue.call(categorySelect, '')
        categorySelect.dispatchEvent(new Event('change', {bubbles: true}))
      })

      itemNames = [...host.querySelectorAll('[role="dialog"][aria-label="切换工作流"] .workflowLoaderItem strong')].map(item => item.textContent)
      expect(itemNames).toContain('Docker 巡检')
      expect(itemNames).toContain('系统巡检')
    } finally {
      root.unmount()
      host.remove()
    }
  })
})

describe('workflow recovery panel', () => {
  const pendingDetail = {
    record: {
      id: 'run-pending-1',
      kind: 'workflow',
      target: 'demo.deploy',
      status: 'pending',
      started_at: '2026-10-23T02:00:00Z',
      error: '健康检查失败',
      pending: {status: 'pending', reason: '节点 verify 执行失败，配置已变更，需要人工确认或回滚', step_id: 'verify', since: '2026-10-23T02:01:00Z'},
      snapshot: {
        workflow: {
          stages: [{id: 'change', name: '变更'}, {id: 'verify', name: '验证'}],
          nodes: [
            {id: 'deploy', type: 'tool', name: '下发配置', stage_id: 'change', tool: 'config.deploy.push'},
            {id: 'verify', type: 'tool', name: '健康检查', stage_id: 'verify', tool: 'config.deploy.health'}
          ]
        }
      },
      steps: [
        {id: 'deploy', type: 'tool', tool: 'config.deploy.push', status: 'succeeded', host_id: 'host-a', host_name: 'A 机'},
        {id: 'verify', type: 'tool', tool: 'config.deploy.health', status: 'failed', host_id: 'host-a', host_name: 'A 机', error: 'HTTP 500'}
      ],
      backups: [{node_id: 'deploy', target_path: '/etc/app.conf', backup_path: '/etc/app.conf.bak.1', sha256: 'abc', status: 'updated', host_id: 'host-a', host_name: 'A 机'}],
      attempts: [{
        id: 'attempt-1-rollback',
        kind: 'rollback',
        status: 'failed',
        started_at: '2026-10-23T02:05:00Z',
        ended_at: '2026-10-23T02:05:10Z',
        error: '配置已回滚但健康检查失败',
        detail: '回滚后健康检查失败',
        log_dir: 'attempts/attempt-1-rollback'
      }],
      notifications: [{url: 'https://hooks.example.com/ops', event: 'pending', status: 'failed', attempts: 1, error: '连接超时', updated_at: '2026-10-23T02:01:05Z'}]
    },
    logs: {
      items: [{id: 'verify', kind: 'workflow_step', title: '健康检查', status: 'failed', stderr: 'HTTP 500'}],
      attempts: {['attempt-1-rollback']: {summary: '回滚后健康检查失败\n', stdout: {attempt: 'rollback output'}, stderr: {attempt: 'rollback error'}}}
    }
  }

  it('renders pending evidence, backups, attempts and notification retry', () => {
    const markup = renderToStaticMarkup(React.createElement(RecoveryPanel, {
      detail: pendingDetail,
      onCheck: () => {},
      onRollback: () => {},
      onResolve: () => {},
      onRetryNotification: () => {}
    }))
    expect(markup).toContain('待处理')
    expect(markup).toContain('验证（verify）')
    expect(markup).toContain('健康检查（verify）')
    expect(markup).toContain('A 机')
    expect(markup).toContain('/etc/app.conf')
    expect(markup).toContain('/etc/app.conf.bak.1')
    expect(markup).toContain('重试检查')
    expect(markup).toContain('回滚配置')
    expect(markup).toContain('结束处理')
    expect(markup).toContain('回滚后健康检查失败')
    expect(markup).toContain('rollback error')
    expect(markup).toContain('重试通知')
  })

  it('explains why rollback is unavailable when no backup exists', () => {
    const detail = {
      record: {
        ...pendingDetail.record,
        backups: [{node_id: 'deploy', target_path: '/etc/new.conf', backup_path: '', status: 'created'}],
        attempts: [],
        notifications: []
      },
      logs: {}
    }
    const markup = renderToStaticMarkup(React.createElement(RecoveryPanel, {
      detail,
      onCheck: () => {},
      onRollback: () => {},
      onResolve: () => {},
      onRetryNotification: () => {}
    }))
    expect(markup).toContain('新建文件')
    expect(markup).toContain('disabled')
  })

  it('shows rolled back state while keeping the original failure conclusion', () => {
    const detail = {
      record: {
        ...pendingDetail.record,
        status: 'rolled_back',
        pending: {status: 'rolled_back', reason: pendingDetail.record.pending.reason, step_id: 'verify', resolution: '配置已回滚，服务已恢复'},
        attempts: []
      },
      logs: {items: pendingDetail.logs.items}
    }
    const markup = renderToStaticMarkup(React.createElement(RecoveryPanel, {
      detail,
      onCheck: () => {},
      onRollback: () => {},
      onResolve: () => {},
      onRetryNotification: () => {}
    }))
    expect(markup).toContain('已回滚，服务已恢复')
    expect(markup).toContain('原变更失败的结论仍保留')
    expect(markup).not.toContain('重试检查')
  })

  it('blocks rollback when any target has no safe rollback metadata', () => {
    const detail = {
      ...pendingDetail,
      record: {...pendingDetail.record, backups: [...pendingDetail.record.backups, {node_id: 'other', target_path: '/etc/other', backup_path: '/backup/other', status: 'updated'}]}
    }
    const markup = renderToStaticMarkup(React.createElement(RecoveryPanel, {detail}))
    expect(markup).toContain('缺少路径或下发后的 SHA256')
    expect(markup).toMatch(/<button[^>]*disabled=""[^>]*>回滚配置<\/button>/)
  })

  it('shows persisted file restoration while allowing recovery to continue', () => {
    const detail = {
      ...pendingDetail,
      record: {...pendingDetail.record, backups: pendingDetail.record.backups.map(backup => ({...backup, restored_sha256: 'restored-content'}))}
    }
    const markup = renderToStaticMarkup(React.createElement(RecoveryPanel, {detail}))
    expect(markup).toContain('文件已还原，待完成服务与健康检查')
    expect(markup).toContain('restored-content')
    expect(markup).toMatch(/<button(?:(?!disabled=)[^>])*>回滚配置<\/button>/)
  })

  it('normalizes node id lists and prunes empty recovery policy', () => {
    expect(splitNodeIDList(' deploy, restart，，verify deploy ')).toEqual(['deploy', 'restart', 'verify'])
    expect(pruneWorkflowRecovery({pending_on_failure: false, change_node_ids: [' '], notification: {url: ''}})).toBeNull()
    expect(pruneWorkflowRecovery({pending_on_failure: true, notification: {url: ' https://hook ', events: ['unknown', 'failed']}})).toEqual({
      pending_on_failure: true,
      notification: {url: 'https://hook', events: ['failed']}
    })
    expect(workflowNotificationSummary({url: 'https://hook', events: []})).toContain('成功、失败、待处理、已回滚')
  })
})
