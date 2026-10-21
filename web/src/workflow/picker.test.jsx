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
