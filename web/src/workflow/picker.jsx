import React from 'react'
import {controlNodeCatalog} from './catalog.js'
import {controlShapeMarker} from './nodes.jsx'

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
      : '搜索插件工具，或插入条件/并行/合流/循环/上传/提取配置节点'
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
            {matchingControls.length === 0 && <div className="empty small">没有匹配的编排节点；可尝试搜索 Switch、Parallel、Join、Loop、Upload 或 Extract。</div>}
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
      <button type="button" onClick={onFitView} title="将全部节点适配到当前视图" aria-label="适配视图">⤢</button>
      <button type="button" onClick={onAutoLayout} title="按依赖关系重新排列当前画布节点" aria-label="自动排版">⇄</button>
      <button type="button" className="canvasDockPrimary" onClick={onRunWorkflow} disabled={runDisabled} title={runDisabled ? '当前工作流正在运行' : '执行当前工作流草稿'} aria-label="运行工作流">▶</button>
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
