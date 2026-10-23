// @vitest-environment jsdom

import {describe, expect, it, vi} from 'vitest'
import React from 'react'
import {createRoot} from 'react-dom/client'
import {act} from 'react'
import {NodePickerPanel} from './picker.jsx'
const tools = [
  {id: 'docker.exec.run', name: '容器内执行命令', source: {plugin_id: 'docker.exec', plugin_name: 'Docker'}},
  {id: 'docker.exec.shell', name: '执行宿主机 Shell 脚本', source: {plugin_id: 'docker.exec', plugin_name: 'Docker'}},
  {id: 'plugin.system-check.info', name: '系统资源检查', source: {plugin_id: 'plugin.system-check', plugin_name: '系统管理'}}
]

describe('NodePickerPanel plugin/tool selection', () => {
  it('selects a plugin first and then adds the chosen tool', async () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const onAddTool = vi.fn()
    const root = createRoot(host)
    await act(async () => {
      root.render(
        <NodePickerPanel
          searchText=""
          setSearchText={() => {}}
          tools={tools}
          totalTools={tools.length}
          position={null}
          panelPosition={null}
          onAddTool={onAddTool}
          onAddControl={() => {}}
          onClose={() => {}}
        />
      )
    })

    const selects = host.querySelectorAll('.toolPluginPicker select')
    expect(selects).toHaveLength(2)
    expect(Array.from(selects[0].options).map(option => option.textContent)).toEqual(['Docker（2）', '系统管理（1）'])
    expect(selects[0].value).toBe('docker.exec')
    expect(Array.from(selects[1].options).map(option => option.textContent)).toEqual(['容器内执行命令', '执行宿主机 Shell 脚本'])

    await act(async () => {
      selects[0].value = 'plugin.system-check'
      selects[0].dispatchEvent(new Event('change', {bubbles: true}))
    })
    const updated = host.querySelectorAll('.toolPluginPicker select')
    expect(updated[1].value).toBe('plugin.system-check.info')
    expect(Array.from(updated[1].options).map(option => option.textContent)).toEqual(['系统资源检查'])

    const addButton = Array.from(host.querySelectorAll('button')).find(button => button.textContent === '添加工具节点')
    await act(async () => {
      addButton.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
    })
    expect(onAddTool).toHaveBeenCalledWith(tools[2])

    await act(async () => { root.unmount() })
    host.remove()
  })
})

describe('NodePickerPanel 节点分类折叠', () => {
  it('按分类分组，内置能力可添加，旧流程标记节点保持禁用', async () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const onAddControl = vi.fn()
    const root = createRoot(host)
    await act(async () => {
      root.render(
        <NodePickerPanel
          searchText=""
          setSearchText={() => {}}
          tools={[]}
          totalTools={0}
          position={null}
          panelPosition={null}
          onAddTool={() => {}}
          onAddControl={onAddControl}
          onClose={() => {}}
        />
      )
    })

    const groups = host.querySelectorAll('.nodePickerCategory')
    expect(groups).toHaveLength(3)
    expect(Array.from(groups).map(group => group.className.includes('cat-flow'))).toEqual([true, false, false])

    const builtin = host.querySelector('.nodePickerCategory.cat-builtin')
    const builtinHead = builtin.querySelector('.nodePickerCategoryHead')
    expect(builtinHead.getAttribute('aria-expanded')).toBe('true')

    const flowItems = host.querySelectorAll('.nodePickerCategory.cat-flow .nodePickerItem')
    expect(flowItems).toHaveLength(5)
    expect(Array.from(flowItems).map(item => [item.querySelector('b').textContent, item.disabled])).toEqual([
      ['条件分支', false],
      ['并行分支', true],
      ['合流', true],
      ['循环', false],
      ['等待 / 轮询', false]
    ])
    expect(flowItems[1].textContent).toContain('旧流程兼容')
    expect(flowItems[2].textContent).toContain('旧流程兼容')

    const builtinItems = host.querySelectorAll('.nodePickerCategory.cat-builtin .nodePickerItem')
    expect(builtinItems).toHaveLength(3)
    expect(Array.from(builtinItems).map(item => item.disabled)).toEqual([false, false, false])

    await act(async () => {
      builtinItems[0].dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
    })
    expect(onAddControl).toHaveBeenCalledWith('http_request')

    await act(async () => {
      builtinItems[2].dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
    })
    expect(onAddControl).toHaveBeenCalledWith('notify')

    await act(async () => {
      builtinHead.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
    })
    const collapsedHead = host.querySelector('.nodePickerCategory.cat-builtin .nodePickerCategoryHead')
    expect(collapsedHead.getAttribute('aria-expanded')).toBe('false')
    expect(host.querySelectorAll('.nodePickerCategory.cat-builtin .nodePickerItem')).toHaveLength(0)

    await act(async () => { root.unmount() })
    host.remove()
  })

  it('搜索时强制展开所有命中分类', async () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    await act(async () => {
      root.render(
        <NodePickerPanel
          searchText="HTTP"
          setSearchText={() => {}}
          tools={[]}
          totalTools={0}
          position={null}
          panelPosition={null}
          onAddTool={() => {}}
          onAddControl={() => {}}
          onClose={() => {}}
        />
      )
    })

    const builtinItems = host.querySelectorAll('.nodePickerCategory.cat-builtin .nodePickerItem')
    expect(builtinItems).toHaveLength(1)
    expect(builtinItems[0].textContent).toContain('HTTP 调用')

    await act(async () => { root.unmount() })
    host.remove()
  })
})
