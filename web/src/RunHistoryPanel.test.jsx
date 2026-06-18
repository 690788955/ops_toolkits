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

async function renderPanel() {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  await act(async () => {
    root.render(<RunHistoryPanel catalog={catalog} />)
  })
  return {host, root}
}

function click(element) {
  act(() => {
    element.dispatchEvent(new MouseEvent('click', {bubbles: true}))
  })
}

describe('RunHistoryPanel canvas view', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn(url => {
      const path = String(url)
      if (path === '/api/runs/') {
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

  it('opens a workflow run directly in canvas view and can switch back to logs', async () => {
    const {host, root} = await renderPanel()
    await vi.waitFor(() => expect(host.textContent).toContain('workflow-run-1'))

    const workflowItem = [...host.querySelectorAll('.runHistoryItem')]
      .find(item => item.textContent.includes('workflow-run-1'))
    expect([...workflowItem.querySelectorAll('button')].some(button => button.textContent === '画布')).toBe(false)
    click(workflowItem.querySelector('.runHistorySelectButton'))
    await vi.waitFor(() => expect(host.querySelector('[aria-label="运行详情视图"]')).not.toBeNull())
    click([...host.querySelectorAll('[aria-label="运行详情视图"] button')].find(button => button.textContent === '画布'))

    await vi.waitFor(() => expect(host.querySelector('.runRecordCanvas')).not.toBeNull())
    expect(host.textContent).toContain('演示流程')
    expect(host.querySelectorAll('.react-flow__node')).toHaveLength(2)

    click([...host.querySelectorAll('.react-flow__node')].find(node => node.textContent.includes('巡检')))
    await vi.waitFor(() => expect(host.textContent).toContain('inspect ok'))

    click([...host.querySelectorAll('.segmentedControl button')].find(button => button.textContent === '日志'))
    await vi.waitFor(() => expect(host.querySelector('.liveTerminal')).not.toBeNull())
    expect(host.querySelector('.runRecordCanvas')).toBeNull()

    click([...host.querySelectorAll('.segmentedControl button')].find(button => button.textContent === '画布'))
    await vi.waitFor(() => expect(host.querySelector('.runRecordCanvas')).not.toBeNull())

    await act(async () => root.unmount())
  })
})
