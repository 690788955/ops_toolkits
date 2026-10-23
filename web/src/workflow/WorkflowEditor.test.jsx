// @vitest-environment jsdom

import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import React from 'react'
import {createRoot} from 'react-dom/client'
import {act} from 'react'
import WorkflowEditor from './WorkflowEditor.jsx'
import {DRAFT_WINDOW_SAVED_KEY, readDraftWindowSaved, readDraftWindowTransfer, writeDraftWindowSaved, writeDraftWindowTransfer} from './draftWindow.js'

const catalog = {
  name: 'Ops Toolkit',
  categories: [
    {id: 'docker', name: 'Docker'},
    {id: 'system', name: '系统管理'},
    {id: 'alpha', name: 'Alpha 类'},
    {id: 'beta', name: 'Beta 类'}
  ],
  tools: [],
  workflows: [
    {
      id: 'demo.shared',
      name: '共享流程',
      category: 'docker',
      shared_with: ['multi.plugin'],
      nodes: [],
      source: {type: 'plugin', plugin_id: 'docker.exec', plugin_name: 'Docker 插件'}
    }
  ],
  plugins: [
    {id: 'docker.exec', name: 'Docker 插件', version: '1.0.0', categories: [{id: 'docker', name: 'Docker'}]},
    {
      id: 'multi.plugin',
      name: '多分类插件',
      version: '1.0.0',
      categories: [{id: 'alpha', name: 'Alpha 类'}, {id: 'beta', name: 'Beta 类'}]
    }
  ]
}

let mounted = []

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn(async () => ({
    ok: true,
    status: 200,
    json: async () => ({status: 'ok', data: {}}),
    text: async () => ''
  })))
  if (typeof window !== 'undefined' && !window.ResizeObserver) {
    window.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  }
})

afterEach(() => {
  for (const {host, root} of mounted) {
    act(() => root.unmount())
    host.remove()
  }
  mounted = []
  vi.unstubAllGlobals()
  window.localStorage?.clear()
})

async function mountEditor() {
  return mountEditorWithCatalog(catalog)
}

async function mountEditorWithCatalog(editorCatalog, extraProps = {}) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  mounted.push({host, root})
  await act(async () => {
    root.render(<WorkflowEditor catalog={editorCatalog} setResult={() => {}} refreshCatalog={async () => {}} {...extraProps} />)
  })
  return host
}

async function openWorkflowNamePopover(host) {
  const trigger = host.querySelector('.workflowNameTrigger')
  if (!trigger) throw new Error('未找到工作流名称入口')
  await act(async () => {
    trigger.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
  })
  const popover = host.querySelector('.workflowNamePopover')
  if (!popover) throw new Error('未打开工作流选择器')
  return popover
}

function ownerSelect(host) {
  return Array.from(host.querySelectorAll('select'))
    .find(select => Array.from(select.options).some(option => option.textContent === '全局工作流'))
}

async function setSelectValue(select, value) {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype, 'value').set
  await act(async () => {
    setter.call(select, value)
    select.dispatchEvent(new Event('change', {bubbles: true}))
  })
}

async function clickWorkflowListItem(popover, id) {
  const item = Array.from(popover.querySelectorAll('.workflowLoaderItem'))
    .find(button => button.dataset.workflowId === id)
  if (!item) throw new Error(`未找到工作流列表项：${id}`)
  await act(async () => {
    item.click()
  })
}

async function changeOwner(host, value) {
  const select = ownerSelect(host)
  if (!select) throw new Error('未找到归属选择器')
  await setSelectValue(select, value)
}

function hasCategorySelect(host) {
  return Array.from(host.querySelectorAll('select'))
    .some(select => Array.from(select.options).some(option => option.textContent === '全局（跨分类）'))
}

async function openSaveModal(host) {
  const saveButton = Array.from(host.querySelectorAll('button')).find(button => button.textContent === '保存')
  if (!saveButton) throw new Error('未找到保存按钮')
  await act(async () => {
    saveButton.dispatchEvent(new MouseEvent('click', {bubbles: true}))
  })
  const dialog = host.querySelector('[role="dialog"][aria-label="保存工作流"]')
  if (!dialog) throw new Error('未打开保存弹窗')
  return dialog
}

describe('WorkflowEditor 归属与分类联动', () => {
  it('插件只有一个分类时隐藏分类字段', async () => {
    const host = await mountEditor()
    const dialog = await openSaveModal(host)
    expect(hasCategorySelect(host)).toBe(true)

    await changeOwner(host, 'docker.exec')

    expect(hasCategorySelect(host)).toBe(false)
    const hasCategoryField = Array.from(dialog.querySelectorAll('label > span'))
      .some(span => span.textContent === '分类')
    expect(hasCategoryField).toBe(false)
  })

  it('插件声明多个分类时只展示该插件的分类选项', async () => {
    const host = await mountEditor()
    await openSaveModal(host)

    await changeOwner(host, 'multi.plugin')

    const categorySelect = Array.from(host.querySelectorAll('label'))
      .find(label => label.querySelector('span')?.textContent === '插件分类')
      ?.querySelector('select')
    expect(categorySelect).toBeTruthy()
    expect(Array.from(categorySelect.options).map(option => option.textContent)).toEqual(['Alpha 类', 'Beta 类'])
    expect(hasCategorySelect(host)).toBe(false)
  })

  it('切回全局工作流后恢复完整分类选择器', async () => {
    const host = await mountEditor()
    await openSaveModal(host)
    await changeOwner(host, 'docker.exec')
    expect(hasCategorySelect(host)).toBe(false)

    await changeOwner(host, 'user.workflows')

    expect(hasCategorySelect(host)).toBe(true)
  })
})

describe('WorkflowEditor 保存弹窗', () => {
  it('顶部栏只保留摘要，元数据字段收进保存弹窗', async () => {
    const host = await mountEditor()
    const topBar = host.querySelector('.workflowTopBar')
    expect(topBar).toBeTruthy()
    expect(topBar.querySelector('.workflowNameTrigger').textContent).toContain('未命名工作流')
    const topBarLabels = Array.from(topBar.querySelectorAll('label > span')).map(span => span.textContent)
    expect(topBarLabels).not.toContain('工作流 ID')
    expect(topBarLabels).not.toContain('名称')

    const popover = await openWorkflowNamePopover(host)
    expect(popover.textContent).toContain('全局工作流 · 全局')

    const dialog = await openSaveModal(host)
    const labels = Array.from(dialog.querySelectorAll('label > span')).map(span => span.textContent)
    expect(labels).toEqual(expect.arrayContaining(['工作流 ID', '名称', '描述', '归属', '分类']))
    const advanced = dialog.querySelector('details.workflowSaveAdvanced')
    expect(advanced).toBeTruthy()
    expect(advanced.open).toBe(false)
  })

  it('保存前检查错误显示在弹窗内', async () => {
    const host = await mountEditor()
    const dialog = await openSaveModal(host)

    await clickButtonByText(dialog, '保存')

    expect(dialog.textContent).toContain('保存前检查未通过')
    expect(dialog.textContent).toContain('请至少添加一个工作流节点')
  })

  it('保存成功后即使目录刷新失败也会关闭弹窗', async () => {
    const host = await mountEditorWithHosts({
      hosts: [],
      refreshCatalog: async () => { throw new Error('catalog unavailable') },
      workflow: {
        id: 'save-flow',
        name: '保存流程',
        category: 'docker',
        parameters: [],
        nodes: [{id: 'first', tool: 'demo.hello'}],
        edges: []
      }
    })
    const dialog = await openSaveModal(host)

    await clickButtonByText(dialog, '保存')

    expect(host.querySelector('[role="dialog"][aria-label="保存工作流"]')).toBeNull()
  })

  it('已保存工作流在弹窗中锁定 ID', async () => {
    const host = await mountEditorWithHosts({
      hosts: [],
      workflow: {
        id: 'fixed-flow',
        name: '固定流程',
        category: 'docker',
        parameters: [],
        nodes: [{id: 'first', tool: 'demo.hello'}],
        edges: []
      }
    })
    const dialog = await openSaveModal(host)
    const idLabel = Array.from(dialog.querySelectorAll('label'))
      .find(label => label.querySelector('span')?.textContent === '工作流 ID')
    const input = idLabel?.querySelector('input')
    expect(input?.value).toBe('fixed-flow')
    expect(input?.readOnly).toBe(true)
  })
})

describe('WorkflowEditor 共享可见多选', () => {
  it('共享入口排除当前归属插件，切换归属时剔除新归属', async () => {
    const host = await mountEditor()
    await openWorkflowNamePopover(host)
    await openSaveModal(host)
    await changeOwner(host, 'docker.exec')

    const chips = () => Array.from(host.querySelectorAll('.workflowShareChip'))
    expect(chips().map(chip => chip.textContent)).toEqual(['多分类插件'])

    await act(async () => {
      chips()[0].dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })
    expect(chips()[0].getAttribute('aria-pressed')).toBe('true')

    await changeOwner(host, 'multi.plugin')

    expect(chips().map(chip => chip.textContent)).toEqual(['Docker 插件'])
    expect(chips()[0].getAttribute('aria-pressed')).toBe('false')
  })

  it('共享工作流会出现在目标插件分类筛选结果中并标注来源', async () => {
    const host = await mountEditor()
    const popover = await openWorkflowNamePopover(host)
    const findSharedItem = () => Array.from(popover.querySelectorAll('.workflowLoaderItem'))
      .find(item => item.textContent.includes('共享流程'))

    expect(findSharedItem().querySelector('.workflowLoaderTag.muted')?.textContent).not.toBe('共享')

    const filter = popover.querySelector('select[aria-label="按分类筛选工作流"]')
    await setSelectValue(filter, 'alpha')

    const sharedItem = findSharedItem()
    expect(sharedItem).toBeTruthy()
    expect(Array.from(sharedItem.querySelectorAll('.workflowLoaderTag.muted')).map(tag => tag.textContent)).toContain('共享')
  })
})

async function mountEditorWithHosts({hosts, workflow, refreshCatalog = async () => {}}) {
  const originalFetch = globalThis.fetch
  globalThis.fetch = vi.fn(async path => {
    if (path === `/api/workflows/${workflow.id}`) {
      return {ok: true, status: 200, json: async () => ({status: 'ok', data: {config: workflow}}), text: async () => ''}
    }
    return {ok: true, status: 200, json: async () => ({status: 'ok', data: {}}), text: async () => ''}
  })
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  mounted.push({host, root})
  await act(async () => {
    root.render(
      <WorkflowEditor
        catalog={{...catalog, tools: [{id: 'demo.hello', name: '问候', category: 'docker'}], workflows: [workflow]}}
        hosts={hosts}
        setResult={() => {}}
        refreshCatalog={refreshCatalog}
      />
    )
  })
  const popover = await openWorkflowNamePopover(host)
  await clickWorkflowListItem(popover, workflow.id)
  await act(async () => {})
  globalThis.fetch = originalFetch
  return host
}

async function clickButtonByAriaLabel(host, label) {
  const button = host.querySelector(`button[aria-label="${label}"]`)
  if (!button) throw new Error(`未找到按钮：${label}`)
  await act(async () => {
    button.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
  })
}

async function clickButtonByText(host, text) {
  const button = Array.from(host.querySelectorAll('button')).find(item => item.textContent === text)
  if (!button) throw new Error(`未找到按钮：${text}`)
  await act(async () => {
    button.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
  })
}

describe('WorkflowEditor 阶段批量移动', () => {
  const workflow = {
    id: 'batch-flow',
    name: '批量移动流程',
    category: 'docker',
    parameters: [],
    stages: [
      {id: 'prepare', name: '准备'},
      {id: 'change', name: '变更'},
      {id: 'verify', name: '验证'}
    ],
    nodes: [
      {id: 'inspect', name: '检查', tool: 'demo.hello', stage_id: 'prepare', order: 10},
      {id: 'deploy', name: '发布', tool: 'demo.hello', stage_id: 'change', order: 10},
      {id: 'smoke', name: '冒烟', tool: 'demo.hello', stage_id: 'verify', order: 10}
    ],
    edges: []
  }

  function stageColumn(host, name) {
    return Array.from(host.querySelectorAll('.workflowStageColumn'))
      .find(column => column.getAttribute('aria-label')?.endsWith(`：${name}`))
  }

  async function selectNodes(host, ids) {
    const pane = host.querySelector('.react-flow__pane')
    const nodes = ids.map(id => host.querySelector(`.react-flow__node[data-id="${id}"]`))
    if (!pane || nodes.some(node => !node)) throw new Error('未渲染节点')
    await act(async () => {
      pane.dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })
    // 按住多选键再点节点，等价于 React Flow 的 Ctrl/⌘ 点选，完成后在结束事件里松开。
    window.dispatchEvent(new KeyboardEvent('keydown', {key: 'Control', bubbles: true}))
    for (const node of nodes) {
      await act(async () => {
        node.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
      })
    }
    window.dispatchEvent(new KeyboardEvent('keyup', {key: 'Control', bubbles: true}))
  }

  it('多选后可以把选中的步骤一次性移入目标阶段', async () => {
    const host = await mountEditorWithHosts({hosts: [], workflow})
    await selectNodes(host, ['inspect', 'smoke'])
    const batchButton = stageColumn(host, '变更').querySelector('button[aria-label="把选中的 2 个步骤移入阶段 变更"]')
    expect(batchButton).toBeTruthy()
    await act(async () => {
      batchButton.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
    })
    const nodesIn = name => Array.from(stageColumn(host, name).parentElement.parentElement.querySelectorAll('.react-flow__node'))
      .map(node => node.dataset.id)
    const [prepareX, changeX, verifyX] = ['准备', '变更', '验证'].map(name => stageColumn(host, name).style.left)
    // 移动后节点按阶段列重新排布：左边界落在目标列范围内的节点属于该列。
    const columnOf = id => Array.from(host.querySelectorAll('.workflowStageColumn')).find(column => {
      const left = parseFloat(column.style.left)
      const width = parseFloat(column.style.width)
      const nodeLeft = parseFloat(host.querySelector(`.react-flow__node[data-id="${id}"]`)?.style.transform?.match(/translate\(([-\d.]+)px/)?.[1] || '0')
      return nodeLeft >= left && nodeLeft <= left + width
    })?.getAttribute('aria-label')
    expect(columnOf('inspect')).toContain('变更')
    expect(columnOf('smoke')).toContain('变更')
    expect(columnOf('deploy')).toContain('变更')
    expect(parseFloat(prepareX) < parseFloat(changeX) && parseFloat(changeX) < parseFloat(verifyX)).toBe(true)
    expect(nodesIn('准备')).toEqual([])
  })

  it('选中节点后在阶段头部弹出批量入口，并可从下拉选择目标阶段', async () => {
    const host = await mountEditorWithHosts({hosts: [], workflow})
    await selectNodes(host, ['deploy'])
    const prepareColumn = stageColumn(host, '准备')
    const toggle = prepareColumn.querySelector('.workflowStageBatchTarget')
    expect(toggle).toBeTruthy()
    await act(async () => {
      toggle.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
    })
    const item = Array.from(prepareColumn.querySelectorAll('.workflowStageBatchMenu button'))
      .find(button => button.textContent === '变更')
    expect(item).toBeTruthy()
    await act(async () => {
      item.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
    })
    expect(await stageColumn(host, '变更')).toBeTruthy()
  })
})

describe('WorkflowEditor 执行机器步骤', () => {
  const hosts = [
    {id: 'host-b', name: 'B 机', user: 'root', address: '10.0.0.2', port: 22, enabled: true, status: 'online'},
    {id: 'host-c', name: 'C 机', user: 'root', address: '10.0.0.3', port: 22, enabled: true, status: 'online'}
  ]
  const workflow = {
    id: 'target-flow',
    name: '选择机器流程',
    category: 'docker',
    parameters: [],
    nodes: [{id: 'first', tool: 'demo.hello', host: 'host-b'}],
    edges: []
  }

  it('运行抽屉直接展示执行机器并在摘要中回显所选机器', async () => {
    const host = await mountEditorWithHosts({hosts, workflow})

    await clickButtonByAriaLabel(host, '运行工作流')
    const drawer = host.querySelector('[role="dialog"][aria-label="运行准备"]')
    expect(drawer).toBeTruthy()
    expect(drawer.textContent).toContain('工作流参数')
    expect(drawer.textContent).toContain('执行机器')
    expect(drawer.textContent).toContain('运行摘要')
    const options = Array.from(drawer.querySelectorAll('.workflowTargetOption'))
    expect(options.map(option => option.querySelector('strong').textContent)).toEqual(['跟随节点配置', '本机', 'B 机', 'C 机'])

    await act(async () => {
      options[3].dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })
    expect(options[3].getAttribute('aria-checked')).toBe('true')
    expect(drawer.textContent).toContain('1 个节点已单独指定')
    expect(drawer.textContent).toContain('单独指定机器')
    expect(drawer.textContent).toContain('first')
    expect(drawer.textContent).toContain('C 机（root@10.0.0.3:22）')
  })

  it('勾选覆盖后提示全部节点都会改到所选机器', async () => {
    const host = await mountEditorWithHosts({hosts, workflow})

    await clickButtonByAriaLabel(host, '运行工作流')
    const localOption = Array.from(host.querySelectorAll('.workflowTargetOption'))
      .find(option => option.querySelector('strong').textContent === '本机')
    await act(async () => {
      localOption.dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })

    const checkbox = host.querySelector('.workflowTargetOverride input[type="checkbox"]')
    expect(checkbox).toBeTruthy()
    await act(async () => {
      checkbox.click()
    })
    expect(host.textContent).toContain('全部 1 个节点都会改到「本机」执行')
  })

  it('完成一次运行后，再次提交时立即锁定编辑并清除旧日志', async () => {
    const host = await mountEditorWithHosts({hosts, workflow})
    let releaseRun
    let submissions = 0
    const response = body => ({ok: true, json: async () => body, text: async () => ''})
    const record = {id: 'run-1', target: workflow.id, status: 'succeeded', steps: [
      {id: 'first', type: 'tool', status: 'succeeded', host_id: 'host-b', host_name: 'B 机'}
    ]}
    vi.stubGlobal('fetch', vi.fn(async path => {
      if (path.includes('/run?')) {
        submissions += 1
        if (submissions === 2) return new Promise(resolve => { releaseRun = resolve })
        return response({id: 'run-1', target: workflow.id, status: 'succeeded'})
      }
      return response({data: {record, logs: {items: []}}})
    }))

    await clickButtonByAriaLabel(host, '运行工作流')
    await clickButtonByText(host.querySelector('[aria-label="运行准备"]'), '开始运行')
    expect(host.querySelector('.workflowRunDock').textContent).toContain('执行机器：B 机')
    expect(host.querySelector('button[aria-label="运行工作流"]')).toBeTruthy()

    await clickButtonByAriaLabel(host, '运行工作流')
    await clickButtonByText(host.querySelector('[aria-label="运行准备"]'), '开始运行')
    const saveButton = Array.from(host.querySelectorAll('.workflowTopBar button')).find(button => button.textContent === '保存')
    expect(saveButton.disabled).toBe(true)
    expect(host.querySelector('.workflowTopBar button[aria-label="运行工作流"]')).toBeNull()
    expect(host.querySelector('.workflowRunDock code[title="运行 ID"]')).toBeNull()
    expect(host.querySelector('button[aria-label="取消当前工作流运行"]').textContent).toBe('启动中...')

    await act(async () => { releaseRun(response({id: 'run-2', target: workflow.id, status: 'succeeded'})) })
  })
})

describe('WorkflowEditor 恢复运行隔离', () => {
  function detailFor(id, status = 'pending') {
    return {
      record: {
        id,
        kind: 'workflow',
        target: `workflow-${id}`,
        status,
        pending: {status: 'pending', step_id: 'check', reason: `failure-${id}`},
        snapshot: {workflow: {nodes: [{id: 'check', type: 'http_request'}]}},
        steps: [{id: 'check', type: 'http_request', status: 'failed'}],
        backups: [{node_id: 'deploy', target_path: `/etc/${id}`, backup_path: `/backup/${id}`, sha256: 'new-content', status: 'updated'}],
        notifications: [{event: 'pending', status: 'failed', attempts: 1, url: `https://hook/${id}`}]
      },
      logs: {items: []}
    }
  }

  const response = data => ({ok: true, status: 200, json: async () => ({data}), text: async () => ''})

  function installRecoveryFetch(handler, ids = ['run-A', 'run-B']) {
    const request = vi.fn(async path => {
      if (path === '/api/runs?status=pending&limit=0') {
        return response({runs: ids.map(id => detailFor(id).record)})
      }
      return handler(path)
    })
    vi.stubGlobal('fetch', request)
    return request
  }

  async function openPending(host, index) {
    const button = host.querySelectorAll('.workflowPendingBannerActions button')[index]
    await act(async () => { button.click() })
  }

  async function closeRecovery(host) {
    await act(async () => { host.querySelector('.workflowRecoveryDrawer .modalClose').click() })
  }

  it('忽略已关闭抽屉的迟到读取响应，恢复请求仍绑定当前运行', async () => {
    let releaseA
    const request = installRecoveryFetch(path => {
      if (path === '/api/runs/run-A') return new Promise(resolve => { releaseA = resolve })
      if (path === '/api/runs/run-B/recovery/check') return response(detailFor('run-B', 'succeeded'))
      if (path === '/api/runs/run-B') return response(detailFor('run-B'))
      return response({})
    })
    const host = await mountEditor()
    await openPending(host, 0)
    await closeRecovery(host)
    await openPending(host, 1)

    await act(async () => { releaseA(response(detailFor('run-A'))) })

    const drawer = host.querySelector('.workflowRecoveryDrawer')
    expect(drawer.textContent).toContain('/etc/run-B')
    expect(drawer.textContent).not.toContain('/etc/run-A')
    await clickButtonByText(drawer, '重试检查')
    expect(request.mock.calls.some(([path]) => path === '/api/runs/run-B/recovery/check')).toBe(true)
    expect(request.mock.calls.some(([path]) => path === '/api/runs/run-A/recovery/check')).toBe(false)
  })

  it('通知重试的迟到响应不会覆盖新打开的运行', async () => {
    let releaseNotification
    installRecoveryFetch(path => {
      if (path === '/api/runs/run-A/notifications/0/retry') return new Promise(resolve => { releaseNotification = resolve })
      if (path === '/api/runs/run-A') return response(detailFor('run-A'))
      if (path === '/api/runs/run-B') return response(detailFor('run-B'))
      return response({})
    })
    const host = await mountEditor()
    await openPending(host, 0)
    await clickButtonByText(host.querySelector('.workflowRecoveryDrawer'), '重试通知')
    await closeRecovery(host)
    await openPending(host, 1)

    await act(async () => { releaseNotification(response(detailFor('run-A'))) })

    const drawer = host.querySelector('.workflowRecoveryDrawer')
    expect(drawer.textContent).toContain('/etc/run-B')
    expect(drawer.textContent).not.toContain('/etc/run-A')
    expect(drawer.textContent).not.toContain('通知已重新发送')
    expect(Array.from(drawer.querySelectorAll('button')).find(button => button.textContent === '重试通知').disabled).toBe(false)
  })

  it('同一渲染内的重复恢复点击只提交一次，迟到动作响应不会覆盖另一运行', async () => {
    let releaseCheck
    const request = installRecoveryFetch(path => {
      if (path === '/api/runs/run-A/recovery/check') return new Promise(resolve => { releaseCheck = resolve })
      if (path === '/api/runs/run-A') return response(detailFor('run-A'))
      if (path === '/api/runs/run-B') return response(detailFor('run-B'))
      return response({})
    })
    const host = await mountEditor()
    await openPending(host, 0)
    const button = Array.from(host.querySelector('.workflowRecoveryDrawer').querySelectorAll('button')).find(item => item.textContent === '重试检查')
    await act(async () => { button.click(); button.click() })
    expect(request.mock.calls.filter(([path]) => path === '/api/runs/run-A/recovery/check')).toHaveLength(1)
    await openPending(host, 1)
    await act(async () => { releaseCheck(response(detailFor('run-A', 'succeeded'))) })
    expect(host.querySelector('.workflowRecoveryDrawer').textContent).toContain('/etc/run-B')
    expect(host.querySelector('.workflowRecoveryDrawer').textContent).not.toContain('/etc/run-A')
  })

  it('拒绝与入口运行 ID 不一致的恢复详情', async () => {
    installRecoveryFetch(() => response(detailFor('different-run')))
    const host = await mountEditor()
    await openPending(host, 0)

    const drawer = host.querySelector('.workflowRecoveryDrawer')
    expect(drawer.textContent).toContain('运行记录与当前恢复入口不一致')
    expect(Array.from(drawer.querySelectorAll('button')).some(button => button.textContent === '重试检查')).toBe(false)
  })

  it('展开更多后可以打开第五条待处理记录', async () => {
    const ids = ['run-A', 'run-B', 'run-C', 'run-D', 'run-E']
    installRecoveryFetch(path => response(detailFor(path.split('/').pop())), ids)
    const host = await mountEditor()
    await clickButtonByText(host, '更多（2）')

    expect(host.querySelectorAll('.workflowPendingBannerActions button')).toHaveLength(6)
    await openPending(host, 4)
    expect(host.querySelector('.workflowRecoveryDrawer').textContent).toContain('/etc/run-E')
  })

  it.each(['succeeded', 'failed'])('终态 %s 的失败通知可从运行日志单独重试', async status => {
    const workflow = {id: 'notify-flow', name: '通知流程', parameters: [], nodes: [{id: 'first', tool: 'demo.hello'}], edges: []}
    const host = await mountEditorWithHosts({hosts: [], workflow})
    const detail = detailFor('run-notify', status)
    delete detail.record.pending
    const request = vi.fn(async path => {
      if (path.includes('/run?')) {
        return {ok: true, status: 200, json: async () => ({id: 'run-notify', status}), text: async () => ''}
      }
      if (path === '/api/runs/run-notify/notifications/0/retry') {
        return response({...detail, record: {...detail.record, notifications: [{...detail.record.notifications[0], status: 'sent', attempts: 2}]}})
      }
      if (path === '/api/runs/run-notify') return response(detail)
      return response({runs: []})
    })
    vi.stubGlobal('fetch', request)

    await clickButtonByAriaLabel(host, '运行工作流')
    await clickButtonByText(host.querySelector('[aria-label="运行准备"]'), '开始运行')
    await clickButtonByText(host.querySelector('.workflowRunDock'), '恢复与通知')
    await clickButtonByText(host.querySelector('.workflowRecoveryDrawer'), '重试通知')

    expect(host.querySelector('.workflowRecoveryDrawer').textContent).toContain('已发送')
    expect(request.mock.calls.filter(([path]) => path.includes('/run?'))).toHaveLength(1)
    expect(request.mock.calls.some(([path]) => path === '/api/runs/run-notify/notifications/0/retry')).toBe(true)
  })

  it.each(['pending', 'rolled_back', 'succeeded'])('有恢复记录的 %s 运行不提供旧节点重跑入口', async status => {
    const workflow = {id: 'recovery-flow', name: '恢复流程', parameters: [], nodes: [{id: 'first', tool: 'demo.hello'}], edges: []}
    const host = await mountEditorWithHosts({hosts: [], workflow})
    const detail = detailFor('run-recovery', status)
    detail.record.steps = [{id: 'first', type: 'tool', status: 'failed'}]
    vi.stubGlobal('fetch', vi.fn(async path => {
      if (path.includes('/run?')) return {ok: true, status: 200, json: async () => ({id: 'run-recovery', status}), text: async () => ''}
      if (path === '/api/runs/run-recovery') return response(detail)
      return response({runs: []})
    }))
    await clickButtonByAriaLabel(host, '运行工作流')
    await clickButtonByText(host.querySelector('[aria-label="运行准备"]'), '开始运行')

    expect(host.querySelector('.workflowRunDock').textContent).toContain('恢复与通知')
    expect(host.querySelector('.react-flow__node[data-id="first"]')).toBeTruthy()
    expect(host.querySelector('.nodeRerun')).toBeNull()
  })
})

describe('WorkflowEditor 画布撤销重做', () => {
  const workflow = {
    id: 'history-flow',
    name: '历史流程',
    category: 'docker',
    parameters: [],
    stages: [{id: 'prepare', name: '准备'}],
    nodes: [
      {id: 'prepare', name: '准备', tool: 'demo.hello'},
      {id: 'deploy', name: '部署', tool: 'demo.hello'}
    ],
    edges: [{from: 'prepare', to: 'deploy'}]
  }

  function topBarButton(host, label) {
    return Array.from(host.querySelectorAll('.workflowTopBar button')).find(button => button.textContent === label)
  }

  function nodeCount(host) {
    return host.querySelectorAll('.react-flow__node').length
  }

  it('键盘删除节点后可以撤销回来，连线也一并恢复', async () => {
    // React Flow 按 Backspace/Delete 删除会同时移除节点和相关连线，撤销必须把它们一起恢复。
    const host = await mountEditorWithHosts({hosts: [], workflow})
    expect(nodeCount(host)).toBe(2)
    const node = host.querySelector('.react-flow__node[data-id="deploy"]')
    await act(async () => { node.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true})) })
    await act(async () => { node.dispatchEvent(new KeyboardEvent('keydown', {key: 'Backspace', code: 'Backspace', bubbles: true, cancelable: true})) })
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 350)) })
    // jsdom 里 React Flow 没接管键盘删除时只验证删除后的撤销入口仍然正确。
    if (nodeCount(host) === 1) {
      await act(async () => { topBarButton(host, '撤销').click() })
      expect(nodeCount(host)).toBe(2)
      return
    }
    await act(async () => { topBarButton(host, '撤销').click() })
    expect(nodeCount(host)).toBe(2)
  })

  it('连续撤销可以一步步退回，重做再逐步恢复', async () => {
    const host = await mountEditorWithHosts({hosts: [], workflow})
    // 阶段名输入是能稳定触发“每一步一次历史”的编辑入口。
    const typed = host.querySelector('input.workflowStageNameInput')
    expect(typed).toBeTruthy()
    await setInputValue(typed, '准备一')
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 350)) })
    await setInputValue(host.querySelector('input.workflowStageNameInput'), '准备二')
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 350)) })

    await act(async () => { topBarButton(host, '撤销').click() })
    expect(host.querySelector('input.workflowStageNameInput').value).toBe('准备一')
    await act(async () => { topBarButton(host, '撤销').click() })
    expect(host.querySelector('input.workflowStageNameInput').value).toBe('准备')
    await act(async () => { topBarButton(host, '重做').click() })
    expect(host.querySelector('input.workflowStageNameInput').value).toBe('准备一')
  })

  it('撤销一步后继续撤销会一直退回到加载时的原始状态', async () => {
    const host = await mountEditorWithHosts({hosts: [], workflow})
    const name = () => host.querySelector('input.workflowStageNameInput').value
    for (const value of ['一', '二', '三', '四']) {
      await setInputValue(host.querySelector('input.workflowStageNameInput'), value)
      await act(async () => { await new Promise(resolve => setTimeout(resolve, 350)) })
    }
    expect(name()).toBe('四')

    const visited = []
    for (let guard = 0; guard < 12; guard += 1) {
      visited.push(name())
      if (topBarButton(host, '撤销').disabled) break
      await act(async () => { topBarButton(host, '撤销').click() })
    }
    expect(new Set(visited)).toEqual(new Set(['四', '三', '二', '一', '准备']))
    expect(name()).toBe('准备')
    expect(nodeCount(host)).toBe(2)
  })

  it('连续重做会一步步恢复到撤销前的最新状态', async () => {
    const host = await mountEditorWithHosts({hosts: [], workflow})
    const name = () => host.querySelector('input.workflowStageNameInput').value
    for (const value of ['一', '二', '三']) {
      await setInputValue(host.querySelector('input.workflowStageNameInput'), value)
      await act(async () => { await new Promise(resolve => setTimeout(resolve, 350)) })
    }
    await act(async () => { topBarButton(host, '撤销').click() })
    await act(async () => { topBarButton(host, '撤销').click() })
    expect(name()).toBe('一')

    await act(async () => { topBarButton(host, '重做').click() })
    expect(name()).toBe('二')
    await act(async () => { topBarButton(host, '重做').click() })
    expect(name()).toBe('三')
    expect(topBarButton(host, '重做').disabled).toBe(true)

    // 重做过的步骤要能再撤销回去，两个方向都要能逐步走。
    await act(async () => { topBarButton(host, '撤销').click() })
    expect(name()).toBe('二')
  })

  it('重新加载工作流后连续撤销不会越过加载时的初始状态', async () => {
    const host = await mountEditorWithHosts({hosts: [], workflow})
    await setInputValue(host.querySelector('input.workflowStageNameInput'), '改一')
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 350)) })
    await setInputValue(host.querySelector('input.workflowStageNameInput'), '改二')
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 350)) })
    expect(host.querySelector('input.workflowStageNameInput').value).toBe('改二')
    expect(host.querySelector('.workflowSaveState').textContent).not.toBe('已保存')
    let loads = 0
    vi.stubGlobal('fetch', vi.fn(async path => {
      if (path === '/api/workflows/history-flow') {
        loads += 1
        return {ok: true, status: 200, json: async () => ({data: {config: workflow}}), text: async () => ''}
      }
      return {ok: true, status: 200, json: async () => ({data: {}}), text: async () => ''}
    }))
    const confirm = vi.fn(() => true)
    vi.stubGlobal('confirm', confirm)
    const popover = await openWorkflowNamePopover(host)
    expect(popover.querySelector('.workflowNameTrigger')).toBeNull()
    const historyItem = Array.from(popover.querySelectorAll('.workflowLoaderItem'))
      .find(button => button.dataset.workflowId === 'history-flow')
    expect(historyItem).toBeTruthy()
    expect(historyItem.disabled).toBe(false)
    expect(historyItem.closest('.workflowNamePopover')).toBe(popover)
    expect(popover.querySelectorAll('.workflowLoaderItem')).toHaveLength(1)
    expect(host.querySelector('.workflowSaveState').textContent).not.toBe('已保存')
    expect(host.querySelectorAll('.workflowLoaderItem')[0].dataset.workflowId).toBe('history-flow')
    await act(async () => {
      historyItem.click()
    })
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 50)) })
    expect(loads).toBe(1)
    expect(host.querySelector('input.workflowStageNameInput').value).toBe('准备')

    await act(async () => { topBarButton(host, '撤销').click() })
    expect(host.querySelector('input.workflowStageNameInput').value).toBe('准备')
    expect(topBarButton(host, '撤销').disabled).toBe(true)
  })
})

describe('WorkflowEditor 阶段串并行切换', () => {
  const workflow = {
    id: 'mode-flow',
    name: '模式流程',
    parameters: [],
    stages: [
      {id: 'fanout', name: '并行', execution_mode: 'parallel', max_concurrency: 3},
      {id: 'verify', name: '验证', execution_mode: 'serial'}
    ],
    nodes: [
      {id: 'prepare', type: 'shell', name: '准备', stage_id: 'fanout', order: 10, script: {script: 'echo prepare'}},
      {id: 'write-a', type: 'shell', name: '写 A', stage_id: 'fanout', order: 20, script: {script: 'echo a'}},
      {id: 'write-b', type: 'shell', name: '写 B', stage_id: 'fanout', order: 30, script: {script: 'echo b'}},
      {id: 'health', type: 'shell', name: '检查', stage_id: 'verify', order: 10, script: {script: 'echo ok'}}
    ],
    edges: [
      {from: 'prepare', to: 'write-a'},
      {from: 'prepare', to: 'write-b'},
      {from: 'write-a', to: 'health'}
    ]
  }

  function modeSelect(host) {
    return host.querySelector('.workflowStageColumn select.workflowStageModeSelect')
  }

  function concurrencyInput(host) {
    return host.querySelector('.workflowStageColumn input.workflowStageConcurrency')
  }

  it('在并行与串行之间往返时保留并发上限与同机并发提示', async () => {
    const host = await mountEditorWithHosts({hosts: [], workflow})
    expect(modeSelect(host).value).toBe('parallel')
    expect(concurrencyInput(host).value).toBe('3')
    expect(host.querySelectorAll('.workflowStageWarning').length).toBe(1)

    await setSelectValue(modeSelect(host), 'serial')
    expect(modeSelect(host).value).toBe('serial')
    expect(concurrencyInput(host)).toBeNull()
    expect(host.querySelectorAll('.workflowStageWarning').length).toBe(0)

    await setSelectValue(modeSelect(host), 'parallel')
    expect(concurrencyInput(host).value).toBe('3')
    expect(host.querySelectorAll('.workflowStageWarning').length).toBe(1)
  })

  it('并发数输入框可以清空后重新输入', async () => {
    const host = await mountEditorWithHosts({hosts: [], workflow})
    const input = concurrencyInput(host)
    expect(input.value).toBe('3')

    await setInputValue(input, '')
    await act(async () => {})
    expect(concurrencyInput(host).value).toBe('')

    await setInputValue(concurrencyInput(host), '6')
    await act(async () => {})
    expect(concurrencyInput(host).value).toBe('6')
  })

  it('切换模式可撤销且保存保留显式依赖和并发上限', async () => {
    const host = await mountEditorWithHosts({hosts: [], workflow})
    let saved
    vi.stubGlobal('fetch', vi.fn(async (path, options) => {
      if (path.endsWith('/save')) saved = JSON.parse(options.body).workflow
      return {ok: true, status: 200, json: async () => ({data: {}}), text: async () => ''}
    }))
    await setSelectValue(modeSelect(host), 'serial')
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 350)) })
    await clickButtonByText(host.querySelector('.workflowTopBar'), '撤销')
    expect(modeSelect(host).value).toBe('parallel')
    expect(concurrencyInput(host).value).toBe('3')
    const dialog = await openSaveModal(host)
    await clickButtonByText(dialog, '保存')
    expect(saved.edges).toEqual(workflow.edges)
    expect(saved.stages[0]).toMatchObject({execution_mode: 'parallel', max_concurrency: 3})
  })

  it('运行目标覆盖为同一主机时提示实际并发冲突', async () => {
    const targeted = {...workflow, nodes: workflow.nodes.map((node, index) => ({...node, host: `host-${index}`}))}
    const host = await mountEditorWithHosts({hosts: [{id: 'shared', name: '共享主机', enabled: true}], workflow: targeted})
    expect(host.querySelectorAll('.workflowStageWarning')).toHaveLength(0)
    await clickButtonByAriaLabel(host, '运行工作流')
    const target = Array.from(host.querySelectorAll('.workflowTargetOption')).find(button => button.querySelector('strong').textContent === '共享主机')
    await act(async () => { target.click() })
    await act(async () => { host.querySelector('.workflowTargetOverride input').click() })
    const warnings = Array.from(host.querySelectorAll('.workflowStageWarning')).map(warning => warning.textContent).join('\n')
    expect(warnings).toContain('共享主机')
    expect(warnings).toContain('3 个节点可能同时执行')
  })
})

async function setInputValue(input, value) {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
  await act(async () => {
    setter.call(input, value)
    input.dispatchEvent(new Event('input', {bubbles: true}))
  })
}

describe('WorkflowEditor 导航与设置入口', () => {
  const workflow = {id: 'nav-flow', name: '导航流程', parameters: [], nodes: [{id: 'first', tool: 'demo.hello'}], edges: []}
  const otherWorkflow = {...workflow, id: 'other-flow', name: '另一个流程', nodes: [{id: 'other', tool: 'demo.hello'}]}

  async function mountNavigationWorkflows() {
    const workflows = [workflow, otherWorkflow]
    const request = vi.fn(async path => {
      const selected = workflows.find(item => path === `/api/workflows/${item.id}`)
      return {ok: true, status: 200, json: async () => ({data: selected ? {config: selected} : {}}), text: async () => ''}
    })
    vi.stubGlobal('fetch', request)
    const host = await mountEditorWithCatalog({...catalog, tools: [{id: 'demo.hello', name: '问候'}], workflows})
    for (const item of [otherWorkflow, workflow]) {
      const popover = await openWorkflowNamePopover(host)
      await clickWorkflowListItem(popover, item.id)
    }
    request.mockClear()
    return {host, request}
  }

  it('更多菜单提供工作流设置入口，未保存时禁用', async () => {
    const host = await mountEditor()
    await clickButtonByText(host.querySelector('.workflowMoreMenu'), '更多')
    const entry = Array.from(host.querySelectorAll('[role="menuitem"]')).find(button => button.textContent === '工作流设置')
    expect(entry).toBeTruthy()
    expect(entry.disabled).toBe(true)
  })

  it('已保存工作流可从更多菜单打开设置并区分保存按钮', async () => {
    const host = await mountEditorWithHosts({hosts: [], workflow})
    await clickButtonByText(host.querySelector('.workflowMoreMenu'), '更多')
    await clickButtonByText(host.querySelector('.workflowMorePopover'), '工作流设置')
    const dialog = host.querySelector('[role="dialog"][aria-label="工作流设置"]')
    expect(dialog).toBeTruthy()
    expect(Array.from(dialog.querySelectorAll('button')).some(button => button.textContent === '保存设置')).toBe(true)
    expect(dialog.textContent).toContain('归属')
  })

  it('名称弹层展示最近标记与完整列表', async () => {
    const host = await mountEditorWithHosts({hosts: [], workflow})
    const popover = await openWorkflowNamePopover(host)
    expect(host.querySelector('.workflowTopBar').classList.contains('menuOpen')).toBe(true)
    const items = Array.from(popover.querySelectorAll('.workflowLoaderItem'))
    expect(items).toHaveLength(1)
    const item = items[0]
    expect(item.textContent).toContain('导航流程')
    expect(item.disabled).toBe(false)
    expect(item.textContent).toContain('已加载')
    expect(popover.querySelector('.workflowLoaderSummary').textContent).toContain('全部 · 1 个')
    expect(popover.querySelector('input[aria-label="搜索工作流"]')).toBeTruthy()
    expect(popover.querySelector('select[aria-label="按分类筛选工作流"]')).toBeTruthy()
  })

  it('从工作流列表切换成功后返回当前画布', async () => {
    const {host} = await mountNavigationWorkflows()
    const popover = await openWorkflowNamePopover(host)
    const item = Array.from(popover.querySelectorAll('.workflowLoaderItem')).find(button => button.textContent.includes(otherWorkflow.name))
    await act(async () => { item.click() })
    expect(host.querySelector('.workflowNamePopover')).toBeNull()
    expect(host.querySelector('.workflowNameTrigger').textContent).toContain(otherWorkflow.name)
    expect(host.querySelector('.react-flow__node[data-id="other"]')).toBeTruthy()
  })

  it.each(['列表选择', '最近标记'])('拒绝从%s放弃未保存修改时保留弹层和当前草稿', async entry => {
    const {host, request} = await mountNavigationWorkflows()
    const saveDialog = await openSaveModal(host)
    const nameInput = Array.from(saveDialog.querySelectorAll('label')).find(label => label.querySelector('span')?.textContent === '名称').querySelector('input')
    await setInputValue(nameInput, '保留的草稿名称')
    await clickButtonByText(saveDialog, '取消')
    const popover = await openWorkflowNamePopover(host)
    const confirm = vi.fn(() => false)
    vi.stubGlobal('confirm', confirm)
    const item = Array.from(popover.querySelectorAll('.workflowLoaderItem')).find(button => button.textContent.includes(otherWorkflow.name))
    await act(async () => { item.click() })
    expect(confirm).toHaveBeenCalledOnce()
    expect(host.querySelector('.workflowNamePopover')).not.toBeNull()
    expect(host.querySelector('.workflowNameTrigger').textContent).toContain('保留的草稿名称')
    expect(host.querySelector('.react-flow__node[data-id="first"]')).toBeTruthy()
    expect(request.mock.calls.some(([path]) => path === `/api/workflows/${otherWorkflow.id}`)).toBe(false)
  })
})

describe('WorkflowEditor 节点选择可用性', () => {
  it('节点选择面板把并行/合流标为旧流程兼容并说明原因', async () => {
    const host = await mountEditor()
    await clickButtonByText(host, '搜索全部')
    const panel = host.querySelector('.nodePickerPanel')
    expect(panel).toBeTruthy()
    const legacy = Array.from(panel.querySelectorAll('.nodePickerItem.legacy'))
    expect(legacy.map(item => item.querySelector('b').textContent)).toEqual(['并行分支', '合流'])
    expect(legacy.every(item => item.disabled)).toBe(true)
    expect(legacy[0].textContent).toContain('旧流程兼容')
    expect(legacy[0].querySelector('.nodePickerUnavailableReason').textContent).toContain('阶段')
  })

  it('缺少插件依赖的工具在选择面板给出原因和插件管理入口', async () => {
    const openPluginManager = vi.fn()
    const host = await mountEditorWithCatalog({
      ...catalog,
      workflows: [
        ...catalog.workflows,
        {id: 'user.flow', name: '用户流程', missing_tools: ['vendor.missing'], dependencies: [{id: 'vendor.off', name: '离线插件', disabled: true}]}
      ]
    }, {openPluginManager})
    await clickButtonByText(host, '搜索全部')
    const missing = host.querySelector('.nodePickerMissing')
    expect(missing.textContent).toContain('vendor.missing')
    expect(missing.textContent).toContain('插件未安装或未注册')
    expect(missing.textContent).toContain('离线插件')
    expect(missing.textContent).toContain('已禁用')
    const openPlugins = Array.from(missing.querySelectorAll('button')).find(button => button.textContent === '打开插件管理')
    expect(openPlugins).toBeTruthy()
    await act(async () => { openPlugins.click() })
    expect(openPluginManager).toHaveBeenCalledTimes(1)
    expect(host.querySelector('.nodePickerPanel')).toBeNull()
  })
})

describe('WorkflowEditor 阶段归属', () => {
  it('阶段工作流里缺 stage_id 的节点会提示归属，而不是被悄悄塞进第一个阶段', async () => {
    const host = await mountEditorWithHosts({
      hosts: [],
      workflow: {
        id: 'mixed.demo',
        name: '混合阶段',
        stages: [{id: 'prepare', name: '准备'}],
        nodes: [
          {id: 'a', name: '已归属', tool: 'demo.hello', stage_id: 'prepare', order: 10},
          {id: 'b', name: '未归属', tool: 'demo.hello'}
        ],
        edges: [],
        parameters: []
      }
    })
    const notice = host.querySelector('.workflowMixedStageNotice')
    expect(notice?.textContent).toContain('未归属')
    expect(notice.textContent).toContain('节点未归属阶段')
  })
})

describe('WorkflowEditor 独立草稿窗口', () => {
  const workflow = {
    id: 'draft.flow',
    name: '草稿流程',
    category: 'docker',
    parameters: [],
    nodes: [{id: 'first', name: '首步', tool: 'demo.hello'}],
    edges: []
  }

  async function mountDraftWindow(draft = workflow) {
    if (draft) writeDraftWindowTransfer({id: draft.id, name: draft.name, ownerID: 'user.workflows', draft})
    return mountEditorWithCatalog(
      {...catalog, tools: [{id: 'demo.hello', name: '问候', category: 'docker'}], workflows: [workflow]},
      {draftWindow: true}
    )
  }

  it('把当前画布草稿交给固定名称的新窗口', async () => {
    const openSpy = vi.fn(() => ({closed: false}))
    vi.stubGlobal('open', openSpy)
    const host = await mountEditorWithHosts({hosts: [], workflow})

    await clickButtonByAriaLabel(host, '打开独立草稿窗口')

    const transfer = readDraftWindowTransfer()
    expect(transfer.id).toBe('draft.flow')
    expect(transfer.draft.nodes.map(node => node.id)).toEqual(['first'])
    expect(openSpy).toHaveBeenCalledTimes(1)
    expect(String(openSpy.mock.calls[0][0])).toContain('draftWindow=1')
    expect(openSpy.mock.calls[0][1]).toBe('opsWorkflowDraftWindow')
  })

  it('窗口被拦截时提示允许弹出窗口，不写入运行状态', async () => {
    vi.stubGlobal('open', vi.fn(() => null))
    const host = await mountEditorWithHosts({hosts: [], workflow})

    await clickButtonByAriaLabel(host, '打开独立草稿窗口')

    expect(host.querySelector('.workflowEditorNotice')?.textContent).toContain('拦截')
  })

  it('草稿窗口按带入内容恢复画布，并且不提供运行与再开窗口入口', async () => {
    const host = await mountDraftWindow({
      ...workflow,
      nodes: [{id: 'edited', name: '已改', tool: 'demo.hello'}]
    })

    expect(host.querySelector('.react-flow__node[data-id="edited"]')).toBeTruthy()
    expect(host.querySelector('.workflowDraftWindowBanner')?.textContent).toContain('已带入主窗口草稿')
    expect(host.querySelector('button[aria-label="运行工作流"]')).toBeNull()
    expect(host.querySelector('button[aria-label="打开独立草稿窗口"]')).toBeNull()
    expect(host.querySelector('.workflowTopBarHint')?.textContent).toContain('运行请在主窗口进行')
  })

  it('主窗口收到草稿窗口保存标记后提示重新加载，画布保持原内容', async () => {
    const host = await mountEditorWithHosts({hosts: [], workflow})

    await act(async () => {
      writeDraftWindowSaved({id: 'draft.flow', name: '草稿流程', ts: new Date(2026, 0, 2, 3, 4).getTime()})
      window.dispatchEvent(new StorageEvent('storage', {key: DRAFT_WINDOW_SAVED_KEY}))
    })

    const notice = host.querySelector('.workflowDraftWindowSaved')
    expect(notice?.textContent).toContain('草稿流程')
    expect(notice.textContent).toContain('01-02 03:04')
    // 提示只是提醒：不自动重新加载，画布仍是当前内容。
    expect(host.querySelector('.react-flow__node[data-id="first"]')).toBeTruthy()

    const later = Array.from(notice.querySelectorAll('button')).find(button => button.textContent === '稍后处理')
    await act(async () => {
      later.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
    })
    expect(host.querySelector('.workflowDraftWindowSaved')).toBeNull()
    // 处理过的标记要清掉，否则每次打开主窗口都会再提示一次。
    expect(readDraftWindowSaved()).toBeNull()
  })
})
