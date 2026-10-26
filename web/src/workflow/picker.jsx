import React, {useMemo, useState} from 'react'
import {createPortal} from 'react-dom'
import {controlNodeCatalog, controlNodeMatchesKeyword, controlUnavailableReason, groupControlsByCategory, isControlReady} from './catalog.js'
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

function ToolPickerItem({tool, onAddTool}) {
  const description = tool.description || ''
  const title = [tool.name || tool.id, tool.id, description, ...(tool.tags || [])].filter(Boolean).join('\n')
  return (
    <button
      type="button"
      className="nodePickerItem tool"
      title={title}
      aria-label={`添加工具节点 ${tool.name || tool.id}`}
      onClick={() => onAddTool(tool)}
    >
      <span className="paletteShape tool" aria-hidden="true" />
      <span className="nodePickerItemText">
        <b>{tool.name || tool.id}</b>
        <span className="toolPickerMeta">
          <code>{tool.id}</code>
          {description && <span>{description}</span>}
        </span>
      </span>
      <span className="toolPickerAddBadge" aria-hidden="true">添加</span>
    </button>
  )
}

function ToolPickerList({tools, keyword, onAddTool}) {
  const groups = useMemo(() => groupToolsByPlugin(tools), [tools])
  // 工具通常较多，默认收起分组；搜索时强制展开，避免命中项被藏起来。
  const [expandedGroups, setExpandedGroups] = useState({})
  const [hoverPreview, setHoverPreview] = useState(null)
  const isCollapsed = key => !keyword && !expandedGroups[key]
  const toggleGroup = key => setExpandedGroups(current => ({...current, [key]: !current[key]}))
  // 悬停只弹出只读备注卡，不展开列表；要添加工具必须先点开分组。
  const showPreview = (group, event) => {
    if (keyword || !isCollapsed(group.key)) return
    const head = event.currentTarget.querySelector('.toolPickerGroupHead')
    const rect = head?.getBoundingClientRect()
    if (!rect) return
    const viewportHeight = window.innerHeight || 800
    // 分组头不在可视区域时不弹卡，避免备注卡贴到屏幕边缘。
    if (rect.bottom < 0 || rect.top > viewportHeight) return
    const cardWidth = 240
    // 卡片只列工具名、不滚动，按条目数估算高度后再决定弹出位置。
    const cardHeight = 56 + group.tools.length * 20
    const openLeft = rect.right + cardWidth + 16 > window.innerWidth
    setHoverPreview({
      key: group.key,
      label: group.label,
      tools: group.tools,
      left: openLeft ? Math.max(8, rect.left - cardWidth - 12) : rect.right + 12,
      top: Math.max(8, Math.min(rect.top, viewportHeight - cardHeight - 8))
    })
  }
  const clearPreview = key => setHoverPreview(current => current?.key === key ? null : current)
  return (
    <div className="toolPickerGroups">
      {groups.map(group => {
        const collapsed = isCollapsed(group.key)
        const previewing = Boolean(hoverPreview && hoverPreview.key === group.key)
        return (
          <section
            key={group.key}
            className={`toolPickerGroup${collapsed ? ' collapsed' : ''}${previewing ? ' previewing' : ''}`}
            onMouseEnter={event => showPreview(group, event)}
            onMouseLeave={() => clearPreview(group.key)}
          >
            <button
              type="button"
              className="toolPickerGroupHead"
              aria-expanded={!collapsed}
              title={`${group.label}${collapsed ? '\n悬停查看工具，点击展开后可添加' : '\n点击收起'}`}
              onClick={() => { clearPreview(group.key); toggleGroup(group.key) }}
            >
              <span>{group.label}</span>
              <em>{group.tools.length}</em>
              <span className="nodePickerCategoryCaret" aria-hidden="true">{collapsed ? '▸' : '▾'}</span>
            </button>
            {collapsed ? null : (
              <div className="toolPickerList">
                {group.tools.map(tool => <ToolPickerItem key={tool.id} tool={tool} onAddTool={onAddTool} />)}
              </div>
            )}
          </section>
        )
      })}
      {hoverPreview && createPortal(
        <div className="toolPickerHoverCard" style={{left: hoverPreview.left, top: hoverPreview.top}} role="tooltip">
          <div className="toolPickerHoverCardHead">
            <strong>{hoverPreview.label}</strong>
            <span>{hoverPreview.tools.length}</span>
          </div>
          <ul>
            {hoverPreview.tools.map(tool => (
              <li key={tool.id} title={[tool.id, tool.description].filter(Boolean).join('\n')}>{tool.name || tool.id}</li>
            ))}
          </ul>
        </div>,
        document.body
      )}
    </div>
  )
}

function ControlPickerItem({control, onAddControl}) {
  const ready = isControlReady(control)
  const reason = controlUnavailableReason(control)
  const title = [
    control.title,
    control.secondary,
    control.help || control.description,
    ready ? '' : reason
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
        {ready ? null : <small className="nodePickerUnavailableReason">{reason}</small>}
      </span>
      {ready ? null : <span className="nodePickerPlannedTag">规划中</span>}
    </button>
  )
}

function MissingToolItem({item, onOpenPlugins}) {
  return (
    <li className="nodePickerMissingItem" title={item.reason}>
      <span className="nodePickerItemText">
        <b>{item.name || item.id}</b>
        <small>{item.id}</small>
        <small className="nodePickerUnavailableReason">{item.reason}</small>
      </span>
      {onOpenPlugins && <button type="button" className="secondary small" onClick={onOpenPlugins}>打开插件管理</button>}
    </li>
  )
}

function NodePickerPanel({searchText, setSearchText, tools, totalTools, missingTools = [], panelPosition, canvasElement, mode = 'add', connection, insertEdge, onAddTool, onAddControl, onOpenPlugins, onClose}) {
  const keyword = searchText.trim().toLowerCase()
  const quickAdd = Boolean(connection?.source)
  const insertMode = mode === 'insert' && insertEdge?.source && insertEdge?.target
  const panelStyle = pickerPanelStyle(panelPosition, canvasElement)
  const title = insertMode ? '插入节点' : quickAdd ? '添加下游节点' : '添加节点'
  const subtitle = insertMode
    ? `选择后插入到 ${insertEdge.source} → ${insertEdge.target}`
    : quickAdd
      ? `选择后会自动连接到 ${connection.source}`
      : '按分类选择编排节点，或搜索并点击插件工具直接添加'
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
            {controlGroups.length === 0 && <div className="empty small">没有匹配的编排节点；可尝试搜索分支、循环、上传、提取、HTTP 或分类名称。</div>}
          </div>
        </div>
        <div className="nodePickerSection">
          <span>插件工具 · {tools.length} / {totalTools} · 点击即添加</span>
          {tools.length > 0
            ? <ToolPickerList tools={tools} keyword={keyword} onAddTool={onAddTool} />
            : <div className="empty small">没有匹配的插件工具；可换用工具名称、描述、ID 或标签继续搜索。</div>}
        </div>
        {missingTools.length > 0 && (
          <div className="nodePickerSection nodePickerMissing">
            <span>依赖缺失 · {missingTools.length}</span>
            <ul className="nodePickerMissingList">
              {missingTools.map(item => (
                <MissingToolItem key={item.id} item={item} onOpenPlugins={onOpenPlugins} />
              ))}
            </ul>
          </div>
        )}
      </div>
    </div>
  )
}

function CanvasDock({onAddNode, onZoomIn, onZoomOut, onFitView}) {
  return (
    <div className="canvasDock nodrag nopan" onMouseDown={event => event.stopPropagation()} aria-label="画布操作">
      {onAddNode && (
        <button type="button" className="canvasDockAdd" onClick={onAddNode} title="添加节点（也可以双击画布空白处）" aria-label="添加节点">
          <span aria-hidden="true">＋</span>
          <span>添加节点</span>
        </button>
      )}
      <button type="button" onClick={onZoomOut} title="缩小画布" aria-label="缩小画布">−</button>
      <button type="button" onClick={onZoomIn} title="放大画布" aria-label="放大画布">+</button>
      <button type="button" onClick={onFitView} title="将全部节点适配到当前视图" aria-label="适配视图">⤢</button>
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
