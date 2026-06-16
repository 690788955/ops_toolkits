import React from 'react'
import {controlShapeMarker} from './nodes.jsx'

const controlNodeCatalog = [
  {type: 'condition', title: '条件分支', secondary: 'Switch / Case', description: '根据上游输出或工作流参数选择后续分支', help: '适合根据巡检结果、返回文本、参数值做分流', enabled: true},
  {type: 'parallel', title: '并行分支', secondary: 'Parallel', description: '将后续任务拆分为多个分支路径', help: '用于明确 fan-out 分支结构；当前运行按 DAG 顺序调度', enabled: true},
  {type: 'join', title: '合流', secondary: 'Join', description: '等待多个上游分支完成后继续流程', help: '用于明确 fan-in 汇聚点；入边完成后节点记为成功', enabled: true},
  {type: 'loop', title: '循环', secondary: 'Loop', description: '按固定次数重复执行一个内嵌选择的插件工具', help: '执行到循环节点时，按最大次数重复运行已选择的插件工具', enabled: true},
  {type: 'upload', title: '上传文件', secondary: 'Upload', description: '运行前上传本地文件、批量文件或目录到平台受控目录', help: '上传节点会在工作流启动前选择文件或目录，运行时输出上传结果 JSON', enabled: true}
]

function NodePickerPanel({searchText, setSearchText, tools, totalTools, panelPosition, canvasElement, mode = 'add', connection, insertEdge, onAddTool, onAddControl, onClose}) {
  const keyword = searchText.trim().toLowerCase()
  const quickAdd = Boolean(connection?.source)
  const insertMode = mode === 'insert' && insertEdge?.source && insertEdge?.target
  const panelStyle = pickerPanelStyle(panelPosition, canvasElement)
  const title = insertMode ? '插入节点' : quickAdd ? '添加下游节点' : '添加节点'
  const subtitle = insertMode
    ? `选择后插入到 ${insertEdge.source} → ${insertEdge.target}`
    : quickAdd
      ? `选择后会自动连接到 ${connection.source}`
      : '搜索插件工具，或插入条件/并行/合流/循环节点'
  const matchingControls = controlNodeCatalog
    .filter(control => control.enabled)
    .filter(control => !keyword || [control.title, control.secondary, control.description, control.help]
      .filter(Boolean)
      .some(value => String(value).toLowerCase().includes(keyword)))
  return (
    <div className="nodePickerLayer nodrag nopan" onMouseDown={event => event.stopPropagation()}>
      <div className="nodePickerPanel" style={panelStyle}>
        <div className="nodePickerHeader">
          <div>
            <strong>{title}</strong>
            <span>{subtitle}</span>
          </div>
          <button type="button" className="modalClose" onClick={onClose}>×</button>
        </div>
        <input value={searchText} placeholder="搜索工具、编排节点、描述或 ID" onChange={event => setSearchText(event.target.value)} autoFocus />
        <div className="nodePickerSection">
          <span>编排节点 · {matchingControls.length} / {controlNodeCatalog.filter(control => control.enabled).length}</span>
          <div className="nodePickerList compact">
            {matchingControls.map(control => (
              <button key={control.type} type="button" className="nodePickerItem control" onClick={() => onAddControl(control.type)} title={`${control.title}\n${control.secondary}\n${control.help || control.description}`}>
                <span className={`paletteShape ${control.type}`} data-symbol={controlShapeMarker(control.type)} aria-hidden="true" />
                <span className="nodePickerItemText">
                  <b>{control.title}</b>
                  <small>{control.secondary}</small>
                </span>
              </button>
            ))}
            {matchingControls.length === 0 && <div className="empty small">没有匹配的编排节点；可尝试搜索 Switch、Parallel、Join 或 Loop。</div>}
          </div>
        </div>
        <div className="nodePickerSection">
          <span>插件工具 · {tools.length} / {totalTools}</span>
          <div className="nodePickerList">
            {tools.map(tool => (
              <button key={tool.id} type="button" className="nodePickerItem" onClick={() => onAddTool(tool)} title={`${tool.name || tool.id}\n${tool.id}${tool.description ? `\n${tool.description}` : ''}`}>
                <span className="paletteShape tool" aria-hidden="true" />
                <span className="nodePickerItemText">
                  <b>{tool.name || tool.id}</b>
                  <small>{toolPickerMeta(tool)}</small>
                </span>
              </button>
            ))}
            {tools.length === 0 && <div className="empty small">没有匹配的插件工具；可换用工具名称、描述、ID 或标签继续搜索。</div>}
          </div>
        </div>
      </div>
    </div>
  )
}

function CanvasDock({onZoomIn, onZoomOut, onFitView, onAutoLayout, onRunWorkflow, runDisabled = false}) {
  return (
    <div className="canvasDock nodrag nopan" onMouseDown={event => event.stopPropagation()} aria-label="画布操作">
      <button type="button" onClick={onZoomOut} title="缩小画布" aria-label="缩小画布">−</button>
      <button type="button" onClick={onZoomIn} title="放大画布" aria-label="放大画布">+</button>
      <button type="button" onClick={onFitView} title="将全部节点适配到当前视图">适配</button>
      <button type="button" onClick={onAutoLayout} title="按依赖关系重新排列当前画布节点">自动排版</button>
      <button type="button" className="canvasDockPrimary" onClick={onRunWorkflow} disabled={runDisabled} title={runDisabled ? '当前工作流正在运行' : '执行当前工作流草稿'}>运行工作流</button>
    </div>
  )
}

function pickerPanelStyle(position, element) {
  if (!position) return undefined
  const viewportWidth = element?.clientWidth || window.innerWidth || position.x
  const viewportHeight = element?.clientHeight || window.innerHeight || position.y
  const x = Math.max(220, Math.min(position.x, Math.max(220, viewportWidth - 220)))
  const y = Math.max(120, Math.min(position.y, Math.max(120, viewportHeight - 120)))
  return {left: `${x}px`, top: `${y}px`}
}

function toolPickerMeta(tool) {
  const paramCount = (tool.parameters || []).length
  const source = tool.source?.plugin_name || tool.source?.plugin_id || tool.category || '插件工具'
  return `${source} · ${paramCount} 参数`
}

export {CanvasDock, NodePickerPanel}
