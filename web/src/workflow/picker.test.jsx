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
  it('按插件分组列出工具，点击工具直接添加', async () => {
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

    const groups = host.querySelectorAll('.toolPickerGroup')
    expect(groups).toHaveLength(2)
    expect(Array.from(groups).map(group => group.querySelector('.toolPickerGroupHead > span').textContent)).toEqual(['Docker', '系统管理'])

    await act(async () => {
      groups.forEach(group => group.querySelector('.toolPickerGroupHead').dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true})))
    })
    expect(Array.from(host.querySelectorAll('.toolPickerGroup')).map(group => group.querySelectorAll('.nodePickerItem.tool').length)).toEqual([2, 1])
    expect(host.querySelector('.toolPickerGroup .nodePickerItem.tool b').textContent).toBe('容器内执行命令')

    const target = host.querySelectorAll('.toolPickerGroup')[1].querySelector('.nodePickerItem.tool')
    await act(async () => {
      target.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
    })
    expect(onAddTool).toHaveBeenCalledWith(tools[2])

    await act(async () => { root.unmount() })
    host.remove()
  })

  it('插件工具分组默认收起，展开后可用，搜索时强制展开', async () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    const renderPanel = searchText => (
      <NodePickerPanel
        searchText={searchText}
        setSearchText={() => {}}
        tools={tools}
        totalTools={tools.length}
        position={null}
        panelPosition={null}
        onAddTool={() => {}}
        onAddControl={() => {}}
        onClose={() => {}}
      />
    )
    await act(async () => { root.render(renderPanel('')) })

    const head = host.querySelector('.toolPickerGroupHead')
    expect(head.getAttribute('aria-expanded')).toBe('false')
    expect(host.querySelectorAll('.toolPickerGroup')[0].querySelectorAll('.nodePickerItem.tool')).toHaveLength(0)

    await act(async () => {
      head.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
    })
    expect(host.querySelector('.toolPickerGroupHead').getAttribute('aria-expanded')).toBe('true')
    expect(host.querySelectorAll('.toolPickerGroup')[0].querySelectorAll('.nodePickerItem.tool')).toHaveLength(2)

    await act(async () => { root.render(renderPanel('Docker')) })
    expect(host.querySelector('.toolPickerGroupHead').getAttribute('aria-expanded')).toBe('true')
    expect(host.querySelectorAll('.toolPickerGroup')[0].querySelectorAll('.nodePickerItem.tool')).toHaveLength(2)

    await act(async () => { root.unmount() })
    host.remove()
  })

  it('收起的分组悬停只展示工具备注，展开后才能添加', async () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
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
          onAddTool={() => {}}
          onAddControl={() => {}}
          onClose={() => {}}
        />
      )
    })

    const group = () => host.querySelectorAll('.toolPickerGroup')[0]
    const card = () => document.querySelector('.toolPickerHoverCard')
    expect(group().querySelectorAll('.nodePickerItem.tool')).toHaveLength(0)
    expect(card()).toBeNull()

    await act(async () => {
      group().dispatchEvent(new MouseEvent('mouseover', {bubbles: true, cancelable: true}))
    })
    expect(group().classList.contains('previewing')).toBe(true)
    // 悬停只出备注卡，不展开可点击的工具列表
    expect(group().querySelectorAll('.nodePickerItem.tool')).toHaveLength(0)
    expect(group().querySelector('.toolPickerGroupHead').getAttribute('aria-expanded')).toBe('false')
    expect(card()?.textContent).toContain('容器内执行命令')
    expect(card()?.textContent).toContain('执行宿主机 Shell 脚本')
    expect(card()?.querySelectorAll('li')).toHaveLength(2)
    // 只展示工具名，不带 ID / 描述等明细
    expect(card()?.textContent).not.toContain('docker.exec.run')

    await act(async () => {
      group().dispatchEvent(new MouseEvent('mouseout', {bubbles: true, cancelable: true, relatedTarget: document.body}))
    })
    expect(group().classList.contains('previewing')).toBe(false)
    expect(card()).toBeNull()

    await act(async () => {
      group().querySelector('.toolPickerGroupHead').dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
    })
    expect(group().querySelectorAll('.nodePickerItem.tool')).toHaveLength(2)

    await act(async () => { root.unmount() })
    host.remove()
  })
})

describe('NodePickerPanel 节点分类折叠', () => {
  it('按分类分组，内置能力可添加，且不再提供并行/合流旧节点', async () => {
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
    expect(flowItems).toHaveLength(4)
    expect(Array.from(flowItems).map(item => [item.querySelector('b').textContent, item.disabled])).toEqual([
      ['条件分支', false],
      ['循环', false],
      ['等待 / 轮询', false],
      ['人工审批', false]
    ])
    expect(host.querySelector('.nodePickerCategory.cat-flow').textContent).not.toContain('旧流程兼容')

    await act(async () => {
      flowItems[3].dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
    })
    expect(onAddControl).toHaveBeenCalledWith('approval')

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
