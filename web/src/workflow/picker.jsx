import React, {useEffect, useMemo, useState} from 'react'
import {controlNodeCatalog, controlNodeMatchesKeyword, groupControlsByCategory, isControlReady} from './catalog.js'
import {controlShapeMarker} from './nodes.jsx'

function toolPluginKey(tool) {
  return tool.source?.plugin_id || tool.source?.plugin_name || tool.category || '其他工具'
}

function toolPluginLabel(tool) {
  return tool.source?.plugin_name || tool.source?.plugin_id || tool.category || '其他工具'
}

function groupToolsByPlugin(tools) {
  const groups = []
  const byKey = new Map()
  for (const tool of tools) {
    const key = toolPluginKey(tool)
    let group = byKey.get(key)
    if (!group) {
      group = {key, label: toolPluginLabel(tool), tools: []}
      byKey.set(key, group)
      groups.push(group)
    }
    group.tools.push(tool)
  }
  return groups
}

function ToolPluginSelect({tools, onAddTool}) {
  const groups = useMemo(() => groupToolsByPlugin(tools), [tools])
  const [pluginKey, setPluginKey] = useState(groups[0]?.key || '')
  const [toolID, setToolID] = useState(groups[0]?.tools[0]?.id || '')
  const activeGroup = groups.find(group => group.key === pluginKey) || groups[0] || null
  const selectedTool = activeGroup?.tools.find(tool => tool.id === toolID) || activeGroup?.tools[0] || null

  useEffect(() => {
    const group = groups.find(item => item.key === pluginKey) || groups[0]
    if (!group) {
      if (pluginKey) setPluginKey('')
      if (toolID) setToolID('')
      return
    }
    if (group.key !== pluginKey) setPluginKey(group.key)
    if (!group.tools.some(tool => tool.id === toolID)) {
      setToolID(group.tools[0]?.id || '')
    }
  }, [groups, pluginKey, toolID])

  if (groups.length === 0) return null

  return (
    <div className="toolPluginPicker">
      <label>
        <span>选择插件</span>
        <select value={activeGroup?.key || ''} onChange={event => setPluginKey(event.target.value)}>
          {groups.map(group => <option key={group.key} value={group.key}>{group.label}（{group.tools.length}）</option>)}
        </select>
      </label>
      <label>
        <span>选择工具</span>
        <select value={selectedTool?.id || ''} onChange={event => setToolID(event.target.value)}>
          {(activeGroup?.tools || []).map(tool => <option key={tool.id} value={tool.id}>{tool.name || tool.id}</option>)}
        </select>
      </label>
      <button type="button" className="primary" disabled={!selectedTool} onClick={() => { if (selectedTool) onAddTool(selectedTool) }}>添加工具节点</button>
      {selectedTool && (
        <small className="toolPluginPickerHint">
          {selectedTool.id}{selectedTool.description ? ` · ${selectedTool.description}` : ''}
        </small>
      )}
    </div>
  )
}

function ControlPickerItem({control, onAddControl}) {
  const ready = isControlReady(control)
  const title = [
    control.title,
    control.secondary,
    control.help || control.description,
    ready ? '' : '规划中：暂不可添加'
  ].filter(Boolean).join('\n')
  return (
    <button
      type="button"
      className={`nodePickerItem control cat-${control.category}${ready ? '' : ' planned'}`}
      disabled={!ready}
      aria-disabled={!ready}
      onClick={ready ? () => onAddControl(control.type) : undefined}
      title={title}
    >
      <span className={`paletteShape ${control.shape} cat-${control.category}`} data-symbol={controlShapeMarker(control.type)} aria-hidden="true" />
      <span className="nodePickerItemText">
        <b>{control.title}</b>
        <small>{control.secondary}{ready ? '' : ' · 规划中'}</small>
      </span>
      {ready ? null : <span className="nodePickerPlannedTag">规划中</span>}
    </button>
  )
}

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
      : '按分类选择：流程控制 / 数据与文件 / 内置能力，或搜索插件工具'
  const matchingControls = controlNodeCatalog.filter(control => controlNodeMatchesKeyword(control, keyword))
  const controlGroups = groupControlsByCategory(matchingControls)
  // 默认全部分类展开；内置能力已有可用节点（HTTP 调用），不再默认收起。
  const [collapsedCategories, setCollapsedCategories] = useState({})
  // 有搜索词时强制展开，避免命中项被折叠隐藏。
  const isCollapsed = categoryID => !keyword && Boolean(collapsedCategories[categoryID])
  const toggleCategory = categoryID => setCollapsedCategories(current => ({...current, [categoryID]: !current[categoryID]}))
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
        <input value={searchText} placeholder="搜索工具、编排节点、分类、描述或 ID" onChange={event => setSearchText(event.target.value)} autoFocus />
        <div className="nodePickerSection">
          <span>编排节点 · {matchingControls.length} / {controlNodeCatalog.length}</span>
          <div className="nodePickerCategories">
            {controlGroups.map(({category, controls}) => {
              const collapsed = isCollapsed(category.id)
              return (
                <div key={category.id} className={`nodePickerCategory cat-${category.id}${collapsed ? ' collapsed' : ''}`}>
                  <button
                    type="button"
                    className="nodePickerCategoryHead"
                    aria-expanded={!collapsed}
                    title={`${category.description}${collapsed ? '\n点击展开' : '\n点击收起'}`}
                    onClick={() => toggleCategory(category.id)}
                  >
                    <i className={`catDot ${category.id}`} aria-hidden="true" />
                    <strong>{category.label}</strong>
                    <small>{category.hint}</small>
                    <em>{controls.length}</em>
                    <span className="nodePickerCategoryCaret" aria-hidden="true">{collapsed ? '▸' : '▾'}</span>
                  </button>
                  {collapsed ? null : (
                    <div className="nodePickerList compact">
                      {controls.map(control => (
                        <ControlPickerItem key={control.type} control={control} onAddControl={onAddControl} />
                      ))}
                    </div>
                  )}
                </div>
              )
            })}
            {controlGroups.length === 0 && <div className="empty small">没有匹配的编排节点；可尝试搜索分支、并行、合流、上传、提取、HTTP 或分类名称。</div>}
          </div>
        </div>
        <div className="nodePickerSection">
          <span>插件工具 · {tools.length} / {totalTools}</span>
          {tools.length > 0
            ? <ToolPluginSelect tools={tools} onAddTool={onAddTool} />
            : <div className="empty small">没有匹配的插件工具；可换用工具名称、描述、ID 或标签继续搜索。</div>}
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

export {CanvasDock, NodePickerPanel}
