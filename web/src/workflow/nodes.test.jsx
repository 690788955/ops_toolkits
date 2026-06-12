import {describe, expect, it} from 'vitest'
import React from 'react'
import {renderToStaticMarkup} from 'react-dom/server'
import {ReactFlow} from '@xyflow/react'
import {defaultCondition} from './model.js'
import {shouldClearCanvasRunOverlayForNodeChanges} from './WorkflowEditor.jsx'
import {nodeTypes, normalizeRunStatus, runStatusLabel} from './nodes.jsx'

describe('workflow node run status', () => {
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
})
