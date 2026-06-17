// @vitest-environment jsdom

import {beforeEach, describe, expect, it, vi} from 'vitest'
import React from 'react'
import {createRoot} from 'react-dom/client'
import {renderToStaticMarkup} from 'react-dom/server'
import {act} from 'react'
import {ReactFlow} from '@xyflow/react'
import {defaultCondition} from './model.js'
import WorkflowEditor, {RerunParamsModal, shouldClearCanvasRunOverlayForNodeChanges, workflowNodeToFlowNode} from './WorkflowEditor.jsx'
import {nodeTypes, normalizeRunStatus, runStatusLabel} from './nodes.jsx'

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

  it('normalizes pending nodes as waiting', () => {
    expect(normalizeRunStatus('pending')).toBe('waiting')
    expect(runStatusLabel('waiting')).toBe('等待中')
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

  it('does not require upload files when moving past workflow parameter step', async () => {
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
          activeCategory: '',
          setResult,
          refreshCatalog: vi.fn()
        }))
      })

      const select = host.querySelector('select')
      expect(select).toBeTruthy()
      const setSelectValue = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set
      await act(async () => {
        setSelectValue.call(select, 'upload-flow')
        select.dispatchEvent(new Event('change', {bubbles: true}))
      })
      await act(async () => {})

      const nextButton = Array.from(host.querySelectorAll('button')).find(item => item.textContent === '下一步')
      expect(nextButton).toBeTruthy()
      await act(async () => {
        nextButton.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true}))
      })

      expect(host.textContent).toContain('启动运行')
      expect(host.textContent).toContain('开始运行')
      expect(setResult).toHaveBeenLastCalledWith({message: '参数已确认，可开始运行。'})
    } finally {
      root.unmount()
      host.remove()
      globalThis.fetch = originalFetch
    }
  })

  it('shows tool node parameters in the workflow parameter step', async () => {
    const workflow = {
      id: 'tool-param-flow',
      name: '工具参数工作流',
      category: 'global',
      parameters: [],
      nodes: [{id: 'merge_1', type: 'tool', name: '分片包 MD5 校验并合并', tool: 'docker.exec.verify_md5_merge', params: {package_name: 'pkg.tar.gz'}}],
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
          activeCategory: '',
          setResult: vi.fn(),
          refreshCatalog: vi.fn()
        }))
      })

      const select = host.querySelector('select')
      const setSelectValue = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set
      await act(async () => {
        setSelectValue.call(select, 'tool-param-flow')
        select.dispatchEvent(new Event('change', {bubbles: true}))
      })
      await act(async () => {})

      expect(host.textContent).toContain('分片包 MD5 校验并合并')
      expect(host.textContent).toContain('Docker 容器执行')
      expect(host.textContent).toContain('分片包文件名')
      expect(host.textContent).not.toContain('当前工作流无需参数。')
    } finally {
      root.unmount()
      host.remove()
      globalThis.fetch = originalFetch
    }
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
})
