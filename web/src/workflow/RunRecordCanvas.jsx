import React, {useMemo, useState} from 'react'
import {Background, Controls, MiniMap, ReactFlow} from '@xyflow/react'
import {nodeTypes} from './nodes.jsx'
import * as workflowModel from './model.js'
import {
  buildCanvasRunStateFromDetail,
  buildDisplayEdges,
  buildDisplayNodes,
  CanvasNodeLogPanel,
  findRunLogItem,
  flowEdgeFromWorkflowEdge,
  legacyLoopTargetMap,
  remapLegacyLoopEdges,
  workflowNodeToFlowNode
} from './WorkflowEditor.jsx'

export default function RunRecordCanvas({catalog, workflow, detail}) {
  const [canvasLogNodeID, setCanvasLogNodeID] = useState('')
  const [canvasLogMinimized, setCanvasLogMinimized] = useState(false)

  const {nodes, edges} = useMemo(() => flowFromWorkflow(workflow), [workflow])
  const canvasRunState = useMemo(() => buildCanvasRunStateFromDetail(detail, nodes), [detail, nodes])
  const displayNodes = useMemo(() => buildDisplayNodes(nodes, canvasRunState, catalog?.tools || [], null, false), [nodes, canvasRunState, catalog?.tools])
  const displayEdges = useMemo(() => buildDisplayEdges(edges, displayNodes, canvasRunState), [edges, displayNodes, canvasRunState])
  const canvasLogItem = useMemo(() => findRunLogItem(detail?.logs?.items || [], canvasLogNodeID), [detail, canvasLogNodeID])

  if (!workflow) {
    return <div className="empty">该运行记录不是工作流，或当前无法读取对应工作流配置。</div>
  }
  if (nodes.length === 0) {
    return <div className="empty">当前工作流没有可展示的画布节点。</div>
  }

  return (
    <section className="runRecordCanvas">
      <div className="runRecordCanvasHeader">
        <div>
          <strong>{workflow.name || workflow.id}</strong>
          <span>{nodes.length} 个节点 · {edges.length} 条依赖</span>
        </div>
      </div>
      <div className="runRecordCanvasViewport">
        <ReactFlow
          nodes={displayNodes}
          edges={displayEdges}
          nodeTypes={nodeTypes}
          fitView
          fitViewOptions={{padding: 0.22}}
          nodesDraggable={false}
          nodesConnectable={false}
          edgesReconnectable={false}
          elementsSelectable={false}
          deleteKeyCode={null}
          onNodeClick={(_, node) => {
            setCanvasLogNodeID(current => {
              if (current === node.id) return ''
              setCanvasLogMinimized(false)
              return node.id
            })
          }}
        >
          <MiniMap pannable zoomable />
          <Controls showInteractive={false} />
          <Background gap={18} />
          {canvasLogNodeID && (
            <CanvasNodeLogPanel
              nodeID={canvasLogNodeID}
              item={canvasLogItem}
              run={canvasRunState.nodes?.[canvasLogNodeID]}
              minimized={canvasLogMinimized}
              onMinimize={() => setCanvasLogMinimized(true)}
              onRestore={() => setCanvasLogMinimized(false)}
              onClose={() => {
                setCanvasLogNodeID('')
                setCanvasLogMinimized(false)
              }}
            />
          )}
        </ReactFlow>
      </div>
    </section>
  )
}

function flowFromWorkflow(workflow) {
  const workflowNodes = workflow?.nodes || []
  const legacyTargetMap = legacyLoopTargetMap(workflowNodes)
  const legacyTargets = new Set(legacyTargetMap.keys())
  const flowNodes = workflowNodes
    .filter(node => !legacyTargets.has(node.id))
    .map((node, index) => workflowNodeToFlowNode(node, index, null, workflowNodes))
  const flowEdges = remapLegacyLoopEdges(workflow?.edges || [], legacyTargetMap)
    .map((edge, index) => flowEdgeFromWorkflowEdge(edge, index, flowNodes))
  return {
    nodes: workflowModel.autoLayoutNodes(flowNodes, flowEdges),
    edges: flowEdges
  }
}
