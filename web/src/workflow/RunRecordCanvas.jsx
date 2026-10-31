import React, {useEffect, useMemo, useRef, useState} from 'react'
import {Background, Controls, MiniMap, ReactFlow} from '@xyflow/react'
import ResizeHandle, {useResizableWidth} from '../ResizeHandle.jsx'
import {nodeTypes} from './nodes.jsx'
import * as workflowModel from './model.js'
import RunLogWindow from './run/RunLogWindow.jsx'
import {runningRunNodeID} from './run/runLog.js'
import {
  buildCanvasRunStateFromDetail,
  buildDisplayEdges,
  buildDisplayNodes,
  flowEdgeFromWorkflowEdge,
  legacyLoopTargetMap,
  remapLegacyLoopEdges,
  workflowNodeToFlowNode
} from './WorkflowEditor.jsx'

// 运行记录页的画布：和工作流工作台用同一套只读画布 + 右栏节点详情，
// 只是这里的画布固定来自运行快照，节点不能重跑、不能移动。
const RUN_RECORD_LOG_WIDTH_KEY = 'ops.runRecords.canvasLogWidth'

export default function RunRecordCanvas({catalog, workflow, detail}) {
  const [selectedNodeID, setSelectedNodeID] = useState('')
  const [logWindowOpen, setLogWindowOpen] = useState(false)
  // 画布和右栏日志的宽度可以拖：历史运行往往要对照着看，固定比例总有一边不够用。
  const bodyRef = useRef(null)
  const logPanelWidth = useResizableWidth({storageKey: RUN_RECORD_LOG_WIDTH_KEY, min: 320, max: 820})
  useEffect(() => {
    setSelectedNodeID('')
    setLogWindowOpen(false)
  }, [detail?.record?.id])

  const {nodes, edges} = useMemo(() => flowFromWorkflow(workflow), [workflow])
  const canvasRunState = useMemo(() => buildCanvasRunStateFromDetail(detail, nodes), [detail, nodes])
  const displayNodes = useMemo(() => buildDisplayNodes(nodes, canvasRunState, catalog?.tools || [], false, null, '', [], selectedNodeID), [nodes, canvasRunState, catalog?.tools, selectedNodeID])
  const displayEdges = useMemo(() => buildDisplayEdges(edges, displayNodes, canvasRunState), [edges, displayNodes, canvasRunState])

  const runNodeNames = useMemo(() => {
    const names = {}
    nodes.forEach(node => { names[node.id] = node.data?.name || node.id })
    return names
  }, [nodes])

  const runningNodeID = useMemo(() => {
    return runningRunNodeID(canvasRunState)
  }, [canvasRunState])

  if (!workflow) {
    return <div className="empty">该运行记录不是工作流，或当前无法读取对应工作流配置。</div>
  }
  if (nodes.length === 0) {
    return <div className="empty">当前工作流没有可展示的画布节点。</div>
  }

  const record = detail?.record || null
  const bodyStyle = logPanelWidth.width ? {'--workflow-run-panel-width': `${logPanelWidth.width}px`} : undefined
  const measureLogPanel = () => {
    const body = bodyRef.current
    if (!body) return 0
    const panel = body.querySelector('.workflowRunPanelHost:not(.workflowLogWindowBackdrop)')
    return panel ? panel.getBoundingClientRect().width : 0
  }
  const logPanelProps = {
    focusNodeID: selectedNodeID,
    focusNodeName: runNodeNames[selectedNodeID] || selectedNodeID,
    node: nodes.find(node => node.id === selectedNodeID) || null,
    run: canvasRunState.nodes?.[selectedNodeID] || null,
    runState: canvasRunState,
    record,
    logItems: detail?.logs?.items || [],
    nodeOrder: nodes.map(node => node.id),
    runNodeNames,
    activeRunID: record?.id || '',
    isRunning: record?.status === 'running',
    runningNodeID,
    onClearFocus: () => setSelectedNodeID(''),
    supportHref: record?.id ? `/api/runs/${encodeURIComponent(record.id)}/support.zip` : null
  }

  return (
    <section className="runRecordCanvas">
      <div className="runRecordCanvasHeader">
        <div>
          <strong>{workflow.name || workflow.id}</strong>
          <span>{nodes.length} 个节点 · {edges.length} 条依赖</span>
        </div>
        <span className="runRecordCanvasReadonly" title="运行结束后保存的快照，不会随当前工作流配置变化">历史记录，只读</span>
      </div>
      <div className="runRecordCanvasBody" ref={bodyRef} style={bodyStyle}>
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
              setSelectedNodeID(node.id)
            }}
          >
            <MiniMap pannable zoomable />
            <Controls showInteractive={false} />
            <Background gap={18} />
          </ReactFlow>
        </div>
        {!logWindowOpen && (
          <ResizeHandle
            label="调整运行日志栏宽度"
            invert
            min={320}
            max={820}
            value={logPanelWidth.width}
            fallback={400}
            measure={measureLogPanel}
            onChange={logPanelWidth.setWidth}
            onReset={logPanelWidth.reset}
          />
        )}
        <RunLogWindow
          embedded
          open={logWindowOpen}
          onExpand={() => setLogWindowOpen(true)}
          onClose={() => setLogWindowOpen(false)}
          title={selectedNodeID ? `运行日志 · ${runNodeNames[selectedNodeID] || selectedNodeID}` : '运行日志'}
          {...logPanelProps}
        />
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
    nodes: workflowModel.autoLayoutWorkflowNodes(flowNodes, flowEdges, workflow?.stages || []),
    edges: flowEdges
  }
}
