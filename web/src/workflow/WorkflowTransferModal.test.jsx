// @vitest-environment jsdom

import {afterEach, describe, expect, it, vi} from 'vitest'
import React from 'react'
import {act} from 'react'
import {createRoot} from 'react-dom/client'
import WorkflowTransferModal from './WorkflowTransferModal.jsx'

let mounted = []

afterEach(() => {
  for (const {host, root} of mounted) {
    act(() => root.unmount())
    host.remove()
  }
  mounted = []
  vi.unstubAllGlobals()
})

async function mountModal(props = {}) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  mounted.push({host, root})
  await act(async () => {
    root.render(<WorkflowTransferModal onClose={props.onClose || (() => {})} onImported={props.onImported} />)
  })
  return host
}

function findButton(host, text) {
  const button = Array.from(host.querySelectorAll('button')).find(item => item.textContent === text)
  if (!button) throw new Error(`未找到按钮：${text}`)
  return button
}

describe('WorkflowTransferModal', () => {
  it('只提供导入入口，不显示导出选项', async () => {
    const host = await mountModal()
    expect(findButton(host, '选择文件…')).toBeTruthy()
    expect(host.querySelector('input[type="checkbox"]')).toBeNull()
    expect(Array.from(host.querySelectorAll('button')).some(button => button.textContent.includes('导出'))).toBe(false)
  })

  it('导入文件后展示成功与失败结果', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({
        status: 'imported',
        data: {imported: [{id: 'demo.flow', new_id: 'demo.flow-2', renamed: true}], failed: [{id: 'demo.bad', error: '引用了不存在的工具 x'}]}
      })
    })))
    const onImported = vi.fn()
    const host = await mountModal({onImported})
    const input = host.querySelector('input[type="file"]')
    const file = new File(['id: demo.flow'], 'demo.flow.yaml', {type: 'text/yaml'})
    Object.defineProperty(input, 'files', {value: [file], configurable: true})
    await act(async () => {
      input.dispatchEvent(new Event('change', {bubbles: true}))
    })

    expect(fetch.mock.calls[0][0]).toBe('/api/workflows/import')
    expect(host.textContent).toContain('已导入')
    expect(host.textContent).toContain('demo.flow → demo.flow-2')
    expect(host.textContent).toContain('引用了不存在的工具 x')
    expect(onImported).toHaveBeenCalled()
  })
})