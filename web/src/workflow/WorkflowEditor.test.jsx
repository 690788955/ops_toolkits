// @vitest-environment jsdom

import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import React from 'react'
import {createRoot} from 'react-dom/client'
import {act} from 'react'
import WorkflowEditor from './WorkflowEditor.jsx'

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
})

async function mountEditor() {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  mounted.push({host, root})
  await act(async () => {
    root.render(<WorkflowEditor catalog={catalog} setResult={() => {}} refreshCatalog={async () => {}} />)
  })
  const editButton = Array.from(host.querySelectorAll('button')).find(button => button.textContent === '编辑模式')
  if (!editButton) throw new Error('未找到编辑模式入口')
  await act(async () => {
    editButton.dispatchEvent(new MouseEvent('click', {bubbles: true}))
  })
  return host
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
  it('编辑侧栏只保留摘要，元数据字段收进保存弹窗', async () => {
    const host = await mountEditor()
    const sidebar = host.querySelector('.editModeSidebar')
    const sidebarLabels = Array.from(sidebar.querySelectorAll('label > span')).map(span => span.textContent)
    expect(sidebarLabels).not.toContain('工作流 ID')
    expect(sidebarLabels).not.toContain('名称')
    expect(sidebar.textContent).toContain('全局工作流 · 全局')

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
    await clickButtonByText(host, '编辑模式')
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
    await clickButtonByText(host, '编辑模式')
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
    const findSharedOption = () => Array.from(host.querySelectorAll('option'))
      .find(option => option.textContent.includes('共享流程'))

    expect(findSharedOption().textContent).not.toContain('共享自')

    const filter = host.querySelector('select[aria-label="按分类筛选工作流"]')
    await setSelectValue(filter, 'alpha')

    const sharedOption = findSharedOption()
    expect(sharedOption).toBeTruthy()
    expect(sharedOption.textContent).toContain('共享自')
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
  const select = host.querySelector('select[aria-label="选择要加载的工作流"]')
  if (!select) throw new Error('未找到工作流选择器')
  await setSelectValue(select, workflow.id)
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

  it('参数之后先选择执行机器，并在确认页展示所选机器', async () => {
    const host = await mountEditorWithHosts({hosts, workflow})

    await clickButtonByAriaLabel(host, '运行工作流')
    expect(host.textContent).toContain('步骤 1 / 3')

    await clickButtonByText(host, '下一步')
    expect(host.textContent).toContain('选择执行机器')
    expect(host.textContent).toContain('步骤 2 / 3')
    const options = Array.from(host.querySelectorAll('.workflowTargetOption'))
    expect(options.map(option => option.querySelector('strong').textContent)).toEqual(['跟随节点配置', '本机', 'B 机', 'C 机'])

    await act(async () => {
      options[3].dispatchEvent(new MouseEvent('click', {bubbles: true}))
    })
    expect(options[3].getAttribute('aria-checked')).toBe('true')
    expect(host.textContent).toContain('1 个节点已单独指定')

    await clickButtonByText(host, '下一步')
    expect(host.textContent).toContain('步骤 3 / 3')
    expect(host.textContent).toContain('执行机器：C 机（root@10.0.0.3:22）')
  })

  it('勾选覆盖后提示全部节点都会改到所选机器', async () => {
    const host = await mountEditorWithHosts({hosts, workflow})

    await clickButtonByAriaLabel(host, '运行工作流')
    await clickButtonByText(host, '下一步')
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
})
