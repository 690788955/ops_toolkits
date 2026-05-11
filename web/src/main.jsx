import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createRoot } from 'react-dom/client'
import {
  Background,
  Controls,
  Handle,
  MiniMap,
  Position,
  ReactFlow,
  useEdgesState,
  useNodesState
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import './styles.css'
import * as yaml from 'js-yaml'

const conditionOperators = [
  {value: 'eq', label: '等于'},
  {value: 'neq', label: '不等于'},
  {value: 'contains', label: '包含'},
  {value: 'not_contains', label: '不包含'},
  {value: 'in', label: '在列表中'},
  {value: 'not_in', label: '不在列表中'},
  {value: 'exists', label: '存在'},
  {value: 'empty', label: '为空'}
]

const controlNodeCatalog = [
  {
    type: 'condition',
    title: '条件分支',
    secondary: 'Switch / Case',
    description: '根据上游输出或工作流参数选择后续分支',
    capabilities: ['多分支', '默认分支', '读取 stdout/stderr/参数'],
    help: '适合根据巡检结果、返回文本、参数值做分流',
    enabled: true
  },
  {
    type: 'parallel',
    title: '并行分支',
    secondary: 'Parallel',
    description: '将后续任务拆分为多个分支路径',
    capabilities: ['多路径', '分支结构'],
    help: '用于明确 fan-out 分支结构；当前运行按 DAG 顺序调度',
    enabled: true
  },
  {
    type: 'join',
    title: '合流',
    secondary: 'Join',
    description: '等待多个上游分支完成后继续流程',
    capabilities: ['分支汇聚', '等待上游'],
    help: '用于明确 fan-in 汇聚点；入边完成后节点记为成功',
    enabled: true
  },
  {
    type: 'loop',
    title: '循环',
    secondary: 'Loop',
    description: '按固定次数重复执行一个内嵌选择的插件工具',
    capabilities: ['固定次数', '内嵌工具', '安全上限'],
    help: '执行到循环节点时，按最大次数重复运行已选择的插件工具',
    enabled: true
  }
]

const nodeTypes = {toolNode: ToolNode, conditionNode: ConditionNode, controlNode: ControlNode}

function FlowchartShape({kind, marker}) {
  return (
    <span className={`flowchartShape ${kind}`} aria-hidden="true">
      <span>{marker}</span>
    </span>
  )
}

function controlShapeKind(type) {
  if (type === 'condition') return 'decision'
  if (type === 'parallel' || type === 'join' || type === 'loop') return `gateway ${type}`
  return 'planned'
}

function controlShapeMarker(type) {
  if (type === 'condition') return '?'
  if (type === 'parallel') return '+'
  if (type === 'join') return '∧'
  if (type === 'loop') return '↻'
  return '·'
}

function controlShapeLabel(type) {
  if (type === 'condition') return 'Decision 条件判断'
  if (type === 'parallel') return 'Gateway 并行分支'
  if (type === 'join') return 'Gateway 合流'
  if (type === 'loop') return 'Loop 固定次数循环'
  return '流程节点'
}

function ToolNode({id, data, selected}) {
  const runTitle = formatNodeRunTitle(data.run)
  const nodeTitle = [data.name || id, data.tool, runTitle].filter(Boolean).join('\n')
  return (
    <div className={nodeRunClass('toolNode', selected, data.run)} title={nodeTitle}>
      <Handle type="target" position={Position.Left} />
      <RunStatusBadge run={data.run} />
      <button className="nodeDelete nodrag nopan" onMouseDown={event => event.stopPropagation()} onClick={event => { event.stopPropagation(); data.onRemove(id) }} title="删除节点">×</button>
      <strong>{data.name || id}</strong>
      {data.tool && <span className="nodeHoverMeta">{data.tool}</span>}
      <Handle type="source" position={Position.Right} />
    </div>
  )
}


function ConditionNode({id, data, selected}) {
  const condition = data.condition || defaultCondition()
  const status = conditionNodeStatus(condition)
  const branches = conditionBranchRows(condition)
  const runTitle = formatNodeRunTitle(data.run)
  const nodeTitle = [data.name || id, conditionSummary(condition), conditionCaseSummary(condition), status.label, runTitle].filter(Boolean).join('\n')
  return (
    <div className={nodeRunClass('conditionNode', selected, data.run)} title={nodeTitle}>
      <Handle type="target" position={Position.Left} />
      <RunStatusBadge run={data.run} />
      <div className="conditionDiamond" aria-hidden="true"><span>?</span></div>
      <button className="nodeDelete nodrag nopan" onMouseDown={event => event.stopPropagation()} onClick={event => { event.stopPropagation(); data.onRemove(id) }} title="删除节点">×</button>
      <div className="conditionInfoCard">
        <strong>{data.name || id}</strong>
        <div className="conditionInputSummary">{conditionSummary(condition)}</div>
        <small>{conditionCaseSummary(condition)}</small>
        <small className={status.ready ? 'conditionState ready' : 'conditionState warning'}>{status.label}</small>
      </div>
      <div className="conditionBranchList" aria-label="条件分支出口">
        {branches.map(branch => (
          <div key={branch.key} className={`conditionBranchRow ${branch.kind}${branch.disabled ? ' disabled' : ''}${data.run?.matchedCase === branch.handleID ? ' matched' : ''}`} title={branch.meta}>
            <span className="conditionBranchLine" aria-hidden="true" />
            <div className="conditionBranchText">
              <strong>{branch.label}</strong>
              <span>{branch.meta}</span>
            </div>
            {branch.handleID ? (
              <Handle
                id={branch.handleID}
                type="source"
                position={Position.Right}
                className="conditionBranchHandle"
                isConnectable={!branch.disabled}
                title={`连接 ${branch.label}`}
              />
            ) : (
              <span className="conditionBranchHandlePreview" aria-hidden="true" />
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
function ControlNode({id, data, selected}) {
  const loop = data.loop || {}
  const loopSummary = loop.tool ? `工具 ${loop.tool} × ${loop.max_iterations || 0}` : '未配置循环工具'
  const runLoopSummary = data.run?.loopIterations ? `实际迭代 ${data.run.loopIterations} 次` : ''
  const helpText = data.controlType === 'loop' ? [loopSummary, runLoopSummary].filter(Boolean).join('；') : controlNodeHelp(data.controlType)
  const runTitle = formatNodeRunTitle(data.run)
  const nodeTitle = [data.name || id, controlShapeLabel(data.controlType), helpText, runTitle].filter(Boolean).join('\n')
  return (
    <div className={nodeRunClass(`controlNode ${data.controlType || ''}`, selected, data.run)} title={nodeTitle}>
      <Handle type="target" position={Position.Left} />
      <RunStatusBadge run={data.run} />
      <button className="nodeDelete nodrag nopan" onMouseDown={event => event.stopPropagation()} onClick={event => { event.stopPropagation(); data.onRemove(id) }} title="删除节点">×</button>
      <FlowchartShape kind={controlShapeKind(data.controlType)} marker={controlShapeMarker(data.controlType)} />
      <div className="controlNodeText">
        <strong>{data.name || id}</strong>
        <small>{controlShapeLabel(data.controlType)} · {helpText}</small>
      </div>
      <Handle type="source" position={Position.Right} />
    </div>
  )
}

function App() {
  const [catalog, setCatalog] = useState(null)
  const [activeCategory, setActiveCategory] = useState('')
  const [activeTab, setActiveTab] = useState('tools')
  const [selected, setSelected] = useState(null)
  const [params, setParams] = useState({})
  const [searchText, setSearchText] = useState('')
  const [activeTag, setActiveTag] = useState('')
  const [result, setResult] = useState({message: '等待执行...'})
  const [pluginModalOpen, setPluginModalOpen] = useState(false)
  const [pluginUploadState, setPluginUploadState] = useState({message: '请选择插件 ZIP 包。'})
  const [configSelectedPlugin, setConfigSelectedPlugin] = useState(null)
  const [platformSettingsOpen, setPlatformSettingsOpen] = useState(false)
  const [globalEnvOpen, setGlobalEnvOpen] = useState(false)

  async function refreshCatalog(options = {}) {
    const body = await fetchJSON('/api/catalog')
    const data = body.data
    const categories = data.categories || []
    const disabledCategoryIDs = new Set(categories.filter(item => item.disabled).map(item => item.id))
    const categoryIDs = new Set(categories.map(item => item.id))
    const entryIDs = new Set([...(data.tools || []), ...(data.workflows || [])].map(item => item.id))
    setCatalog(data)
    setActiveCategory(current => current && (disabledCategoryIDs.has(current) || !categoryIDs.has(current)) ? '' : current || '')
    setSelected(current => {
      if (!current) return current
      if (current.category && disabledCategoryIDs.has(current.category)) return null
      if (current.id && !entryIDs.has(current.id)) return null
      return current
    })
    return data
  }

  useEffect(() => {
    let ignore = false
    refreshCatalog()
      .catch(err => {
        if (!ignore) setResult({message: String(err)})
      })
    return () => { ignore = true }
  }, [])

  const category = useMemo(() => {
    return catalog?.categories?.find(item => item.id === activeCategory)
  }, [catalog, activeCategory])

  const categoryDisabled = Boolean(category?.disabled)

  const hasConfigTab = useMemo(() => {
    const tools = catalog?.tools || []
    return tools.some(tool => (tool.config_file_entries && tool.config_file_entries.length > 0) || (tool.config_files && tool.config_files.length > 0))
  }, [catalog])

  const sourceEntries = useMemo(() => {
    if (!catalog || categoryDisabled) return []
    const source = activeTab === 'tools' ? catalog.tools || [] : catalog.workflows || []
    if (!activeCategory) return source
    return source.filter(item => item.category === activeCategory)
  }, [catalog, activeCategory, activeTab, categoryDisabled])

  const availableTags = useMemo(() => tagsForEntries(sourceEntries), [sourceEntries])

  const entries = useMemo(() => {
    return filterEntries(sourceEntries, searchText, activeTag)
  }, [sourceEntries, searchText, activeTag])

  function resetResult() {
    setResult({message: '等待执行...'})
  }

  function selectEntry(entry) {
    setSelected({...entry, kind: activeTab === 'tools' ? 'tool' : 'workflow'})
    const next = {}
    ;(entry.parameters || []).forEach(param => {
      next[param.name] = param.default === undefined || param.default === null ? '' : String(param.default)
    })
    setParams(next)
    resetResult()
  }

  async function runSelected() {
    if (!selected) return
    const path = selected.kind === 'tool'
      ? `/api/tools/${selected.id}/run`
      : `/api/workflows/${selected.id}/run`
    const needsConfirm = selected.confirm?.required
    if (needsConfirm && !window.confirm(selected.confirm.message || '该操作需要确认，是否继续？')) return
    setResult({message: '执行中...'})
    try {
      const body = await postJSON(path, {params, confirm: Boolean(needsConfirm)})
      if (body.id) {
        setResult({run: body, detail: await fetchRunDetail(body.id)})
        return
      }
      setResult({message: summarizeAPIResponse(body, '执行请求已提交。'), response: body})
    } catch (err) {
      setResult({message: readableAPIError(err, '执行失败。'), response: err.body})
    }
  }

  if (!catalog) {
    return <div className="loading">加载控制台...</div>
  }

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="brand">
          <span className="brandMark">OPS</span>
          <div>
            <h1>{catalog.name || '运维控制台'}</h1>
            <p>{catalog.description || '运维工具执行控制台'}</p>
          </div>
        </div>
        <div className="sectionTitle">运维分类</div>
        <div className="categoryList">
          <button
            className={activeCategory === '' ? 'category active' : 'category'}
            onClick={() => { setActiveCategory(''); setSelected(null); setActiveTag(''); resetResult() }}
          >
            <span>全局工作流</span>
            <small>跨分类选择所有可见工具和工作流</small>
          </button>
          {(catalog.categories || []).map(item => {
            const disabled = Boolean(item.disabled)
            return (
              <button
                key={item.id}
                className={`${item.id === activeCategory ? 'category active' : 'category'}${disabled ? ' disabled' : ''}`}
                disabled={disabled}
                aria-disabled={disabled}
                title={disabled ? `插件 ${item.source?.plugin_name || item.source?.plugin_id || ''} 已禁用，此分类暂不可用` : undefined}
                onClick={() => { setActiveCategory(item.id); setSelected(null); setActiveTag(''); resetResult() }}
              >
                <span>{item.name || item.id}</span>
                <small>{disabled ? `插件已禁用：${item.source?.plugin_name || item.source?.plugin_id || '未知插件'}` : item.description}</small>
              </button>
            )
          })}
        </div>
        <button className="pluginAction" onClick={() => setPluginModalOpen(true)} title="插件管理">+</button>
      </aside>

      <main className="content">
        <header className="topbar">
          <div>
            <h2>{category?.name || '全局工作流'}</h2>
            <p>{category?.description || '跨分类选择工具、工作流或打开编排器'}</p>
          </div>
          <div className="topbarActions">
            <div className="hint">可视化工作流编排</div>
            <button className="settingsButton" type="button" title="全局环境" aria-label="全局环境" onClick={() => setGlobalEnvOpen(true)}>🌐</button>
            <button className="settingsButton" type="button" title="平台设置" aria-label="平台设置" onClick={() => setPlatformSettingsOpen(true)}>⚙️</button>
          </div>
        </header>

        <div className="tabs">
          <button className={activeTab === 'tools' ? 'tab active' : 'tab'} onClick={() => { setActiveTab('tools'); setSelected(null); setActiveTag(''); resetResult() }}>工具</button>
          <button className={activeTab === 'workflows' ? 'tab active' : 'tab'} onClick={() => { setActiveTab('workflows'); setSelected(null); setActiveTag(''); resetResult() }}>工作流</button>
          <button className={activeTab === 'editor' ? 'tab active' : 'tab'} onClick={() => { setActiveTab('editor'); setSelected(null); setActiveTag(''); resetResult() }}>编排器</button>
          {hasConfigTab && <button className={activeTab === 'config' ? 'tab active' : 'tab'} onClick={() => { setActiveTab('config'); setSelected(null); setActiveTag(''); setConfigSelectedPlugin(null); resetResult() }}>配置</button>}
        </div>

        {activeTab === 'editor' ? (
          <WorkflowEditor catalog={catalog} activeCategory={activeCategory} setResult={setResult} refreshCatalog={refreshCatalog} />
        ) : (activeTab === 'config' && hasConfigTab) ? (
          <ConfigPanel catalog={catalog} activeCategory={activeCategory} configSelectedPlugin={configSelectedPlugin} setConfigSelectedPlugin={setConfigSelectedPlugin} refreshCatalog={refreshCatalog} />
        ) : (
          <RunPanel activeTab={activeTab} entries={entries} totalEntries={sourceEntries.length} selected={selected} params={params} setParams={setParams} selectEntry={selectEntry} runSelected={runSelected} searchText={searchText} setSearchText={setSearchText} activeTag={activeTag} setActiveTag={setActiveTag} availableTags={availableTags} />
        )}

        {activeTab !== 'config' && (
          <section className="card resultCard">
            <div className="cardHeader">
              <h3>执行结果</h3>
            </div>
            <ResultView result={result} />
          </section>
        )}
      </main>
      {pluginModalOpen && (
        <PluginManagerModal
          catalog={catalog}
          state={pluginUploadState}
          setState={setPluginUploadState}
          onClose={() => setPluginModalOpen(false)}
          onUploaded={async body => {
            await refreshCatalog({keepCategory: true})
            setResult({message: JSON.stringify(body, null, 2)})
          }}
          onChanged={async body => {
            await refreshCatalog({keepCategory: true})
            setResult({message: summarizeAPIResponse(body, '插件状态已更新。'), response: body})
          }}
        />
      )}
      {platformSettingsOpen && (
        <PlatformSettingsModal
          onClose={() => setPlatformSettingsOpen(false)}
          onSaved={async body => {
            await refreshCatalog()
            setPlatformSettingsOpen(false)
            setResult({message: summarizeAPIResponse(body, '平台设置已保存。'), response: body})
          }}
        />
      )}
      {globalEnvOpen && (
        <GlobalEnvModal
          onClose={() => setGlobalEnvOpen(false)}
          onSaved={async body => {
            await refreshCatalog()
            setGlobalEnvOpen(false)
            setResult({message: summarizeAPIResponse(body, '全局环境已保存。'), response: body})
          }}
        />
      )}
    </div>
  )
}

function ConfigPanel({catalog, activeCategory, configSelectedPlugin, setConfigSelectedPlugin, refreshCatalog}) {
  const configItems = useMemo(() => buildConfigItems(catalog, activeCategory), [catalog, activeCategory])

  if (configSelectedPlugin) {
    return (
      <PluginConfigPanel
        plugin={configSelectedPlugin}
        onBack={() => setConfigSelectedPlugin(null)}
        onSaved={async () => {
          await refreshCatalog()
        }}
      />
    )
  }

  return (
    <div className="grid">
      <section className="card listCard">
        <div className="cardHeader">
          <h3>配置项</h3>
          <span>{configItems.length} 项</span>
        </div>
        <ConfigItemsList items={configItems} onSelect={setConfigSelectedPlugin} />
      </section>
    </div>
  )
}

function buildConfigItems(catalog, activeCategory) {
  const items = []

  const plugins = catalog?.plugins || []
  const tools = catalog?.tools || []
  const workflows = catalog?.workflows || []

  plugins.forEach(plugin => {
    const configFiles = []
    const seenFiles = new Set()
    tools.forEach(tool => {
      if (tool.source?.plugin_id !== plugin.id) {
        return
      }
      ;(tool.config_file_entries || tool.config_files || []).forEach(file => {
        const key = typeof file === 'string' ? file : (file.id || file.path)
        if (key && !seenFiles.has(key)) {
          seenFiles.add(key)
          configFiles.push(normalizeConfigFileItem(file, plugin.id))
        }
      })
    })

    if (configFiles.length === 0) {
      return
    }

    const relatedCategories = new Set()
    tools.forEach(tool => {
      if (tool.source?.plugin_id === plugin.id && tool.category) {
        relatedCategories.add(tool.category)
      }
    })
    workflows.forEach(wf => {
      if (wf.source?.plugin_id === plugin.id && wf.category) {
        relatedCategories.add(wf.category)
      }
    })

    if (activeCategory && !relatedCategories.has(activeCategory)) {
      return
    }

    configFiles.sort((a, b) => `${a.path || ''}${a.label || ''}`.localeCompare(`${b.path || ''}${b.label || ''}`))

    items.push({
      type: 'plugin',
      id: plugin.id,
      name: plugin.name || plugin.id,
      typeLabel: '插件',
      description: plugin.description || `插件 ${plugin.id} 声明的配置文件`,
      files: configFiles,
      disabled: plugin.disabled,
      version: plugin.version,
      relatedCategories: Array.from(relatedCategories)
    })
  })

  return items
}


function normalizeConfigFileItem(file, pluginID) {
  if (typeof file === 'string') {
    return {path: `plugins/${pluginID}/${file}`, label: '配置文件'}
  }
  const scope = file.scope || 'plugin'
  const configDir = file.config_dir || (scope === 'plugin' ? 'config' : '')
  const itemPath = file.path || file.id
  const displayPath = scope === 'host_absolute' ? [configDir, itemPath].filter(Boolean).join('/') : `plugins/${pluginID}/${[configDir, itemPath].filter(Boolean).join('/')}`
  const accessLabel = file.access === 'read_write' ? '可读写' : '只读'
  return {
    path: displayPath,
    label: file.label || (scope === 'host_absolute' ? `宿主配置文件（${accessLabel}）` : '配置文件')
  }
}


function ConfigItemsList({items, onSelect}) {
  return (
    <div className="list">
      {items.map(item => (
        <button
          className={item.disabled ? 'listItem pluginDisabled' : 'listItem'}
          key={item.id}
          onClick={() => onSelect(item)}
        >
          <div>
            <strong>{item.name}</strong>
            <small className="configTypeLabel">{item.typeLabel}</small>
            {item.description && <small>{item.description}</small>}
            <div className="configFilePaths">
              {item.files.map((file, index) => (
                <small key={index} className="configFilePath">
                  {file.label && <span className="fileLabel">{file.label}：</span>}
                  {file.path}
                </small>
              ))}
            </div>
            {item.disabled && <small>状态：插件已禁用</small>}
          </div>
        </button>
      ))}
      {items.length === 0 && <div className="empty">当前没有可配置项。</div>}
    </div>
  )
}

function PluginManagerModal({catalog, state, setState, onClose, onUploaded, onChanged}) {
  const [file, setFile] = useState(null)
  const [uploading, setUploading] = useState(false)
  const [exportModalOpen, setExportModalOpen] = useState(false)
  const [pluginActionID, setPluginActionID] = useState('')
  const plugins = useMemo(() => [...(catalog?.plugins || [])].sort((left, right) => String(left.id || '').localeCompare(String(right.id || ''), 'zh-CN')), [catalog])
  const exportablePlugins = useMemo(() => plugins.filter(item => !item.disabled), [plugins])

  async function uploadPlugin(replace = false) {
    if (!file) {
      setState({message: '请先选择插件 ZIP 包。'})
      return
    }
    setUploading(true)
    setState({message: replace ? '正在更新插件...' : '正在上传插件...'})
    try {
      const body = await postPluginZip(file, replace)
      setState({message: `插件${body.status === 'updated' ? '更新' : '上传'}成功。`, response: body})
      await onUploaded(body)
    } catch (err) {
      const detail = err.body?.data
      if (err.status === 409 && detail?.existing) {
        setState({message: `插件已存在，是否更新？当前版本：${detail.existing_version || '-'}，上传版本：${detail.version || '-'}`, duplicate: true, response: err.body})
      } else {
        setState({message: String(err), response: err.body})
      }
    } finally {
      setUploading(false)
    }
  }

  async function disablePlugin(pluginID) {
    if (!pluginID) return
    setPluginActionID(pluginID)
    setState({message: `正在禁用插件 ${pluginID}...`})
    try {
      const body = await postJSON(`/api/plugins/${encodeURIComponent(pluginID)}/disable`, {})
      setState({message: `插件 ${pluginID} 已禁用，可在确认影响后删除。`, response: body})
      await onChanged(body)
    } catch (err) {
      setState({message: readableAPIError(err, '禁用插件失败。'), response: err.body})
    } finally {
      setPluginActionID('')
    }
  }

  async function enablePlugin(pluginID) {
    if (!pluginID) return
    setPluginActionID(pluginID)
    setState({message: `正在启用插件 ${pluginID}...`})
    try {
      const body = await postJSON(`/api/plugins/${encodeURIComponent(pluginID)}/enable`, {})
      setState({message: `插件 ${pluginID} 已启用。`, response: body})
      await onChanged(body)
    } catch (err) {
      setState({message: readableAPIError(err, '启用插件失败。'), response: err.body})
    } finally {
      setPluginActionID('')
    }
  }

  async function deletePlugin(pluginID) {
    if (!pluginID) return
    if (!window.confirm(`将永久删除插件 ${pluginID} 的插件目录。此操作不会删除运行日志，但无法撤销。是否继续？`)) return
    setPluginActionID(pluginID)
    setState({message: `正在删除插件 ${pluginID}...`})
    try {
      const body = await deleteJSON(`/api/plugins/${encodeURIComponent(pluginID)}`)
      setState({message: `插件 ${pluginID} 已删除。`, response: body})
      await onChanged(body)
    } catch (err) {
      setState({message: readableAPIError(err, '删除插件失败。'), response: err.body})
    } finally {
      setPluginActionID('')
    }
  }

  return (
    <div className="modalBackdrop" onClick={onClose}>
      <div className="modal" onClick={event => event.stopPropagation()}>
        <div className="modalHeader">
          <div>
            <h3>插件管理</h3>
            <p>下载插件模板、上传插件 ZIP，或按“先禁用、再删除”的流程管理已安装插件。</p>
          </div>
          <button className="modalClose" onClick={onClose}>×</button>
        </div>
        <div className="pluginModalActions">
          <a className="primary downloadTemplate" href="/api/dev/toolkit.zip">下载插件模板</a>
          <label>
            <span>上传插件 ZIP</span>
            <input type="file" accept=".zip,application/zip" onChange={event => { setFile(event.target.files?.[0] || null); setState({message: '已选择插件 ZIP，点击上传开始安装。'}) }} />
          </label>
          <div className="buttonRow">
            <button className="primary" disabled={!file || uploading} onClick={() => uploadPlugin(false)}>上传插件 ZIP</button>
            {state.duplicate && <button className="secondary" disabled={uploading} onClick={() => uploadPlugin(true)}>确认更新</button>}
          </div>
          <section className="pluginSecondaryOptions">
            <div>
              <strong>其他选项</strong>
              <span>导出用户工作流插件，或将已安装插件打包为可再次上传的 ZIP。</span>
            </div>
            <a className="secondary downloadTemplate" href="/api/plugins/user-workflows.zip">导出用户工作流插件</a>
            <button className="secondary downloadTemplate" type="button" onClick={() => setExportModalOpen(true)}>导出已安装插件</button>
          </section>
          <section className="pluginInstalledListSection">
            <div>
              <strong>已安装插件</strong>
              <span>启用插件需先禁用；禁用后可执行删除。</span>
            </div>
            <div className="pluginExportList">
              {plugins.map(item => (
                <div className={item.disabled ? 'pluginExportItem pluginDisabled' : 'pluginExportItem'} key={item.id}>
                  <div>
                    <strong>{item.name || item.id}</strong>
                    <span>{item.id}@{item.version || '-'}</span>
                    <small>{item.disabled ? '状态：已禁用' : '状态：启用中'}</small>
                    {item.description && <small>{item.description}</small>}
                  </div>
                  <div className="buttonRow pluginItemActions">
                    {item.disabled ? (
                      <>
                        <button className="secondary" disabled={pluginActionID === item.id} onClick={() => enablePlugin(item.id)}>启用</button>
                        <button className="secondary danger" disabled={pluginActionID === item.id} onClick={() => deletePlugin(item.id)}>删除</button>
                      </>
                    ) : (
                      <button className="secondary" disabled={pluginActionID === item.id} onClick={() => disablePlugin(item.id)}>禁用</button>
                    )}
                  </div>
                </div>
              ))}
              {plugins.length === 0 && <div className="empty small">当前没有已安装插件。</div>}
            </div>
          </section>
        </div>
        <pre className="modalResult">{state.response ? JSON.stringify(state.response, null, 2) : state.message}</pre>
      </div>
      {exportModalOpen && (
        <PluginExportModal plugins={exportablePlugins} onClose={() => setExportModalOpen(false)} />
      )}
    </div>
  )
}

function PlatformSettingsModal({onClose, onSaved}) {
  return (
    <div className="modalBackdrop" onMouseDown={event => { if (event.target === event.currentTarget) onClose() }}>
      <div className="modal platformSettingsModal" role="dialog" aria-modal="true" aria-labelledby="platform-settings-title">
        <div className="modalHeader">
          <div>
            <h3 id="platform-settings-title">平台设置</h3>
            <p>配置应用名称、插件目录、服务端口等基础信息</p>
          </div>
          <button className="modalClose" type="button" onClick={onClose} aria-label="关闭">×</button>
        </div>
        <GlobalConfigPanel modalMode onBack={onClose} onSaved={onSaved} />
      </div>
    </div>
  )
}

function GlobalEnvModal({onClose, onSaved}) {
  return (
    <div className="modalBackdrop" onMouseDown={event => { if (event.target === event.currentTarget) onClose() }}>
      <div className="modal platformSettingsModal" role="dialog" aria-modal="true" aria-labelledby="global-env-title">
        <div className="modalHeader">
          <div>
            <h3 id="global-env-title">全局环境</h3>
            <p>编辑全局环境配置文件（.env 格式），插件脚本可通过 $OPS_GLOBAL_ENV_FILE 访问</p>
          </div>
          <button className="modalClose" type="button" onClick={onClose} aria-label="关闭">×</button>
        </div>
        <GlobalEnvConfigPanel modalMode onBack={onClose} onSaved={onSaved} />
      </div>
    </div>
  )
}

function GlobalConfigPanel({onBack, onSaved, modalMode = false}) {
  const [content, setContent] = useState('')
  const [path, setPath] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('正在加载配置...')
  const [editMode, setEditMode] = useState('form') // 'form' | 'yaml'
  const [formData, setFormData] = useState({
    appName: '',
    appDescription: '',
    appVersion: '',
    serverEnabled: true,
    serverHost: '0.0.0.0',
    serverPort: 8080,
    pluginsPaths: 'plugins',
    pluginsStrict: false,
    pluginsAllowedCommands: '',
    pathsRuns: 'runs',
    pathsLogs: 'runs/logs',
    hostConfigAllowedDirs: ''
  })

  useEffect(() => {
    let cancelled = false
    async function loadConfig() {
      setLoading(true)
      setMessage('正在加载配置...')
      try {
        const body = await fetchJSON('/api/config/global')
        if (cancelled) return
        const yamlContent = body.data?.content || ''
        setContent(yamlContent)
        setPath(body.data?.path || 'configs/ops.yaml')

        // 解析 YAML 到表单
        try {
          const config = yaml.load(yamlContent) || {}
          setFormData({
            appName: config.app?.name || '',
            appDescription: config.app?.description || '',
            appVersion: config.app?.version || '',
            serverEnabled: config.server?.enabled !== false,
            serverHost: config.server?.host || '0.0.0.0',
            serverPort: config.server?.port || 8080,
            pluginsPaths: (config.plugins?.paths || ['plugins']).join(', '),
            pluginsStrict: config.plugins?.strict || false,
            pluginsAllowedCommands: (config.plugins?.allowed_commands || []).join('\n'),
            pathsRuns: config.paths?.runs || 'runs',
            pathsLogs: config.paths?.logs || 'runs/logs',
            hostConfigAllowedDirs: (config.host_config_files?.allowed_dirs || []).join('\n')
          })
        } catch (parseErr) {
          console.warn('YAML 解析失败，使用默认表单值', parseErr)
        }

        setMessage('配置已加载')
      } catch (err) {
        if (!cancelled) setMessage(readableAPIError(err, '加载配置失败'))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    loadConfig()
    return () => { cancelled = true }
  }, [])

  function updateFormField(field, value) {
    setFormData(prev => ({...prev, [field]: value}))
  }

  function formLines(value) {
    return value.split('\n').map(item => item.trim()).filter(Boolean)
  }

  function buildConfigFromForm(sourceContent) {
    let base = {}
    try {
      const parsed = yaml.load(sourceContent) || {}
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) base = parsed
    } catch (err) {
      base = {}
    }

    return {
      ...base,
      app: {
        ...(base.app || {}),
        name: formData.appName,
        description: formData.appDescription,
        version: formData.appVersion
      },
      server: {
        ...(base.server || {}),
        enabled: formData.serverEnabled,
        host: formData.serverHost,
        port: formData.serverPort
      },
      plugins: {
        ...(base.plugins || {}),
        paths: formData.pluginsPaths.split(',').map(p => p.trim()).filter(Boolean),
        strict: formData.pluginsStrict,
        allowed_commands: formLines(formData.pluginsAllowedCommands)
      },
      paths: {
        ...(base.paths || {}),
        runs: formData.pathsRuns,
        logs: formData.pathsLogs
      },
      host_config_files: {
        ...(base.host_config_files || {}),
        allowed_dirs: formLines(formData.hostConfigAllowedDirs)
      }
    }
  }

  function switchToYaml() {
    // 表单 → YAML：把表单字段合并回当前 YAML，避免丢失 disabled 等未展示字段
    setContent(yaml.dump(buildConfigFromForm(content), {indent: 2, lineWidth: -1}))
    setEditMode('yaml')
  }

  function switchToForm() {
    // YAML → 表单：解析当前 YAML
    try {
      const config = yaml.load(content) || {}
      setFormData({
        appName: config.app?.name || '',
        appDescription: config.app?.description || '',
        appVersion: config.app?.version || '',
        serverEnabled: config.server?.enabled !== false,
        serverHost: config.server?.host || '0.0.0.0',
        serverPort: config.server?.port || 8080,
        pluginsPaths: (config.plugins?.paths || ['plugins']).join(', '),
        pluginsStrict: config.plugins?.strict || false,
        pluginsAllowedCommands: (config.plugins?.allowed_commands || []).join('\n'),
        pathsRuns: config.paths?.runs || 'runs',
        pathsLogs: config.paths?.logs || 'runs/logs',
        hostConfigAllowedDirs: (config.host_config_files?.allowed_dirs || []).join('\n')
      })
      setEditMode('form')
    } catch (err) {
      setMessage('YAML 格式错误，无法切换到表单模式')
    }
  }

  async function saveConfig() {
    setSaving(true)
    setMessage('正在保存...')
    try {
      let yamlToSave = content
      if (editMode === 'form') {
        // 表单模式：把表单字段合并回当前 YAML，保留 plugins.disabled 等未在表单中编辑的配置
        yamlToSave = yaml.dump(buildConfigFromForm(content), {indent: 2, lineWidth: -1})
      }
      const body = await putJSON('/api/config/global', {content: yamlToSave})
      setMessage('设置已保存')
      await onSaved(body)
    } catch (err) {
      setMessage(readableAPIError(err, '保存失败'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className={modalMode ? 'platformSettingsPanel' : 'grid'}>
      <section className={modalMode ? 'platformSettingsContent' : 'card pluginConfigCard'}>
        {!modalMode && (
          <div className="cardHeader">
            <div>
              <h3>框架设置</h3>
              <p>{path} - 定义应用名称、插件路径、服务器端口等框架设置</p>
            </div>
          </div>
        )}
        <div className="pluginConfigEditor">
          <div className="empty small">配置文件路径：{path || 'configs/ops.yaml'}</div>

          {editMode === 'form' ? (
            <div className="form">
              <fieldset>
                <legend>应用信息</legend>
                <label>
                  <span>应用名称</span>
                  <input type="text" value={formData.appName} disabled={loading || saving} placeholder="运维控制台" onChange={e => updateFormField('appName', e.target.value)} />
                </label>
                <label>
                  <span>应用描述</span>
                  <input type="text" value={formData.appDescription} disabled={loading || saving} placeholder="运维工具执行控制台" onChange={e => updateFormField('appDescription', e.target.value)} />
                </label>
                <label>
                  <span>应用版本</span>
                  <input type="text" value={formData.appVersion} disabled={loading || saving} placeholder="0.1.0" onChange={e => updateFormField('appVersion', e.target.value)} />
                </label>
              </fieldset>

              <fieldset>
                <legend>服务器配置</legend>
                <label className="checkboxLabel">
                  <input type="checkbox" checked={formData.serverEnabled} disabled={loading || saving} onChange={e => updateFormField('serverEnabled', e.target.checked)} />
                  <span>启用服务器</span>
                </label>
                <label>
                  <span>监听地址</span>
                  <input type="text" value={formData.serverHost} disabled={loading || saving} placeholder="0.0.0.0" onChange={e => updateFormField('serverHost', e.target.value)} />
                </label>
                <label>
                  <span>监听端口</span>
                  <input type="number" value={formData.serverPort} disabled={loading || saving} placeholder="8080" onChange={e => updateFormField('serverPort', parseInt(e.target.value) || 8080)} />
                </label>
              </fieldset>

              <fieldset>
                <legend>插件配置</legend>
                <label>
                  <span>插件目录（逗号分隔）</span>
                  <input type="text" value={formData.pluginsPaths} disabled={loading || saving} placeholder="plugins" onChange={e => updateFormField('pluginsPaths', e.target.value)} />
                </label>
                <label className="checkboxLabel">
                  <input type="checkbox" checked={formData.pluginsStrict} disabled={loading || saving} onChange={e => updateFormField('pluginsStrict', e.target.checked)} />
                  <span>严格模式（插件加载失败时中断启动）</span>
                </label>
                <label>
                  <span>允许 PATH 命令（每行一个命令）</span>
                  <textarea className="smallTextarea" value={formData.pluginsAllowedCommands} disabled={loading || saving} placeholder="java&#10;ansible-playbook" onChange={e => updateFormField('pluginsAllowedCommands', e.target.value)} />
                </label>
                <div className="empty small">仅裸 command 命中此白名单时，才会通过运行环境 PATH 执行；带路径 command 仍必须位于插件目录内，参数请继续通过 args 数组声明。</div>
              </fieldset>

              <fieldset>
                <legend>路径配置</legend>
                <label>
                  <span>运行目录</span>
                  <input type="text" value={formData.pathsRuns} disabled={loading || saving} placeholder="runs" onChange={e => updateFormField('pathsRuns', e.target.value)} />
                </label>
                <label>
                  <span>日志目录</span>
                  <input type="text" value={formData.pathsLogs} disabled={loading || saving} placeholder="runs/logs" onChange={e => updateFormField('pathsLogs', e.target.value)} />
                </label>
              </fieldset>

              <fieldset>
                <legend>宿主配置文件白名单</legend>
                <label>
                  <span>允许目录（每行一个绝对目录）</span>
                  <textarea value={formData.hostConfigAllowedDirs} disabled={loading || saving} placeholder="/etc/myapp&#10;C:\ProgramData\Vendor" onChange={e => updateFormField('hostConfigAllowedDirs', e.target.value)} />
                </label>
                <div className="empty small">仅允许目录白名单；宿主 mapping 的 config_dir 必须落在这些目录内。</div>
              </fieldset>
            </div>
          ) : (
            <label>
              <span>配置内容（YAML 格式）</span>
              <textarea value={content} disabled={loading || saving} placeholder="app:&#10;  name: 运维控制台&#10;server:&#10;  port: 8080" onChange={event => setContent(event.target.value)} />
            </label>
          )}

          <div className="buttonRow">
            <button className="primary" disabled={loading || saving} onClick={saveConfig}>保存设置</button>
            {editMode === 'form' ? (
              <button className="secondary" disabled={loading || saving} onClick={switchToYaml}>高级编辑（YAML）</button>
            ) : (
              <button className="secondary" disabled={loading || saving} onClick={switchToForm}>返回表单</button>
            )}
            <button className="secondary" disabled={saving} onClick={onBack}>{modalMode ? '关闭' : '返回'}</button>
          </div>
          <pre className="modalResult">{message}</pre>
        </div>
      </section>
    </div>
  )
}

function GlobalEnvConfigPanel({onBack, onSaved, modalMode = false}) {
  const [content, setContent] = useState('')
  const [path, setPath] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('正在读取全局环境配置...')

  useEffect(() => {
    let cancelled = false
    async function loadConfig() {
      setLoading(true)
      setMessage('正在读取全局环境配置...')
      try {
        const body = await fetchJSON('/api/config/global-env')
        if (cancelled) return
        setContent(body.data?.content || '')
        setPath(body.data?.path || 'configs/global-env.conf')
        setMessage('已加载全局环境配置文件。')
      } catch (err) {
        if (!cancelled) setMessage(readableAPIError(err, '读取全局环境配置失败。'))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    loadConfig()
    return () => { cancelled = true }
  }, [])

  async function saveConfig() {
    setSaving(true)
    setMessage('正在保存全局环境配置...')
    try {
      const body = await putJSON('/api/config/global-env', {content})
      setMessage('全局环境配置已保存并重新加载运行时配置。')
      await onSaved(body)
    } catch (err) {
      setMessage(readableAPIError(err, '保存全局环境配置失败。'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className={modalMode ? 'platformSettingsPanel' : 'grid'}>
      <section className={modalMode ? 'platformSettingsContent' : 'card pluginConfigCard'}>
        {!modalMode && (
          <div className="cardHeader">
            <div>
              <h3>全局环境</h3>
              <p>{path} - 存储可被所有工具插件调用的全局参数（数据库连接、API密钥等）</p>
            </div>
          </div>
        )}
        <div className="pluginConfigEditor">
          <div className="empty small">编辑全局环境配置文件（.env 格式），保存后会重新加载运行时配置。插件脚本可通过 $OPS_GLOBAL_ENV_FILE 环境变量访问此文件。</div>
          <label>
            <span>.env 格式内容</span>
            <textarea value={content} disabled={loading || saving} placeholder="# 数据库配置&#10;DB_HOST=localhost&#10;DB_PORT=5432&#10;&#10;# API 配置&#10;API_BASE_URL=https://api.example.com&#10;&#10;# 环境标识&#10;ENVIRONMENT=dev" onChange={event => setContent(event.target.value)} />
          </label>
          <div className="buttonRow">
            <button className="primary" disabled={loading || saving} onClick={saveConfig}>保存全局环境配置</button>
            {!modalMode && <button className="secondary" disabled={saving} onClick={onBack}>返回列表</button>}
          </div>
          <pre className="modalResult">{message}</pre>
        </div>
      </section>
    </div>
  )
}

function PluginConfigPanel({plugin, onBack, onSaved}) {
  const pluginID = plugin?.id || ''

  return (
    <div className="grid">
      <section className="card pluginConfigCard">
        <div className="cardHeader">
          <div>
            <h3>插件配置文件</h3>
            <p>{plugin.name || pluginID}：管理插件工具使用的配置文件</p>
          </div>
        </div>
        <PluginConfigFilesPanel pluginID={pluginID} onBack={onBack} />
      </section>
    </div>
  )
}

function PluginConfigFilesPanel({pluginID, onBack}) {
  const [files, setFiles] = useState([])
  const [loading, setLoading] = useState(true)
  const [message, setMessage] = useState('正在加载配置文件列表...')
  const [editingFile, setEditingFile] = useState(null)
  const fileTree = useMemo(() => buildPluginConfigFileTree(files, pluginID), [files, pluginID])

  useEffect(() => {
    loadFiles()
  }, [pluginID])

  async function loadFiles() {
    setLoading(true)
    setMessage('正在加载配置文件列表...')
    try {
      const body = await fetchJSON(`/api/plugins/${encodeURIComponent(pluginID)}/files`)
      const fileList = (body.data?.files || []).map(normalizePluginConfigFile)
      setFiles(fileList)
      setMessage(fileList.length > 0 ? `已加载 ${fileList.length} 个配置文件。` : '当前没有配置文件。')
    } catch (err) {
      setMessage(readableAPIError(err, '加载配置文件列表失败。'))
    } finally {
      setLoading(false)
    }
  }

  if (editingFile) {
    return (
      <PluginConfigFileEditor
        pluginID={pluginID}
        file={editingFile}
        onBack={() => {
          setEditingFile(null)
          loadFiles()
        }}
      />
    )
  }

  return (
    <div className="pluginConfigFilesPanel">
      <div className="empty small">配置文件由工具声明；插件内文件可维护，宿主绝对路径文件必须由平台白名单和插件 mapping 授权。目录默认展开，可按目录折叠查找。</div>
      <div className="configFilesTree" role="tree" aria-label="配置文件目录树">
        {fileTree.children.map(node => (
          <ConfigFileTreeNode key={node.key} node={node} depth={0} onSelectFile={setEditingFile} />
        ))}
        {files.length === 0 && !loading && (
          <div className="empty">当前没有配置文件。配置文件由插件工具在 plugin.yaml 中声明。</div>
        )}
      </div>
      <div className="buttonRow">
        <button className="secondary" onClick={onBack}>返回列表</button>
      </div>
      <pre className="modalResult">{message}</pre>
    </div>
  )
}

function normalizePluginConfigFile(file) {
  if (typeof file === 'string') {
    return {
      id: file,
      label: file,
      path: file,
      displayPath: file,
      scope: 'plugin',
      access: 'read_write',
      create: true,
      exists: true,
      readable: true,
      writable: true,
      reason: ''
    }
  }
  const scope = file.scope || 'plugin'
  const path = file.path || file.id || ''
  const configDir = file.config_dir || file.configDir || ''
  const displayPath = file.display_path || file.displayPath || [configDir, path].filter(Boolean).join('/') || path || file.id || ''
  return {
    ...file,
    id: file.id || path,
    label: file.label || file.name || file.id || path,
    path,
    config_dir: configDir || file.config_dir,
    displayPath,
    scope,
    access: file.access || (scope === 'plugin' ? 'read_write' : 'read')
  }
}

function buildPluginConfigFileTree(files, pluginID) {
  const root = {key: 'root', type: 'root', children: []}
  const roots = new Map()

  files.forEach(file => {
    const scope = file.scope || 'plugin'
    const rootLabel = scope === 'host_absolute'
      ? (file.config_dir || '宿主配置')
      : `plugins/${pluginID}`
    const rootKey = `${scope}:${rootLabel}`
    let rootNode = roots.get(rootKey)
    if (!rootNode) {
      rootNode = {key: rootKey, type: 'dir', name: rootLabel, path: rootLabel, scope, children: [], childMap: new Map()}
      roots.set(rootKey, rootNode)
      root.children.push(rootNode)
    }

    const rawPath = scope === 'host_absolute'
      ? (file.path || file.displayPath || file.id || file.label || '')
      : (file.path || file.displayPath || file.id || file.label || '')
    const segments = splitConfigPath(rawPath)
    insertConfigFileTreeNode(rootNode, segments.length > 0 ? segments : [file.label || file.id || '未命名配置文件'], file)
  })

  sortConfigFileTree(root)
  return root
}

function splitConfigPath(value) {
  return String(value || '')
    .replace(/\\/g, '/')
    .split('/')
    .map(item => item.trim())
    .filter(Boolean)
}

function insertConfigFileTreeNode(rootNode, segments, file) {
  let current = rootNode
  segments.slice(0, -1).forEach(segment => {
    const key = `${current.key}/${segment}`
    let next = current.childMap.get(segment)
    if (!next) {
      next = {key, type: 'dir', name: segment, path: key, scope: rootNode.scope, children: [], childMap: new Map()}
      current.childMap.set(segment, next)
      current.children.push(next)
    }
    current = next
  })
  const fileName = segments[segments.length - 1] || file.label || file.id
  current.children.push({
    key: `file:${file.id}:${current.key}/${fileName}`,
    type: 'file',
    name: fileName,
    file
  })
}

function sortConfigFileTree(node) {
  if (!node.children) return
  node.children.sort((left, right) => {
    if (left.type !== right.type) return left.type === 'dir' ? -1 : 1
    return String(left.name || '').localeCompare(String(right.name || ''), 'zh-CN')
  })
  node.children.forEach(sortConfigFileTree)
}

function ConfigFileTreeNode({node, depth, onSelectFile}) {
  if (node.type === 'file') {
    const file = node.file
    const actionLabel = file.access === 'read_write' && file.writable !== false && !(file.exists === false && file.create === false) ? '编辑' : '查看'
    return (
      <div
        className="configFileTreeFile"
        role="treeitem"
        tabIndex={0}
        style={{'--tree-depth': depth}}
        onClick={() => onSelectFile(file)}
        onKeyDown={event => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault()
            onSelectFile(file)
          }
        }}
      >
        <div className="configFileTreeFileMain">
          <strong>{file.label || node.name || file.id}</strong>
          <span>{file.displayPath || file.path || file.id}</span>
          <ConfigFileStatusBadges file={file} />
          {file.reason && <small className="configFileReason">{file.reason}</small>}
        </div>
        <button
          type="button"
          className="secondary"
          onClick={event => {
            event.stopPropagation()
            onSelectFile(file)
          }}
        >
          {actionLabel}
        </button>
      </div>
    )
  }

  return (
    <details className="configFileTreeDir" open role="group" style={{'--tree-depth': depth}}>
      <summary role="treeitem">
        <span className="configFileTreeTwisty" aria-hidden="true">›</span>
        <strong>{node.name}</strong>
        <small>{countConfigTreeFiles(node)} 个文件</small>
      </summary>
      <div className="configFileTreeChildren">
        {node.children.map(child => (
          <ConfigFileTreeNode key={child.key} node={child} depth={depth + 1} onSelectFile={onSelectFile} />
        ))}
      </div>
    </details>
  )
}

function countConfigTreeFiles(node) {
  if (!node?.children) return 0
  return node.children.reduce((total, child) => total + (child.type === 'file' ? 1 : countConfigTreeFiles(child)), 0)
}

function ConfigFileStatusBadges({file}) {
  const badges = []
  if (file.scope === 'host_absolute') badges.push({key: 'scope', label: '宿主', kind: 'warning'})
  if (file.access === 'read_write') {
    badges.push({key: 'access', label: '可写', kind: 'success'})
  } else {
    badges.push({key: 'access', label: '只读', kind: 'muted'})
  }
  if (file.exists === false) badges.push({key: 'exists', label: '不存在', kind: 'warning'})
  if (file.readable === false) badges.push({key: 'readable', label: '不可读', kind: 'danger'})
  if (file.writable === false) badges.push({key: 'writable', label: '不可写', kind: 'danger'})
  return (
    <div className="configFileBadges" aria-label="文件状态">
      {badges.map(badge => <span key={badge.key} className={`configFileBadge ${badge.kind}`}>{badge.label}</span>)}
    </div>
  )
}

function pluginConfigFileSaveBlockReason(file) {
  if (!file) return '未选择配置文件，不能保存。'
  if (file.access !== 'read_write') return '当前文件声明为只读，只允许查看，不能保存。'
  if (file.exists === false && file.create === false) return '当前文件不存在，且声明不允许创建，不能保存。'
  if (file.writable === false) return file.reason || '当前进程对该文件或父目录没有写入权限，不能保存。'
  return ''
}

function PluginConfigFileEditor({pluginID, file, onBack}) {
  const [content, setContent] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('正在读取配置文件...')
  const fileID = file?.id || ''
  const saveBlockReason = pluginConfigFileSaveBlockReason(file)
  const canSave = !saveBlockReason

  useEffect(() => {
    loadFile()
  }, [pluginID, fileID])

  async function loadFile() {
    setLoading(true)
    setMessage('正在读取配置文件...')
    try {
      const body = await fetchJSON(`/api/plugins/${encodeURIComponent(pluginID)}/files/${encodeURIComponent(fileID)}`)
      setContent(body.data?.content || '')
      setMessage('已加载配置文件。')
    } catch (err) {
      setMessage(readableAPIError(err, '读取配置文件失败。'))
    } finally {
      setLoading(false)
    }
  }

  async function saveFile() {
    if (!canSave) {
      setMessage(saveBlockReason)
      return
    }
    setSaving(true)
    setMessage('正在保存配置文件...')
    try {
      await putJSON(`/api/plugins/${encodeURIComponent(pluginID)}/files/${encodeURIComponent(fileID)}`, {content})
      setMessage('配置文件已保存。')
    } catch (err) {
      setMessage(readableAPIError(err, '保存配置文件失败。'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="pluginConfigEditor">
      <div className="configFileEditorSummary">
        <div>
          <strong>{file?.label || fileID}</strong>
          <span>{file?.displayPath || file?.path || fileID}</span>
        </div>
        <ConfigFileStatusBadges file={file || {}} />
        {file?.reason && <small className="configFileReason">{file.reason}</small>}
        {saveBlockReason && <small className="configFileSaveHint">{saveBlockReason}</small>}
      </div>
      <label>
        <span>文件内容</span>
        <textarea
          value={content}
          disabled={loading || saving || !canSave}
          placeholder="输入配置文件内容..."
          onChange={event => setContent(event.target.value)}
        />
      </label>
      <div className="buttonRow">
        <button className="primary" disabled={loading || saving || !canSave} onClick={saveFile}>保存配置文件</button>
        <button className="secondary" disabled={saving} onClick={onBack}>返回列表</button>
      </div>
      <pre className="modalResult">{message}</pre>
    </div>
  )
}

function PluginExportModal({plugins, onClose}) {
  const [target, setTarget] = useState('linux/amd64')
  const [openPluginID, setOpenPluginID] = useState('')
  const [goos, goarch] = target.split('/')
  const targets = [
    ['linux/amd64', 'Linux amd64'],
    ['linux/arm64', 'Linux arm64'],
    ['windows/amd64', 'Windows amd64'],
    ['windows/arm64', 'Windows arm64'],
    ['darwin/amd64', 'macOS amd64'],
    ['darwin/arm64', 'macOS arm64']
  ]
  const runtimeBase = item => `/api/plugins/${encodeURIComponent(item.id)}/runtime`
  return (
    <div className="modalBackdrop modalBackdropNested" onClick={onClose}>
      <div className="modal pluginExportModal" onClick={event => event.stopPropagation()}>
        <div className="modalHeader">
          <div>
            <h3>导出已安装插件</h3>
            <p>选择插件导出标准 ZIP，或导出包含 opsctl 的单插件运行包。</p>
          </div>
          <button className="modalClose" onClick={onClose}>×</button>
        </div>
        <label className="runtimeTargetSelect">
          <span>运行包目标平台</span>
          <select value={target} onChange={event => setTarget(event.target.value)}>
            {targets.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
          <small>运行包会从服务端 base/bin 读取对应平台的 opsctl 二进制。</small>
        </label>
        <div className="pluginExportList">
          {plugins.map(item => (
            <div className="pluginExportItem" key={item.id}>
              <div>
                <strong>{item.name || item.id}</strong>
                <span>{item.id}@{item.version || '-'}</span>
                {item.description && <small>{item.description}</small>}
              </div>
              <div className="pluginExportDropdown">
                <button className="secondary" type="button" onClick={() => setOpenPluginID(openPluginID === item.id ? '' : item.id)}>导出</button>
                {openPluginID === item.id && (
                  <div className="pluginExportMenu">
                    <a href={`/api/plugins/${encodeURIComponent(item.id)}.zip`}>标准插件包 ZIP</a>
                    <a href={`${runtimeBase(item)}.tar.gz?goos=${encodeURIComponent(goos)}&goarch=${encodeURIComponent(goarch)}`}>含 opsctl 运行包 tar.gz</a>
                    <a href={`${runtimeBase(item)}.zip?goos=${encodeURIComponent(goos)}&goarch=${encodeURIComponent(goarch)}`}>含 opsctl 运行包 ZIP</a>
                  </div>
                )}
              </div>
            </div>
          ))}
          {plugins.length === 0 && <div className="empty small">当前没有可导出的已安装插件。</div>}
        </div>
      </div>
    </div>
  )
}

function ResultView({result}) {
  if (typeof result === 'string') {
    return <pre>{result}</pre>
  }
  if (result?.detail?.data) {
    return <RunDetail detail={result.detail.data} run={result.run} />
  }
  if (result?.response) {
    return <MessageWithDetails message={result.message} details={result.response} />
  }
  return <pre>{result?.message || '暂无结果'}</pre>
}

function MessageWithDetails({message, details}) {
  return (
    <div className="runDetail">
      <pre>{message || '暂无结果'}</pre>
      <details>
        <summary>查看完整响应</summary>
        <pre>{JSON.stringify(details, null, 2)}</pre>
      </details>
    </div>
  )
}

function RunDetail({detail, run}) {
  const record = detail.record || {}
  const logs = detail.logs || {}
  const combinedStepLogs = combineWorkflowStepLogs(logs.steps, record)
  return (
    <div className="runDetail">
      <div className="runSummary">
        <span>运行 ID：{record.id || run?.id}</span>
        <span>状态：{record.status || run?.status}</span>
        <span>目标：{record.target || '-'}</span>
      </div>
      {combinedStepLogs ? (
        <LogBlock title="工作流日志" value={combinedStepLogs} />
      ) : (
        <>
          <LogBlock title="标准输出" value={logs.stdout} />
          <LogBlock title="错误输出" value={logs.stderr} />
        </>
      )}
      <details>
        <summary>查看完整运行记录</summary>
        <pre>{JSON.stringify(detail, null, 2)}</pre>
      </details>
    </div>
  )
}

function LogBlock({title, value}) {
  return (
    <div className="logBlock">
      <h4>{title}</h4>
      <pre>{value || '无日志内容'}</pre>
    </div>
  )
}

function TagList({tags}) {
  if (!tags || tags.length === 0) return null
  return (
    <div className="tagList">
      {tags.map(tag => <span key={tag}>{tag}</span>)}
    </div>
  )
}

function RunPanel({activeTab, entries, totalEntries, selected, params, setParams, selectEntry, runSelected, searchText, setSearchText, activeTag, setActiveTag, availableTags}) {
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const paginationEnabled = activeTab === 'tools'
  const totalPages = paginationEnabled ? Math.max(1, Math.ceil(entries.length / pageSize)) : 1
  const safePage = Math.min(currentPage, totalPages)
  const visibleEntries = paginationEnabled ? entries.slice((safePage - 1) * pageSize, safePage * pageSize) : entries

  useEffect(() => {
    setCurrentPage(1)
  }, [activeTab, searchText, activeTag, totalEntries])

  useEffect(() => {
    setCurrentPage(page => Math.min(page, totalPages))
  }, [totalPages])

  function changePageSize(value) {
    setPageSize(Number(value))
    setCurrentPage(1)
  }

  return (
    <div className="grid">
      <section className="card listCard">
        <div className="cardHeader">
          <h3>{activeTab === 'tools' ? '工具列表' : '工作流列表'}</h3>
          <span>{entries.length} / {totalEntries} 项</span>
        </div>
        <div className="filters">
          <input value={searchText} placeholder="搜索名称、描述、ID 或标签" onChange={event => setSearchText(event.target.value)} />
          <div className="tagFilters">
            <button className={activeTag === '' ? 'tagChip active' : 'tagChip'} onClick={() => setActiveTag('')}>全部</button>
            {availableTags.map(tag => (
              <button key={tag} className={activeTag === tag ? 'tagChip active' : 'tagChip'} onClick={() => setActiveTag(tag)}>{tag}</button>
            ))}
          </div>
        </div>
        <div className="entryList">
          {visibleEntries.map(entry => (
            <button key={entry.id} className={selected?.id === entry.id ? 'entry active' : 'entry'} onClick={() => selectEntry(entry)}>
              <strong>{entry.name || entry.id}</strong>
              <span>{entry.description || '暂无描述'}</span>
              <em>{entry.id}</em>
              <TagList tags={entry.tags || []} />
            </button>
          ))}
          {entries.length === 0 && <div className="empty">没有匹配的{activeTab === 'tools' ? '工具' : '工作流'}。</div>}
        </div>
        {paginationEnabled && entries.length > 0 && (
          <div className="paginationBar">
            <label>
              <span>每页</span>
              <select value={pageSize} onChange={event => changePageSize(event.target.value)}>
                <option value="10">10</option>
                <option value="20">20</option>
                <option value="50">50</option>
              </select>
            </label>
            <div className="paginationControls">
              <button className="secondary paginationButton" disabled={safePage <= 1} onClick={() => setCurrentPage(page => Math.max(1, page - 1))}>上一页</button>
              <span>{safePage} / {totalPages}</span>
              <button className="secondary paginationButton" disabled={safePage >= totalPages} onClick={() => setCurrentPage(page => Math.min(totalPages, page + 1))}>下一页</button>
            </div>
          </div>
        )}
      </section>

      <section className="card runCard">
        <div className="cardHeader">
          <h3>执行配置</h3>
          {selected && <span>{selected.kind === 'tool' ? '工具' : '工作流'}</span>}
        </div>
        {!selected ? <div className="empty">请先选择一个工具或工作流。</div> : (
          <>
            <div className="selectedTitle">
              <h3>{selected.name || selected.id}</h3>
              <p>{selected.description}</p>
              {selected.source?.type === 'plugin' && <small>来源插件：{selected.source.plugin_name || selected.source.plugin_id}@{selected.source.plugin_version || '-'}</small>}
              {selected.confirm?.required && <small>执行前需要确认：{selected.confirm.message || '该操作需要确认'}</small>}
              <TagList tags={selected.tags || []} />
            </div>
            <div className="form">
              {(selected.parameters || []).map(param => (
                <label key={param.name}>
                  <span>{param.description || param.name}{param.required ? ' *' : ''}</span>
                  <input value={params[param.name] || ''} placeholder={param.name} onChange={event => setParams({...params, [param.name]: event.target.value})} />
                </label>
              ))}
              {(selected.parameters || []).length === 0 && <div className="empty small">无需参数。</div>}
              <button className="primary" onClick={runSelected}>开始执行</button>
            </div>
          </>
        )}
      </section>
    </div>
  )
}

function WorkflowEditor({catalog, activeCategory, setResult, refreshCatalog}) {
  const [workflow, setWorkflow] = useState(emptyWorkflow(activeCategory))
  const sidebarScopedCategory = activeCategory || ''
  const workflowOptions = useMemo(() => {
    return (catalog.workflows || []).filter(workflow => !sidebarScopedCategory || workflow.category === sidebarScopedCategory)
  }, [catalog, sidebarScopedCategory])
  const workflowScope = workflowScopeCategory(workflow.category)
  const scopedCategory = workflowScope === 'global' ? '' : workflowScope
  const toolOptions = useMemo(() => {
    return (catalog.tools || []).filter(tool => !sidebarScopedCategory || tool.category === sidebarScopedCategory)
  }, [catalog, sidebarScopedCategory])
  const workflowTagOptions = useMemo(() => tagsForEntries([...(catalog.workflows || []), workflow]), [catalog.workflows, workflow])
  const [selectedWorkflowID, setSelectedWorkflowID] = useState('')
  const [selectedNodeID, setSelectedNodeID] = useState('')
  const [selectedEdgeID, setSelectedEdgeID] = useState('')
  const [nodeConfigModalOpen, setNodeConfigModalOpen] = useState(false)
  const [edgeConfigModalOpen, setEdgeConfigModalOpen] = useState(false)
  const [workflowParamsText, setWorkflowParamsText] = useState('[]')
  const [runParamsText, setRunParamsText] = useState('{}')
  const [nodeParamsText, setNodeParamsText] = useState('{}')
  const [editorSearchText, setEditorSearchText] = useState('')
  const [editorActiveTag, setEditorActiveTag] = useState('')
  const [editorValidation, setEditorValidation] = useState(null)
  const [paletteTab, setPaletteTab] = useState('tools')
  const [flowInstance, setFlowInstance] = useState(null)
  const canvasCardRef = useRef(null)
  const [nodePicker, setNodePicker] = useState({open: false, position: null})
  const [nodePickerSearchText, setNodePickerSearchText] = useState('')
  const [nodes, setNodes, onNodesChange] = useNodesState([])
  const [edges, setEdges, onEdgesChange] = useEdgesState([])
  const [canvasRunState, setCanvasRunState] = useState(() => emptyCanvasRunState())
  const canvasRunVersionRef = useRef(0)

  const clearCanvasRunOverlay = useCallback(() => {
    canvasRunVersionRef.current += 1
    setCanvasRunState(current => current.status === 'idle' ? current : emptyCanvasRunState())
  }, [])

  const handleNodesChange = useCallback(changes => {
    if (changes.some(change => change.type === 'position' || change.type === 'remove')) {
      clearCanvasRunOverlay()
    }
    onNodesChange(changes)
  }, [clearCanvasRunOverlay, onNodesChange])

  const handleEdgesChange = useCallback(changes => {
    if (changes.some(change => change.type === 'remove')) {
      clearCanvasRunOverlay()
    }
    onEdgesChange(changes)
  }, [clearCanvasRunOverlay, onEdgesChange])

  useEffect(() => {
    setWorkflow(prev => ({...prev, category: activeCategory || 'global'}))
    setEditorActiveTag('')
    setEditorValidation(null)
    setNodeConfigModalOpen(false)
    setEdgeConfigModalOpen(false)
    clearCanvasRunOverlay()
  }, [activeCategory, clearCanvasRunOverlay])

  const selectedNode = useMemo(() => nodes.find(node => node.id === selectedNodeID), [nodes, selectedNodeID])
  const selectedEdge = useMemo(() => edges.find(edge => edge.id === selectedEdgeID), [edges, selectedEdgeID])
  const selectedTool = useMemo(() => (catalog.tools || []).find(tool => tool.id === selectedNode?.data.tool), [catalog.tools, selectedNode])
  const selectedLoopTool = useMemo(() => (catalog.tools || []).find(tool => tool.id === selectedNode?.data.loop?.tool), [catalog.tools, selectedNode])
  const editorAvailableTags = useMemo(() => tagsForEntries(toolOptions), [toolOptions])
  const editorToolOptions = useMemo(() => filterEntries(toolOptions, editorSearchText, editorActiveTag), [toolOptions, editorSearchText, editorActiveTag])
  const nodePickerToolOptions = useMemo(() => filterEntries(toolOptions, nodePickerSearchText, ''), [toolOptions, nodePickerSearchText])
  const workflowParameters = useMemo(() => parseJSONList(workflowParamsText), [workflowParamsText])
  const mappingSources = useMemo(() => buildMappingSources(workflowParameters, selectedNodeID, nodes, edges), [workflowParameters, selectedNodeID, nodes, edges])
  const displayNodes = useMemo(() => buildDisplayNodes(nodes, canvasRunState), [nodes, canvasRunState])
  const displayEdges = useMemo(() => buildDisplayEdges(edges, displayNodes, canvasRunState), [edges, displayNodes, canvasRunState])

  useEffect(() => {
    if (!selectedEdge) {
      setEdgeConfigModalOpen(false)
    }
  }, [selectedEdge])

  useEffect(() => {
    if (!selectedNode) {
      setNodeParamsText('{}')
      setNodeConfigModalOpen(false)
      return
    }
    setNodeParamsText(JSON.stringify(selectedNode.data.params || {}, null, 2))
  }, [selectedNode])

  const closeNodeConfigModal = useCallback(() => {
    setNodeConfigModalOpen(false)
  }, [])

  function openNodeConfigModal(nodeID) {
    setSelectedNodeID(nodeID)
    setSelectedEdgeID('')
    setNodeConfigModalOpen(true)
    setEdgeConfigModalOpen(false)
    closeNodePicker()
  }

  function openEdgeConfigModal(edgeID) {
    setSelectedEdgeID(edgeID)
    setSelectedNodeID('')
    setNodeConfigModalOpen(false)
    setEdgeConfigModalOpen(true)
    closeNodePicker()
  }

  const selectedNodeKindLabel = selectedNode?.type === 'conditionNode'
    ? '条件分支'
    : selectedNode?.type === 'controlNode'
      ? controlNodeTitle(selectedNode.data.controlType)
      : '工具节点'

  const onConnect = useCallback(
    params => {
      clearCanvasRunOverlay()
      setEdges(current => {
        const isDuplicate = current.some(edge => (
          edge.source === params.source &&
          edge.target === params.target &&
          (edge.sourceHandle || null) === (params.sourceHandle || null) &&
          (edge.targetHandle || null) === (params.targetHandle || null)
        ))
        if (isDuplicate) return current
        const sourceNode = nodes.find(node => node.id === params.source)
        const edgeCase = edgeCaseFromHandle(sourceNode, params.sourceHandle)
        const label = edgeCase ? conditionCaseLabel(sourceNode?.data.condition, edgeCase) : ''
        return [
          ...current,
          {
            ...params,
            id: `${params.source}-${params.target}-${params.sourceHandle || 'source'}-${params.targetHandle || 'target'}-${Date.now()}`,
            type: 'smoothstep',
            animated: true,
            label,
            data: edgeCase ? {case: edgeCase} : {}
          }
        ]
      })
    },
    [clearCanvasRunOverlay, nodes, setEdges]
  )

  async function loadWorkflow(id) {
    if (!id) return
    clearCanvasRunOverlay()
    setResult({message: '加载工作流...'})
    try {
      const body = await fetchJSON(`/api/workflows/${id}`)
      const config = body.data.Config || body.data.config
      setWorkflow(config)
      setWorkflowParamsText(JSON.stringify(config.parameters || [], null, 2))
      setRunParamsText(JSON.stringify(defaultParams(config.parameters || []), null, 2))
      const workflowNodes = config.nodes || []
      const legacyTargetMap = legacyLoopTargetMap(workflowNodes)
      const legacyTargets = new Set(legacyTargetMap.keys())
      const flowNodes = workflowNodes
        .filter(node => !legacyTargets.has(node.id))
        .map((node, index) => workflowNodeToFlowNode(node, index, removeNode, workflowNodes))
      setNodes(flowNodes)
      setEdges(remapLegacyLoopEdges(config.edges || [], legacyTargetMap).map((edge, index) => flowEdgeFromWorkflowEdge(edge, index, flowNodes)))
      setSelectedWorkflowID(id)
      setSelectedNodeID('')
      setSelectedEdgeID('')
      setNodeConfigModalOpen(false)
      setEdgeConfigModalOpen(false)
      closeNodePicker()
      setResult({message: `已加载工作流 ${id}`})
    } catch (err) {
      setResult({message: String(err)})
    }
  }

  function createWorkflow() {
    clearCanvasRunOverlay()
    const next = emptyWorkflow(activeCategory)
    setWorkflow(next)
    setWorkflowParamsText('[]')
    setRunParamsText('{}')
    setNodes([])
    setEdges([])
    setSelectedWorkflowID('')
    setSelectedNodeID('')
    setSelectedEdgeID('')
    setNodeConfigModalOpen(false)
    setEdgeConfigModalOpen(false)
    closeNodePicker()
    setResult({message: '已创建空白工作流草稿'})
  }

  const removeNode = useCallback(id => {
    clearCanvasRunOverlay()
    setNodes(current => current.filter(node => node.id !== id))
    setEdges(current => current.filter(edge => edge.source !== id && edge.target !== id))
    setResult({message: `已移除节点 ${id}`})
    setSelectedNodeID(current => current === id ? '' : current)
    setNodeConfigModalOpen(false)
    setEdgeConfigModalOpen(false)
    setSelectedEdgeID('')
  }, [clearCanvasRunOverlay, setEdges, setNodes, setResult])

  function addToolNode(tool, position) {
    clearCanvasRunOverlay()
    const nodeID = uniqueNodeID(tool.id, nodes)
    const nextNode = newToolFlowNode(tool, nodeID, position || {x: 80 + nodes.length * 220, y: 120 + (nodes.length % 3) * 90}, removeNode)
    setNodes(current => [...current, nextNode])
    setSelectedNodeID(nodeID)
    setSelectedEdgeID('')
    setEditorValidation(null)
    closeNodePicker()
    setNodeConfigModalOpen(false)
    setEdgeConfigModalOpen(false)
  }

  function addConditionNode(position) {
    clearCanvasRunOverlay()
    const nodeID = uniqueNodeID('condition', nodes)
    const nextNode = newConditionFlowNode(nodeID, position || {x: 80 + nodes.length * 220, y: 120 + (nodes.length % 3) * 90}, removeNode)
    setNodes(current => [...current, nextNode])
    setSelectedNodeID(nodeID)
    setSelectedEdgeID('')
    setEditorValidation(null)
    closeNodePicker()
    setNodeConfigModalOpen(false)
    setEdgeConfigModalOpen(false)
  }

  function addControlNode(controlType, position) {
    if (controlType === 'condition') {
      addConditionNode(position)
      return
    }
    clearCanvasRunOverlay()
    const control = controlNodeCatalog.find(item => item.type === controlType && item.enabled)
    if (!control) return
    const nodeID = uniqueNodeID(controlType, nodes)
    const nextNode = newControlFlowNode(control, nodeID, position || {x: 80 + nodes.length * 220, y: 120 + (nodes.length % 3) * 90}, removeNode)
    setNodes(current => [...current, nextNode])
    setSelectedNodeID(nodeID)
    setSelectedEdgeID('')
    setEditorValidation(null)
    closeNodePicker()
    setNodeConfigModalOpen(false)
    setEdgeConfigModalOpen(false)
  }

  function defaultCanvasInsertPosition() {
    if (flowInstance) {
      const bounds = canvasCardRef.current?.getBoundingClientRect()
      if (bounds) {
        return flowInstance.screenToFlowPosition({x: bounds.left + bounds.width / 2, y: bounds.top + bounds.height / 2})
      }
    }
    return {x: 80 + nodes.length * 220, y: 120 + (nodes.length % 3) * 90}
  }

  function openNodePicker(position) {
    clearCanvasRunOverlay()
    setSelectedNodeID('')
    setSelectedEdgeID('')
    setNodeConfigModalOpen(false)
    setEdgeConfigModalOpen(false)
    setNodePicker({open: true, position: position || defaultCanvasInsertPosition()})
    setNodePickerSearchText('')
  }

  function openNodePickerFromEvent(event) {
    event.stopPropagation()
    const position = flowInstance
      ? flowInstance.screenToFlowPosition({x: event.clientX, y: event.clientY})
      : defaultCanvasInsertPosition()
    openNodePicker(position)
  }

  function closeNodePicker() {
    setNodePicker({open: false, position: null})
  }

  function zoomCanvas(direction) {
    if (!flowInstance) return
    if (direction === 'in') {
      flowInstance.zoomIn()
      return
    }
    flowInstance.zoomOut()
  }

  function fitCanvasView() {
    flowInstance?.fitView({padding: 0.2, duration: 240})
  }

  function fitCanvasViewAfterLayout() {
    if (!flowInstance) return
    const fit = () => flowInstance.fitView({padding: 0.24, duration: 260})
    if (typeof window !== 'undefined' && window.requestAnimationFrame) {
      window.requestAnimationFrame(() => window.requestAnimationFrame(fit))
      return
    }
    fit()
  }

  function optimizeCanvasLayout() {
    clearCanvasRunOverlay()
    setNodes(current => autoLayoutNodes(current, edges))
    setResult({message: nodes.length === 0 ? '当前画布没有节点，无需优化排版。' : '已优化工作流排版。'})
    fitCanvasViewAfterLayout()
  }

  function updateSelectedNodeName(nextName) {
    clearCanvasRunOverlay()
    setNodes(current => current.map(node => node.id === selectedNodeID ? {...node, data: {...node.data, name: nextName}} : node))
    setEditorValidation(null)
  }

  function updateSelectedNodeCondition(nextCondition) {
    clearCanvasRunOverlay()
    setNodes(current => current.map(node => node.id === selectedNodeID ? {...node, data: {...node.data, condition: nextCondition}} : node))
    syncConditionEdgeLabels(selectedNodeID, nextCondition)
    setEditorValidation(null)
  }

  function updateSelectedNodeLoop(nextLoop) {
    clearCanvasRunOverlay()
    setNodes(current => current.map(node => node.id === selectedNodeID ? {...node, data: {...node.data, loop: normalizeLoopConfig(nextLoop)}} : node))
    setEditorValidation(null)
  }

  function updateSelectedLoopParam(name, value) {
    const loop = normalizeLoopConfig(selectedNode?.data.loop || defaultLoop())
    updateSelectedNodeLoop({...loop, params: {...(loop.params || {}), [name]: value}})
  }

  function syncConditionEdgeLabels(nodeID, condition) {
    setEdges(current => current.map(edge => {
      if (edge.source !== nodeID) return edge
      const edgeCase = edge.data?.case || edge.sourceHandle || ''
      const label = conditionCaseLabel(condition, edgeCase)
      return {
        ...edge,
        sourceHandle: edgeCase || edge.sourceHandle,
        label: label || edgeCase || '',
        data: edgeCase ? {...(edge.data || {}), case: edgeCase} : (edge.data || {})
      }
    }))
  }

  function updateSelectedEdgeCase(value) {
    clearCanvasRunOverlay()
    const edgeToUpdate = selectedEdge
    const sourceNode = nodes.find(node => node.id === edgeToUpdate?.source)
    const edgeCase = sourceNode?.type === 'conditionNode' ? value : ''
    const duplicate = edgeToUpdate && edges.some(edge => (
      edge.id !== edgeToUpdate.id &&
      edge.source === edgeToUpdate.source &&
      edge.target === edgeToUpdate.target &&
      (edge.sourceHandle || edge.data?.case || null) === (edgeCase || null) &&
      (edge.targetHandle || null) === (edgeToUpdate.targetHandle || null)
    ))
    if (duplicate) {
      setResult({message: '已存在相同起点、分支、终点的条件连线。'})
      return
    }
    setEdges(current => current.map(edge => {
      if (edge.id !== selectedEdgeID) return edge
      const label = conditionCaseLabel(sourceNode?.data.condition, edgeCase)
      return {
        ...edge,
        sourceHandle: edgeCase || undefined,
        label: label || edgeCase || '',
        data: edgeCase ? {...(edge.data || {}), case: edgeCase} : {}
      }
    }))
  }

  function handleToolDragStart(event, tool) {
    event.dataTransfer.setData('application/ops-tool', tool.id)
    event.dataTransfer.effectAllowed = 'move'
  }

  function handleControlDragStart(event, control) {
    event.dataTransfer.setData('application/ops-control', control.type)
    event.dataTransfer.effectAllowed = 'move'
  }

  function handleCanvasDragOver(event) {
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
  }

  function handleCanvasDrop(event) {
    event.preventDefault()
    const position = flowInstance
      ? flowInstance.screenToFlowPosition({x: event.clientX, y: event.clientY})
      : {x: event.clientX - 420, y: event.clientY - 180}
    const controlType = event.dataTransfer.getData('application/ops-control')
    if (controlType) {
      addControlNode(controlType, position)
      return
    }
    const toolID = event.dataTransfer.getData('application/ops-tool')
    const tool = (catalog.tools || []).find(item => item.id === toolID)
    if (!tool) return
    addToolNode(tool, position)
  }

  function removeSelectedNode() {
    if (!selectedNodeID) return
    removeNode(selectedNodeID)
  }

  function removeSelectedEdge() {
    if (!selectedEdgeID) return
    clearCanvasRunOverlay()
    setEdges(current => current.filter(edge => edge.id !== selectedEdgeID))
    setResult({message: `已移除依赖 ${selectedEdgeID}`})
    setSelectedEdgeID('')
    setEdgeConfigModalOpen(false)
  }

  function clearSelection() {
    setNodeConfigModalOpen(false)
    setEdgeConfigModalOpen(false)
    setSelectedNodeID('')
    setSelectedEdgeID('')
  }

  function updateWorkflowCategory(value) {
    if (activeCategory && value !== activeCategory) return
    clearCanvasRunOverlay()
    setWorkflow(current => ({...current, category: workflowScopeCategory(value)}))
    setEditorValidation(null)
  }

  function updateWorkflowTags(nextTags) {
    clearCanvasRunOverlay()
    setWorkflow(current => ({...current, tags: normalizeTags(nextTags)}))
    setEditorValidation(null)
  }

  function applyNodeParams() {
    if (!selectedNodeID) return false
    try {
      updateSelectedNodeParams(JSON.parse(nodeParamsText || '{}'))
      setResult({message: `已更新节点 ${selectedNodeID} 参数`})
      return true
    } catch (err) {
      setResult({message: `参数（JSON） 无效: ${err.message}`})
      return false
    }
  }

  function updateSelectedNodeParams(nextParams) {
    clearCanvasRunOverlay()
    setNodes(current => current.map(node => node.id === selectedNodeID ? {...node, data: {...node.data, params: nextParams}} : node))
    setNodeParamsText(JSON.stringify(nextParams, null, 2))
    setEditorValidation(null)
  }

  function updateMappedParam(name, value) {
    const nextParams = {...(selectedNode?.data.params || {}), [name]: value}
    updateSelectedNodeParams(nextParams)
  }

  function showEditorValidation(status) {
    setEditorValidation(status)
    setResult({message: formatPreflightMessage(status)})
  }

  function preflightWorkflowDraft(mode) {
    const errors = []
    let workflowParameters = []
    try {
      workflowParameters = JSON.parse(workflowParamsText || '[]')
      if (!Array.isArray(workflowParameters)) {
        errors.push('工作流参数必须是 JSON 数组。')
        workflowParameters = []
      }
    } catch (err) {
      errors.push(`工作流参数 JSON 无效：${err.message}`)
    }
    const draft = buildWorkflowDraft(workflow, nodes, edges, activeCategory, workflowParameters)
    validateConditionDraft(nodes, edges).forEach(error => errors.push(error))
    validateControlDraft(nodes, edges, catalog.tools || []).forEach(error => errors.push(error))
    if (activeCategory && draft.category !== activeCategory) errors.push(`当前分类上下文只能保存为 ${activeCategory} 工作流。`)
    if (!String(draft.id || '').trim()) {
      if (mode === 'save') {
        errors.push('请先填写工作流 ID。')
      } else {
        draft.id = 'draft'
      }
    }
    if (!String(draft.name || '').trim()) {
      if (mode === 'save') {
        errors.push('请先填写工作流名称。')
      } else {
        draft.name = '未保存草稿'
      }
    }
    if (nodes.length === 0) errors.push('请至少添加一个工作流节点。')
    findOutOfScopeToolNodes(nodes, catalog.tools || [], scopedCategory).forEach(item => {
      errors.push(`节点 ${item.nodeID}（${item.toolID}）不属于当前工作流工具范围：${item.scopeName}`)
    })
    findMissingRequiredNodeParams(nodes, catalog.tools || []).forEach(item => {
      errors.push(`节点 ${item.nodeID}（${item.toolName}）缺少必填参数：${item.paramName}`)
    })
    const title = mode === 'save' ? '保存前检查未通过' : mode === 'run' ? '执行前检查未通过' : '校验前检查未通过'
    return {draft, errors, warnings: [], title}
  }

  async function validateDraft() {
    const check = preflightWorkflowDraft('validate')
    if (check.errors.length > 0) {
      showEditorValidation(check)
      return
    }
    try {
      setEditorValidation(null)
      setWorkflow(check.draft)
      const body = await postJSON(`/api/workflows/${check.draft.id || 'draft'}/validate`, {workflow: check.draft})
      setResult({message: summarizeAPIResponse(body, '工作流校验通过。'), response: body})
    } catch (err) {
      setResult({message: readableAPIError(err, '工作流校验失败。'), response: err.body})
    }
  }

  async function saveDraft() {
    const check = preflightWorkflowDraft('save')
    if (check.errors.length > 0) {
      showEditorValidation(check)
      return
    }
    try {
      setEditorValidation(null)
      setWorkflow(check.draft)
      const body = await postJSON(`/api/workflows/${check.draft.id}/save`, {workflow: check.draft})
      setSelectedWorkflowID(check.draft.id)
      await refreshCatalog({keepCategory: true})
      setResult({message: summarizeAPIResponse(body, '工作流保存成功。'), response: body})
    } catch (err) {
      setResult({message: readableAPIError(err, '工作流保存失败。'), response: err.body})
    }
  }

  async function runDraft() {
    const check = preflightWorkflowDraft('run')
    if (check.errors.length > 0) {
      clearCanvasRunOverlay()
      showEditorValidation(check)
      return
    }
    const runVersion = canvasRunVersionRef.current + 1
    canvasRunVersionRef.current = runVersion
    const runNodesSnapshot = nodes
    setCanvasRunState(buildRunningCanvasRunState(runNodesSnapshot))
    const applyRunOverlay = nextState => {
      if (canvasRunVersionRef.current !== runVersion) return
      setCanvasRunState(nextState)
    }
    try {
      setEditorValidation(null)
      let runParams = {}
      try {
        runParams = JSON.parse(runParamsText || '{}')
      } catch (err) {
        applyRunOverlay(buildFailedCanvasRunState(runNodesSnapshot, `执行参数 JSON 无效：${err.message}`))
        const status = {title: '执行前检查未通过', errors: [`执行参数 JSON 无效：${err.message}`], warnings: []}
        showEditorValidation(status)
        return
      }
      setResult({message: '执行工作流...'})
      const body = await postJSON(`/api/workflows/${check.draft.id}/run`, {params: runParams, workflow: check.draft})
      if (body.id) {
        const detail = await fetchRunDetail(body.id)
        applyRunOverlay(buildCanvasRunStateFromDetail(detail?.data || detail, runNodesSnapshot))
        setResult({run: body, detail})
        return
      }
      applyRunOverlay(buildFailedCanvasRunState(runNodesSnapshot, summarizeAPIResponse(body, '工作流已提交执行，但没有返回运行记录。')))
      setResult({message: summarizeAPIResponse(body, '工作流已提交执行。'), response: body})
    } catch (err) {
      const body = err.body || {}
      const runID = body.id || body.ID
      let detail = extractRunDetailFromResponse(body)
      if (!detail && runID) {
        try {
          const fetched = await fetchRunDetail(runID)
          detail = fetched?.data || fetched
        } catch {
          detail = null
        }
      }
      if (detail?.record) {
        applyRunOverlay(buildCanvasRunStateFromDetail(detail, runNodesSnapshot, readableAPIError(err, '工作流执行失败。')))
      } else {
        applyRunOverlay(buildFailedCanvasRunState(runNodesSnapshot, readableAPIError(err, '工作流执行失败。')))
      }
      setResult({message: readableAPIError(err, '工作流执行失败。'), response: body, detail: detail ? {data: detail} : undefined, run: runID ? {id: runID, status: body.status || 'failed'} : undefined})
    }
  }

  return (
    <div className="editorLayout">
      <section className="card editorToolbar">
        <div className="cardHeader">
          <h3>工作流编排器</h3>
          <span>{nodes.length} 节点 / {edges.length} 依赖</span>
        </div>
        {editorValidation?.errors?.length > 0 && <ValidationSummary status={editorValidation} />}
        <div className="form compact">
          <label>
            <span>加载已有工作流</span>
            <select value={selectedWorkflowID} onChange={event => loadWorkflow(event.target.value)}>
              <option value="">选择工作流...</option>
              {workflowOptions.map(item => <option key={item.id} value={item.id}>{item.name || item.id}</option>)}
            </select>
          </label>
          <div className="buttonRow">
            <button className="secondary" onClick={createWorkflow}>新建</button>
            <button className="secondary" onClick={validateDraft}>校验</button>
            <button className="secondary" onClick={runDraft}>执行</button>
            <button className="primary" onClick={saveDraft}>保存</button>
          </div>
          <div className="buttonRow">
            <button className="secondary danger" onClick={removeSelectedNode} disabled={!selectedNode}>删除节点</button>
            <button className="secondary danger" onClick={removeSelectedEdge} disabled={!selectedEdge}>删除依赖</button>
            <button className="secondary" onClick={clearSelection} disabled={!selectedNode && !selectedEdge}>取消选择</button>
          </div>
          <label>
            <span>工作流 ID</span>
            <input value={workflow.id || ''} onChange={event => { clearCanvasRunOverlay(); setWorkflow({...workflow, id: event.target.value}) }} placeholder="demo.my-flow" />
          </label>
          <label>
            <span>名称</span>
            <input value={workflow.name || ''} onChange={event => { clearCanvasRunOverlay(); setWorkflow({...workflow, name: event.target.value}) }} placeholder="工作流名称" />
          </label>
          <label>
            <span>描述</span>
            <input value={workflow.description || ''} onChange={event => { clearCanvasRunOverlay(); setWorkflow({...workflow, description: event.target.value}) }} placeholder="工作流描述" />
          </label>
          <label>
            <span>分类 / 范围</span>
            <select value={workflow.category || 'global'} onChange={event => updateWorkflowCategory(event.target.value)} disabled={Boolean(activeCategory)}>
              {!activeCategory && <option value="global">全局工作流（可选择全部工具）</option>}
              {(catalog.categories || [])
                .filter(item => !item.disabled && (!activeCategory || item.id === activeCategory))
                .map(item => <option key={item.id} value={item.id}>{item.name || item.id}（仅当前分类工具）</option>)}
            </select>
          </label>
          <WorkflowTagsEditor tags={workflow.tags || []} availableTags={workflowTagOptions} onChange={updateWorkflowTags} />
          <label>
            <span>工作流参数（JSON）</span>
            <textarea className="smallTextarea" value={workflowParamsText} onChange={event => { clearCanvasRunOverlay(); setWorkflowParamsText(event.target.value) }} />
          </label>
          <label>
            <span>执行参数（JSON）</span>
            <textarea className="smallTextarea" value={runParamsText} onChange={event => { clearCanvasRunOverlay(); setRunParamsText(event.target.value) }} />
          </label>
        </div>
      </section>

      <section className="card editorPalette">
        <div className="cardHeader">
          <h3>节点面板</h3>
          <span>{paletteTab === 'tools' ? `${editorToolOptions.length} / ${toolOptions.length} 项` : `${controlNodeCatalog.length} 项`}</span>
        </div>
        <div className="paletteTabs" role="tablist" aria-label="节点类型">
          <button
            type="button"
            role="tab"
            aria-selected={paletteTab === 'tools'}
            className={paletteTab === 'tools' ? 'paletteTab active' : 'paletteTab'}
            onClick={() => setPaletteTab('tools')}
          >
            插件工具
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={paletteTab === 'control'}
            className={paletteTab === 'control' ? 'paletteTab active' : 'paletteTab'}
            onClick={() => setPaletteTab('control')}
          >
            编排节点
          </button>
        </div>
        {paletteTab === 'tools' ? (
          <>
            <div className="filters editorPaletteFilters">
              <input value={editorSearchText} placeholder="搜索工具名称、描述、ID 或标签" onChange={event => setEditorSearchText(event.target.value)} />
              <div className="tagFilters">
                <button className={editorActiveTag === '' ? 'tagChip active' : 'tagChip'} onClick={() => setEditorActiveTag('')}>全部</button>
                {editorAvailableTags.map(tag => (
                  <button key={tag} className={editorActiveTag === tag ? 'tagChip active' : 'tagChip'} onClick={() => setEditorActiveTag(tag)}>{tag}</button>
                ))}
              </div>
            </div>
            <div className="toolPalette" role="tabpanel" aria-label="插件工具">
              {editorToolOptions.map(tool => (
                <button key={tool.id} type="button" className="paletteItem" draggable onDragStart={event => handleToolDragStart(event, tool)} onClick={() => addToolNode(tool)} title={`${tool.name || tool.id}\n${tool.id}${tool.description ? `\n${tool.description}` : ''}`}>
                  <span className="paletteShape tool" aria-hidden="true" />
                  <span className="paletteItemLabel">{tool.name || tool.id}</span>
                </button>
              ))}
              {editorToolOptions.length === 0 && <div className="empty small">没有匹配的插件工具。</div>}
            </div>
          </>
        ) : (
          <div className="controlPalette" role="tabpanel" aria-label="编排节点">
            {controlNodeCatalog.map(control => {
              const disabled = !control.enabled
              const cardClassName = disabled ? 'paletteItem controlPaletteItem disabled' : 'paletteItem controlPaletteItem'
              return (
                <button
                  key={control.type}
                  type="button"
                  className={cardClassName}
                  draggable={!disabled}
                  disabled={disabled}
                  aria-disabled={disabled}
                  onDragStart={disabled ? undefined : event => handleControlDragStart(event, control)}
                  onClick={disabled ? undefined : () => addControlNode(control.type)}
                  title={`${control.title}\n${control.secondary}\n${control.help || control.description}${disabled ? '\n规划中' : ''}`}
                >
                  <span className={`paletteShape ${control.type}`} data-symbol={controlShapeMarker(control.type)} aria-hidden="true" />
                  <span className="paletteItemLabel">{control.title}</span>
                </button>
              )
            })}
          </div>
        )}
      </section>

      <section className="card canvasCard" ref={canvasCardRef} onDragOver={handleCanvasDragOver} onDrop={handleCanvasDrop}>
        <ReactFlow
          nodes={displayNodes}
          edges={displayEdges}
          nodeTypes={nodeTypes}
          onNodesChange={handleNodesChange}
          onEdgesChange={handleEdgesChange}
          onConnect={onConnect}
          onNodeClick={(_, node) => openNodeConfigModal(node.id)}
          onEdgeClick={(_, edge) => openEdgeConfigModal(edge.id)}
          onPaneClick={() => { clearSelection(); closeNodePicker() }}
          onInit={setFlowInstance}
          fitView
        >
          <MiniMap />
          <Controls />
          <Background />
          {nodes.length === 0 && !nodePicker.open && (
            <div className="canvasEmptyCallout nodrag nopan" onMouseDown={event => event.stopPropagation()}>
              <strong>从添加节点开始编排</strong>
              <span>搜索插件工具或添加条件分支，仍可从节点面板拖拽到画布。</span>
              <button type="button" className="secondary" onClick={openNodePickerFromEvent}>添加节点</button>
            </div>
          )}
          {nodePicker.open && (
            <NodePickerPanel
              searchText={nodePickerSearchText}
              setSearchText={setNodePickerSearchText}
              tools={nodePickerToolOptions}
              totalTools={toolOptions.length}
              position={nodePicker.position}
              onAddTool={tool => addToolNode(tool, nodePicker.position)}
              onAddControl={controlType => addControlNode(controlType, nodePicker.position)}
              onClose={closeNodePicker}
            />
          )}
          <CanvasDock
            onZoomIn={() => zoomCanvas('in')}
            onZoomOut={() => zoomCanvas('out')}
            onFitView={fitCanvasView}
            onAutoLayout={optimizeCanvasLayout}
            onAddNode={() => openNodePicker()}
            onRunWorkflow={runDraft}
          />
        </ReactFlow>
      </section>

      {nodeConfigModalOpen && selectedNode && (
        <NodeConfigModal
          node={selectedNode}
          kindLabel={selectedNodeKindLabel}
          onClose={closeNodeConfigModal}
          onSave={() => {
            if (selectedNode.type === 'toolNode' && !applyNodeParams()) return
            closeNodeConfigModal()
          }}
        >
          <NodeConfigEditor
            node={selectedNode}
            tool={selectedTool}
            sources={mappingSources}
            paramsText={nodeParamsText}
            setParamsText={setNodeParamsText}
            onNameChange={updateSelectedNodeName}
            onConditionChange={updateSelectedNodeCondition}
            onLoopChange={updateSelectedNodeLoop}
            loopTool={selectedLoopTool}
            tools={toolOptions}
            onParamChange={updateMappedParam}
            onLoopParamChange={updateSelectedLoopParam}
            onApplyParams={applyNodeParams}
          />
        </NodeConfigModal>
      )}

      {edgeConfigModalOpen && selectedEdge && (
        <EdgeConfigModal
          edge={selectedEdge}
          sourceNode={nodes.find(node => node.id === selectedEdge.source)}
          onCaseChange={updateSelectedEdgeCase}
          onClose={() => setEdgeConfigModalOpen(false)}
        />
      )}
    </div>
  )
}

function NodeConfigModal({node, kindLabel, children, onClose, onSave}) {
  return (
    <div className="modalBackdrop" onClick={onClose}>
      <div className="modal nodeConfigModal" onClick={event => event.stopPropagation()}>
        <div className="modalHeader">
          <div>
            <h3>节点配置</h3>
            <p>{kindLabel} · {node.data.name || node.id}</p>
          </div>
          <button type="button" className="modalClose" onClick={onClose}>×</button>
        </div>
        {children}
        <div className="modalFooter">
          <button type="button" className="secondary" onClick={onClose}>取消</button>
          <button type="button" className="primary" onClick={onSave}>保存配置</button>
        </div>
      </div>
    </div>
  )
}

function NodeConfigEditor({node, tool, loopTool, tools, sources, paramsText, setParamsText, onNameChange, onConditionChange, onLoopChange, onParamChange, onLoopParamChange, onApplyParams}) {
  if (!node) return null
  return (
    <div className="form nodeConfigEditor">
      {node.type === 'conditionNode' ? (
        <ConditionEditor node={node} sources={sources} onNameChange={onNameChange} onChange={onConditionChange} />
      ) : node.type === 'controlNode' ? (
        <ControlNodeInspector node={node} tools={tools} loopTool={loopTool} sources={sources} onNameChange={onNameChange} onLoopChange={onLoopChange} onLoopParamChange={onLoopParamChange} />
      ) : (
        <ToolNodeConfigEditor
          node={node}
          tool={tool}
          sources={sources}
          paramsText={paramsText}
          setParamsText={setParamsText}
          onParamChange={onParamChange}
          onApplyParams={onApplyParams}
        />
      )}
    </div>
  )
}

function ToolNodeConfigEditor({node, tool, sources, paramsText, setParamsText, onParamChange, onApplyParams}) {
  return (
    <>
      <label>
        <span>节点标识</span>
        <input value={node.id} disabled />
      </label>
      <label>
        <span>工具标识</span>
        <input value={node.data.tool} disabled />
      </label>
      <ParamMappingEditor tool={tool} params={node.data.params || {}} sources={sources} onChange={onParamChange} />
      <details className="advancedParams">
        <summary>高级 JSON 编辑</summary>
        <label>
          <span>参数（JSON）</span>
          <textarea value={paramsText} onChange={event => setParamsText(event.target.value)} />
        </label>
        <div className="empty small">可引用上游节点：{"{{ .steps.节点ID.stdout }}"} 或 {"{{ .steps.节点ID.params.参数名 }}"}</div>
        <button type="button" className="secondary" onClick={() => onApplyParams()}>应用参数</button>
      </details>
    </>
  )
}

function NodePickerPanel({searchText, setSearchText, tools, totalTools, onAddTool, onAddControl, onClose}) {
  const keyword = searchText.trim().toLowerCase()
  const matchingControls = controlNodeCatalog
    .filter(control => control.enabled)
    .filter(control => !keyword || [control.title, control.secondary, control.description, control.help]
      .filter(Boolean)
      .some(value => String(value).toLowerCase().includes(keyword)))
  return (
    <div className="nodePickerLayer nodrag nopan" onMouseDown={event => event.stopPropagation()}>
      <div className="nodePickerPanel">
        <div className="nodePickerHeader">
          <div>
            <strong>添加节点</strong>
            <span>选择工具或编排节点</span>
          </div>
          <button type="button" className="modalClose" onClick={onClose}>×</button>
        </div>
        <input value={searchText} placeholder="搜索工具名称、描述或 ID" onChange={event => setSearchText(event.target.value)} autoFocus />
        {matchingControls.length > 0 && (
          <div className="nodePickerSection">
            <span>编排节点</span>
            {matchingControls.map(control => (
              <button key={control.type} type="button" className="nodePickerItem control" onClick={() => onAddControl(control.type)} title={`${control.title}\n${control.secondary}\n${control.help || control.description}`}>
                <span className={`paletteShape ${control.type}`} data-symbol={controlShapeMarker(control.type)} aria-hidden="true" />
                <b>{control.title}</b>
              </button>
            ))}
          </div>
        )}
        <div className="nodePickerSection">
          <span>插件工具 · {tools.length} / {totalTools}</span>
          <div className="nodePickerList">
            {tools.map(tool => (
              <button key={tool.id} type="button" className="nodePickerItem" onClick={() => onAddTool(tool)} title={`${tool.name || tool.id}\n${tool.id}${tool.description ? `\n${tool.description}` : ''}`}>
                <span className="paletteShape tool" aria-hidden="true" />
                <b>{tool.name || tool.id}</b>
              </button>
            ))}
            {tools.length === 0 && <div className="empty small">没有匹配的插件工具。</div>}
          </div>
        </div>
      </div>
    </div>
  )
}

function CanvasDock({onZoomIn, onZoomOut, onFitView, onAutoLayout, onAddNode, onRunWorkflow}) {
  return (
    <div className="canvasDock nodrag nopan" onMouseDown={event => event.stopPropagation()}>
      <button type="button" onClick={onZoomOut} title="缩小">−</button>
      <button type="button" onClick={onZoomIn} title="放大">+</button>
      <button type="button" onClick={onFitView}>适配视图</button>
      <button type="button" onClick={onAutoLayout}>优化排版</button>
      <button type="button" onClick={onAddNode}>添加节点</button>
      <button type="button" className="canvasDockPrimary" onClick={onRunWorkflow}>运行工作流</button>
    </div>
  )
}

function EdgeConfigModal({edge, sourceNode, onCaseChange, onClose}) {
  return (
    <div className="modalBackdrop" onClick={onClose}>
      <div className="modal edgeConfigModal" onClick={event => event.stopPropagation()}>
        <div className="modalHeader">
          <div>
            <h3>连线配置</h3>
            <p>{edge.source} → {edge.target}</p>
          </div>
          <button type="button" className="modalClose" onClick={onClose}>×</button>
        </div>
        <EdgeInspector edge={edge} sourceNode={sourceNode} onCaseChange={onCaseChange} />
        <div className="modalFooter">
          <button type="button" className="primary" onClick={onClose}>完成</button>
        </div>
      </div>
    </div>
  )
}

function EdgeInspector({edge, sourceNode, onCaseChange}) {
  if (sourceNode?.type !== 'conditionNode') {
    return <div className="empty small">已选择依赖线，可使用左侧工具栏“删除依赖”。</div>
  }
  const condition = sourceNode.data.condition || defaultCondition()
  const currentCase = edge.data?.case || edge.sourceHandle || ''
  const showDefaultOption = condition.default_case === 'default' || currentCase === 'default'
  return (
    <div className="edgeInspector">
      <strong>条件分支连线</strong>
      <select value={currentCase} onChange={event => onCaseChange(event.target.value)}>
        <option value="">请选择 case...</option>
        {(condition.cases || []).map(item => <option key={item.id} value={item.id}>{item.name || item.id}</option>)}
        {showDefaultOption && <option value="default">默认分支 default</option>}
      </select>
      <div className="empty small">从条件节点发出的连线必须选择 case，标签会显示在连线上。</div>
    </div>
  )
}

function ConditionEditor({node, sources, onNameChange, onChange}) {
  const condition = node.data.condition || defaultCondition()
  function updateCase(index, patch) {
    const cases = (condition.cases || []).map((item, current) => current === index ? {...item, ...patch} : item)
    onChange({...condition, cases})
  }
  function addCase() {
    const index = (condition.cases || []).length + 1
    onChange({...condition, cases: [...(condition.cases || []), {id: `case_${index}`, name: `分支 ${index}`, operator: 'contains', values: ['']}]})
  }
  function removeCase(index) {
    onChange({...condition, cases: (condition.cases || []).filter((_, current) => current !== index)})
  }
  return (
    <div className="conditionEditor">
      <label>
        <span>节点标识</span>
        <input value={node.id} disabled />
      </label>
      <label>
        <span>显示名称</span>
        <input value={node.data.name || ''} placeholder="按巡检结果分支" onChange={event => onNameChange(event.target.value)} />
      </label>
      <label>
        <span>条件输入</span>
        <select value={condition.input || ''} onChange={event => onChange({...condition, input: event.target.value})}>
          <option value="">选择上游输出或工作流参数...</option>
          {sources.map(source => <option key={source.value} value={source.value}>{source.label}</option>)}
        </select>
        <input value={condition.input || ''} placeholder="{{ .steps.inspect.stdout }}" onChange={event => onChange({...condition, input: event.target.value})} />
      </label>
      <label>
        <span>默认分支</span>
        <select value={condition.default_case || ''} onChange={event => onChange({...condition, default_case: event.target.value})}>
          <option value="default">启用 default 分支</option>
          <option value="">不启用</option>
        </select>
      </label>
      <div className="caseList">
        <strong>Case 分支</strong>
        {(condition.cases || []).map((item, index) => (
          <div className="caseEditor" key={`${item.id}-${index}`}>
            <input value={item.id || ''} placeholder="case_id" onChange={event => updateCase(index, {id: event.target.value})} />
            <input value={item.name || ''} placeholder="分支名称" onChange={event => updateCase(index, {name: event.target.value})} />
            <select value={item.operator || 'contains'} onChange={event => updateCase(index, {operator: event.target.value})}>
              {conditionOperators.map(operator => <option key={operator.value} value={operator.value}>{operator.label}</option>)}
            </select>
            <textarea className="smallTextarea" value={(item.values || []).join('\n')} placeholder="匹配值；多个值可换行或用逗号分隔" onChange={event => updateCase(index, {values: updateCaseValuesText(event.target.value)})} />
            <button className="secondary danger" onClick={() => removeCase(index)}>删除 case</button>
          </div>
        ))}
        <button className="secondary" onClick={addCase}>添加 case</button>
      </div>
    </div>
  )
}

function ControlNodeInspector({node, tools, loopTool, sources, onNameChange, onLoopChange, onLoopParamChange}) {
  const isLoop = node.data.controlType === 'loop'
  const loop = normalizeLoopConfig(node.data.loop || defaultLoop())
  function updateLoop(patch) {
    onLoopChange(normalizeLoopConfig({...loop, ...patch}))
  }
  function changeLoopTool(toolID) {
    const tool = (tools || []).find(item => item.id === toolID)
    updateLoop({tool: toolID, params: defaultParams(tool?.parameters || [])})
  }
  return (
    <div className="controlNodeInspector">
      <label>
        <span>节点标识</span>
        <input value={node.id} disabled />
      </label>
      <label>
        <span>节点类型</span>
        <input value={controlNodeTitle(node.data.controlType)} disabled />
      </label>
      <label>
        <span>显示名称</span>
        <input value={node.data.name || ''} placeholder={controlNodeTitle(node.data.controlType)} onChange={event => onNameChange(event.target.value)} />
      </label>
      {isLoop ? (
        <>
          <label>
            <span>循环工具</span>
            <select value={loop.tool || ''} onChange={event => changeLoopTool(event.target.value)}>
              <option value="">选择要重复执行的插件工具...</option>
              {(tools || []).map(tool => <option key={tool.id} value={tool.id}>{tool.name || tool.id}（{tool.id}）</option>)}
            </select>
          </label>
          <label>
            <span>最大循环次数</span>
            <input type="number" min="1" max="20" value={loop.max_iterations || 1} onChange={event => updateLoop({max_iterations: event.target.value})} />
          </label>
          <ParamMappingEditor tool={loopTool} params={loop.params || {}} sources={sources} onChange={onLoopParamChange} emptyMessage="请先选择循环工具，再配置该工具的输入参数。" />
          <div className="empty small">循环节点会重复执行上方选择的工具，不需要在画布上额外添加目标工具节点；普通连线仍保持 DAG。</div>
        </>
      ) : (
        <div className="empty small">{controlNodeHelp(node.data.controlType)}。该节点不需要配置工具或参数，运行时自身记录为成功。</div>
      )}
    </div>
  )
}

function WorkflowTagsEditor({tags, availableTags, onChange}) {
  const [draftTag, setDraftTag] = useState('')
  function addTag(value) {
    const tag = String(value || '').trim()
    if (!tag) return
    onChange([...(tags || []), tag])
    setDraftTag('')
  }
  function removeTag(tag) {
    onChange((tags || []).filter(item => item !== tag))
  }
  const candidates = (availableTags || []).filter(tag => !(tags || []).includes(tag))
  return (
    <div className="workflowTagsEditor">
      <span>标签</span>
      <div className="tagList editable">
        {(tags || []).map(tag => (
          <button key={tag} type="button" className="tagChip active" onClick={() => removeTag(tag)} title="点击移除标签">{tag} ×</button>
        ))}
        {(tags || []).length === 0 && <small>暂无标签，可选择已有标签或输入新标签。</small>}
      </div>
      <div className="tagFilters selectableTags">
        {candidates.map(tag => <button key={tag} type="button" className="tagChip" onClick={() => addTag(tag)}>{tag}</button>)}
      </div>
      <div className="tagInputRow">
        <input value={draftTag} placeholder="输入新标签，回车添加" onChange={event => setDraftTag(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') { event.preventDefault(); addTag(draftTag) } }} />
        <button type="button" className="secondary" onClick={() => addTag(draftTag)}>添加标签</button>
      </div>
    </div>
  )
}

function ValidationSummary({status}) {
  return (
    <div className="validationSummary">
      <strong>{status.title || '检查未通过'}</strong>
      <ul>
        {(status.errors || []).map(error => <li key={error}>{error}</li>)}
      </ul>
    </div>
  )
}

function ParamMappingEditor({tool, params, sources, onChange, emptyMessage}) {
  const parameters = tool?.parameters || []
  if (!tool) return <div className="empty small">{emptyMessage || '未找到当前节点工具定义。'}</div>
  if (parameters.length === 0) return <div className="empty small">当前工具没有声明输入参数。</div>
  return (
    <div className="paramMappings">
      <strong>输入参数映射</strong>
      {parameters.map(param => (
        <div key={param.name} className="mappingRow">
          <div>
            <span>{param.description || param.name}{param.required ? ' *' : ''}</span>
            <em>{param.name}</em>
          </div>
          <select value={params[param.name] || ''} onChange={event => onChange(param.name, event.target.value)}>
            <option value="">手动输入 / 不设置</option>
            {sources.map(source => <option key={source.value} value={source.value}>{source.label}</option>)}
          </select>
          <input value={params[param.name] || ''} placeholder={param.default || param.name} onChange={event => onChange(param.name, event.target.value)} />
        </div>
      ))}
    </div>
  )
}

function emptyCanvasRunState() {
  return {
    status: 'idle',
    nodes: {},
    edges: {},
    conditionMatches: {},
    message: ''
  }
}

function buildRunningCanvasRunState(nodes) {
  const overlay = emptyCanvasRunState()
  overlay.status = 'running'
  overlay.message = '工作流执行中'
  ;(nodes || []).forEach(node => {
    overlay.nodes[node.id] = {
      status: 'running',
      label: runStatusLabel('running'),
      title: '执行中'
    }
  })
  return overlay
}

function buildFailedCanvasRunState(nodes, message) {
  const overlay = emptyCanvasRunState()
  overlay.status = 'failed'
  overlay.message = message || '工作流执行失败。'
  ;(nodes || []).forEach(node => {
    overlay.nodes[node.id] = {
      status: 'failed',
      label: runStatusLabel('failed'),
      error: overlay.message,
      title: overlay.message
    }
  })
  return overlay
}

function buildCanvasRunStateFromDetail(detail, nodes, fallbackError = '') {
  const record = detail?.record || detail?.data?.record || {}
  const nodeIDs = new Set((nodes || []).map(node => node.id))
  const nodeByID = new Map((nodes || []).map(node => [node.id, node]))
  const overlay = emptyCanvasRunState()
  overlay.status = normalizeRunStatus(record.status || (fallbackError ? 'failed' : 'succeeded'))
  overlay.message = record.error || fallbackError || ''
  ;(record.steps || []).forEach(step => {
    if (!step) return
    const normalized = normalizeStepForOverlay(step)
    const directNodeID = nodeIDs.has(step.id) ? step.id : ''
    const iteration = parseLoopIterationStepID(step.id)
    if (directNodeID) {
      overlay.nodes[directNodeID] = mergeRunNodeOverlay(overlay.nodes[directNodeID], normalized)
      if (normalized.matchedCase) overlay.conditionMatches[directNodeID] = normalized.matchedCase
      return
    }
    if (iteration?.loopID && nodeIDs.has(iteration.loopID)) {
      const loopIterationSteps = [...(overlay.nodes[iteration.loopID]?.iterationSteps || []), normalized]
      const loopSummary = {
        status: normalized.status,
        label: runStatusLabel(normalized.status),
        loopIterations: Math.max(Number(overlay.nodes[iteration.loopID]?.loopIterations || 0), iteration.iteration || 0),
        iterationSteps: loopIterationSteps
      }
      if (normalized.error) loopSummary.error = normalized.error
      overlay.nodes[iteration.loopID] = mergeRunNodeOverlay(overlay.nodes[iteration.loopID], loopSummary)
    }
    if (iteration?.targetID && nodeIDs.has(iteration.targetID)) {
      const targetIterationSteps = [...(overlay.nodes[iteration.targetID]?.iterationSteps || []), normalized]
      const targetSummary = {
        status: normalized.status,
        label: runStatusLabel(normalized.status),
        loopIterationCount: Number(overlay.nodes[iteration.targetID]?.loopIterationCount || 0) + 1,
        iterationSteps: targetIterationSteps
      }
      if (normalized.error) targetSummary.error = normalized.error
      overlay.nodes[iteration.targetID] = mergeRunNodeOverlay(overlay.nodes[iteration.targetID], targetSummary)
    }
  })
  if ((record.status === 'failed' || fallbackError) && (record.steps || []).length === 0) {
    ;(nodes || []).forEach(node => {
      overlay.nodes[node.id] = {status: 'failed', label: runStatusLabel('failed'), error: record.error || fallbackError || '工作流执行失败。'}
    })
  }
  if ((record.status === 'failed' || fallbackError) && !Object.values(overlay.nodes).some(item => item.status === 'failed')) {
    const lastRunnableNode = [...(record.steps || [])].reverse().find(step => nodeIDs.has(step.id))
    if (lastRunnableNode) {
      overlay.nodes[lastRunnableNode.id] = mergeRunNodeOverlay(overlay.nodes[lastRunnableNode.id], {status: 'failed', label: runStatusLabel('failed'), error: record.error || fallbackError})
    }
  }
  ;(nodes || []).forEach(node => {
    if (overlay.status === 'running') {
      overlay.nodes[node.id] = overlay.nodes[node.id] || {status: 'running', label: runStatusLabel('running')}
      return
    }
    if (!overlay.nodes[node.id] && (record.steps || []).length > 0) {
      overlay.nodes[node.id] = {status: 'skipped', label: runStatusLabel('skipped'), skippedReason: '运行记录中没有该节点步骤'}
    }
    const run = overlay.nodes[node.id]
    if (node.type === 'controlNode' && node.data?.controlType === 'loop' && run?.loopIterations === undefined && run?.iterationSteps?.length) {
      run.loopIterations = run.iterationSteps.length
    }
    if (nodeByID.get(node.id)?.type === 'conditionNode' && run?.matchedCase) {
      overlay.conditionMatches[node.id] = run.matchedCase
    }
  })
  return overlay
}

function normalizeStepForOverlay(step) {
  const status = normalizeRunStatus(step.status)
  return {
    id: step.id,
    type: step.type,
    tool: step.tool,
    status,
    label: runStatusLabel(status),
    error: step.error || '',
    skippedReason: step.skipped_reason || step.skippedReason || '',
    conditionInput: step.condition_input || step.conditionInput || '',
    matchedCase: step.matched_case || step.matchedCase || '',
    loopTarget: step.loop_target || step.loopTarget || '',
    loopIterations: Number(step.loop_iterations || step.loopIterations || 0) || 0
  }
}

function mergeRunNodeOverlay(previous, next) {
  const merged = {...(previous || {}), ...(next || {})}
  const previousStatus = previous?.status
  const nextStatus = next?.status
  if (previousStatus || nextStatus) {
    merged.status = strongerRunStatus(previousStatus, nextStatus)
    merged.label = runStatusLabel(merged.status)
  }
  if (previous?.error && next?.error && previous.error !== next.error) merged.error = `${previous.error}\n${next.error}`
  if (previous?.skippedReason && next?.skippedReason && previous.skippedReason !== next.skippedReason) merged.skippedReason = `${previous.skippedReason}\n${next.skippedReason}`
  if (previous?.iterationSteps || next?.iterationSteps) merged.iterationSteps = dedupeRunSteps([...(previous?.iterationSteps || []), ...(next?.iterationSteps || [])])
  if (previous?.loopIterations || next?.loopIterations) merged.loopIterations = Math.max(Number(previous?.loopIterations || 0), Number(next?.loopIterations || 0))
  if (previous?.loopIterationCount || next?.loopIterationCount) merged.loopIterationCount = Math.max(Number(previous?.loopIterationCount || 0), Number(next?.loopIterationCount || 0))
  return merged
}

function dedupeRunSteps(steps) {
  const seen = new Set()
  const out = []
  ;(steps || []).forEach(step => {
    const key = step?.id || JSON.stringify(step)
    if (seen.has(key)) return
    seen.add(key)
    out.push(step)
  })
  return out
}

function strongerRunStatus(left, right) {
  const rank = {failed: 5, running: 4, skipped: 3, succeeded: 2, idle: 1}
  return (rank[normalizeRunStatus(right)] || 0) > (rank[normalizeRunStatus(left)] || 0) ? normalizeRunStatus(right) : normalizeRunStatus(left)
}

function parseLoopIterationStepID(id) {
  const match = String(id || '').match(/^(.+)#(\d+)-(.+)$/)
  if (!match) return null
  return {loopID: match[1], iteration: Number(match[2]) || 0, targetID: match[3]}
}

function buildDisplayNodes(nodes, runState) {
  const overlayNodes = runState?.nodes || {}
  return (nodes || []).map(node => ({
    ...node,
    data: {
      ...node.data,
      run: overlayNodes[node.id] || null
    }
  }))
}

function buildDisplayEdges(edges, displayNodes, runState) {
  const nodeRun = new Map((displayNodes || []).map(node => [node.id, node.data?.run]))
  const nodeByID = new Map((displayNodes || []).map(node => [node.id, node]))
  const conditionMatches = runState?.conditionMatches || {}
  return (edges || []).map(edge => {
    const sourceNode = nodeByID.get(edge.source)
    const sourceRun = nodeRun.get(edge.source)
    const targetRun = nodeRun.get(edge.target)
    const edgeCase = edge.data?.case || edge.sourceHandle || ''
    const matchedCase = conditionMatches[edge.source] || sourceRun?.matchedCase || ''
    const isConditionEdge = sourceNode?.type === 'conditionNode' || Boolean(edgeCase)
    let runClass = ''
    if (isConditionEdge && matchedCase) {
      runClass = edgeCase === matchedCase ? 'runEdgeMatched' : 'runEdgeDimmed'
    } else if (sourceRun?.status === 'succeeded' && targetRun?.status === 'succeeded') {
      runClass = 'runEdgeSucceeded'
    } else if (sourceRun?.status === 'failed' || targetRun?.status === 'failed') {
      runClass = 'runEdgeFailed'
    }
    return {
      ...edge,
      animated: edge.animated || runClass === 'runEdgeMatched',
      className: [edge.className, runClass].filter(Boolean).join(' '),
      data: {
        ...(edge.data || {}),
        run: runClass || null
      }
    }
  })
}

function nodeRunClass(baseClass, selected, run) {
  const runStatus = run?.status
  return [baseClass, selected ? 'selected' : '', runStatus && runStatus !== 'idle' ? `run${capitalizeStatus(runStatus)}` : ''].filter(Boolean).join(' ')
}

function RunStatusBadge({run}) {
  if (!run?.status || run.status === 'idle') return null
  return <span className={`runStatusBadge run${capitalizeStatus(run.status)}`} title={formatNodeRunTitle(run)}>{runStatusLabel(run.status)}</span>
}

function formatNodeRunTitle(run) {
  if (!run?.status) return ''
  const parts = [`运行状态：${runStatusLabel(run.status)}`]
  if (run.error) parts.push(`错误：${run.error}`)
  if (run.skippedReason) parts.push(`跳过原因：${run.skippedReason}`)
  if (run.matchedCase) parts.push(`命中分支：${run.matchedCase}`)
  if (run.conditionInput) parts.push(`条件输入：${run.conditionInput}`)
  if (run.loopIterations) parts.push(`循环次数：${run.loopIterations}`)
  if (run.loopTarget) parts.push(`兼容目标：${run.loopTarget}`)
  if (run.loopIterationCount) parts.push(`目标节点循环执行：${run.loopIterationCount} 次`)
  if (run.iterationSteps?.length) {
    const failedIterations = run.iterationSteps.filter(step => step.status === 'failed')
    parts.push(`循环迭代记录：${run.iterationSteps.length} 条`)
    if (failedIterations.length > 0) parts.push(`失败迭代：${failedIterations.map(step => step.id).join(', ')}`)
  }
  return parts.join('\n')
}

function runStatusLabel(status) {
  const normalized = normalizeRunStatus(status)
  if (normalized === 'succeeded') return '成功'
  if (normalized === 'failed') return '失败'
  if (normalized === 'skipped') return '跳过'
  if (normalized === 'running') return '运行中'
  return '未运行'
}

function normalizeRunStatus(status) {
  const value = String(status || '').toLowerCase()
  if (value === 'success' || value === 'succeeded' || value === 'ok') return 'succeeded'
  if (value === 'fail' || value === 'failed' || value === 'error') return 'failed'
  if (value === 'skip' || value === 'skipped') return 'skipped'
  if (value === 'running' || value === 'pending') return 'running'
  return value || 'idle'
}

function capitalizeStatus(status) {
  const normalized = normalizeRunStatus(status)
  return normalized.charAt(0).toUpperCase() + normalized.slice(1)
}

function extractRunDetailFromResponse(body) {
  if (!body) return null
  if (body.record) return body
  if (body.data?.record) return body.data
  if (body.detail?.record) return body.detail
  if (body.data?.detail?.record) return body.data.detail
  return null
}

function emptyWorkflow(category) {
  return {
    id: '',
    name: '',
    description: '',
    version: '1.0.0',
    category: category || 'global',
    tags: [],
    parameters: [],
    nodes: [],
    edges: [],
    confirm: {required: false}
  }
}

function newToolFlowNode(tool, id, position, onRemove) {
  return {
    id,
    type: 'toolNode',
    data: {
      tool: tool.id,
      name: tool.name || tool.id,
      params: defaultParams(tool.parameters || []),
      onRemove
    },
    position
  }
}

function newConditionFlowNode(id, position, onRemove) {
  return {
    id,
    type: 'conditionNode',
    data: {
      name: id,
      condition: defaultCondition(),
      onRemove
    },
    position
  }
}

function newControlFlowNode(control, id, position, onRemove) {
  const data = {
    controlType: control.type,
    title: control.title,
    name: id,
    onRemove
  }
  if (control.type === 'loop') data.loop = defaultLoop()
  return {
    id,
    type: 'controlNode',
    data,
    position
  }
}

function defaultLoop() {
  return {
    tool: '',
    params: {},
    max_iterations: 3
  }
}

function normalizeLoopConfig(loop) {
  const params = loop?.params && typeof loop.params === 'object' && !Array.isArray(loop.params) ? loop.params : {}
  return {
    tool: String(loop?.tool || '').trim(),
    params,
    max_iterations: clampLoopIterations(loop?.max_iterations || loop?.maxIterations || 1)
  }
}

function clampLoopIterations(value) {
  const parsed = Number.parseInt(value, 10)
  if (!Number.isFinite(parsed)) return 1
  return Math.min(20, Math.max(1, parsed))
}
function loopNodeStatus(loop) {
  const hasTool = Boolean(String(loop?.tool || '').trim())
  const iterations = Number(loop?.max_iterations || 0)
  return hasTool && iterations >= 1 && iterations <= 20
    ? {ready: true, label: '可运行'}
    : {ready: false, label: '配置不完整'}
}

function controlNodeTitle(type) {
  const control = controlNodeCatalog.find(item => item.type === type)
  return control?.title || type || '编排节点'
}

function controlNodeHelp(type) {
  const control = controlNodeCatalog.find(item => item.type === type)
  return control?.help || '编排控制节点'
}

function updateCaseValuesText(value) {
  return value.split(/[\n,]/).map(item => item.trim()).filter(Boolean)
}

function conditionBranchRows(condition) {
  const cases = condition?.cases || []
  const rows = cases.map((item, index) => {
    const id = String(item.id || '').trim()
    return {
      key: `${id || 'case'}-${index}`,
      handleID: id,
      label: item.name || id || `未命名分支 ${index + 1}`,
      meta: id ? `case: ${id}` : '请先填写 case ID',
      kind: 'case',
      disabled: !id
    }
  })
  if (condition?.default_case === 'default') {
    rows.push({
      key: 'default',
      handleID: 'default',
      label: '默认分支',
      meta: 'default',
      kind: 'default',
      disabled: false
    })
  } else {
    rows.push({
      key: 'default-disabled',
      handleID: '',
      label: '默认分支',
      meta: '未启用',
      kind: 'default',
      disabled: true
    })
  }
  return rows
}

function edgeCaseFromHandle(sourceNode, sourceHandle) {
  if (sourceNode?.type !== 'conditionNode') return ''
  return sourceHandle || ''
}

function legacyLoopTargetMap(workflowNodes) {
  const nodeIDs = new Set((workflowNodes || []).map(node => node.id))
  const targetMap = new Map()
  ;(workflowNodes || []).forEach(node => {
    const nodeType = node.type || (node.tool ? 'tool' : node.loop ? 'loop' : '')
    const targetID = nodeType === 'loop' && !node.loop?.tool ? String(node.loop?.target || '') : ''
    if (targetID && nodeIDs.has(targetID)) targetMap.set(targetID, node.id)
  })
  return targetMap
}

function remapLegacyLoopEdges(edges, targetMap) {
  if (!targetMap || targetMap.size === 0) return edges || []
  const seen = new Set()
  return (edges || [])
    .map(edge => {
      if (targetMap.has(edge.from)) return {...edge, from: targetMap.get(edge.from)}
      if (targetMap.has(edge.to)) return {...edge, to: targetMap.get(edge.to)}
      return edge
    })
    .filter(edge => {
      if (edge.from === edge.to) return false
      const key = `${edge.from}->${edge.to}:${edge.case || ''}`
      if (seen.has(key)) return false
      seen.add(key)
      return true
    })
}

function workflowNodeToFlowNode(node, index, onRemove, workflowNodes = []) {
  const nodeType = node.type || (node.tool ? 'tool' : node.loop ? 'loop' : 'condition')
  if (nodeType === 'condition') {
    return {
      id: node.id,
      type: 'conditionNode',
      data: {
        name: node.name || node.id,
        condition: node.condition || defaultCondition(),
        onRemove
      },
      position: {x: 80 + index * 220, y: 120 + (index % 3) * 90}
    }
  }
  if (nodeType === 'parallel' || nodeType === 'join' || nodeType === 'loop') {
    const control = controlNodeCatalog.find(item => item.type === nodeType) || {type: nodeType, title: controlNodeTitle(nodeType)}
    const flowNode = newControlFlowNode(control, node.id, {x: 80 + index * 220, y: 120 + (index % 3) * 90}, onRemove)
    flowNode.data.name = node.name || node.id
    if (nodeType === 'loop') {
      const loop = normalizeLoopConfig(node.loop || defaultLoop())
      if (!loop.tool && node.loop?.target) {
        const target = (workflowNodes || []).find(item => item.id === node.loop.target)
        if (target?.tool) {
          loop.tool = target.tool
          loop.params = target.params && Object.keys(loop.params || {}).length === 0 ? target.params : loop.params
        }
      }
      flowNode.data.loop = loop
    }
    return flowNode
  }
  return {
    id: node.id,
    type: 'toolNode',
    data: {
      tool: node.tool,
      name: node.name || node.id,
      params: node.params || {},
      on_failure: node.on_failure || 'stop',
      onRemove
    },
    position: {x: 80 + index * 220, y: 120 + (index % 3) * 90}
  }
}

function workflowScopeCategory(value, fallbackCategory = '') {
  if (value === 'global') return 'global'
  return value || fallbackCategory || 'global'
}

function buildWorkflowDraft(workflow, nodes, edges, category, parameters) {
  return {
    ...workflow,
    category: workflowScopeCategory(workflow.category, category),
    tags: normalizeTags(workflow.tags || []),
    parameters: parameters || workflow.parameters || [],
    nodes: nodes.map(node => {
      if (node.type === 'conditionNode') {
        return {
          id: node.id,
          type: 'condition',
          name: node.data.name || node.id,
          condition: node.data.condition || defaultCondition()
        }
      }
      if (node.type === 'controlNode') {
        const draftNode = {
          id: node.id,
          type: node.data.controlType,
          name: node.data.name || node.id
        }
        if (node.data.controlType === 'loop') draftNode.loop = normalizeLoopConfig(node.data.loop || defaultLoop())
        return draftNode
      }
      return {
        id: node.id,
        type: 'tool',
        name: node.data.name || node.id,
        tool: node.data.tool,
        params: node.data.params || {},
        on_failure: node.data.on_failure || 'stop'
      }
    }),
    edges: edges.map(edge => {
      const sourceNode = nodes.find(node => node.id === edge.source)
      const out = {from: edge.source, to: edge.target}
      const edgeCase = sourceNode?.type === 'conditionNode' ? (edge.data?.case || edge.sourceHandle || '') : ''
      if (edgeCase) out.case = edgeCase
      return out
    })
  }
}

function defaultCondition() {
  return {
    input: '',
    cases: [
      {id: 'case1', name: '分支 1', operator: 'contains', values: []},
      {id: 'case2', name: '分支 2', operator: 'contains', values: []}
    ],
    default_case: 'default'
  }
}

function flowEdgeFromWorkflowEdge(edge, index, nodes = []) {
  const sourceNode = nodes.find(node => node.id === edge.from)
  const edgeCase = edge.case || ''
  const isConditionEdge = sourceNode?.type === 'conditionNode' || Boolean(edgeCase)
  return {
    id: `${edge.from}-${edge.to}-${edgeCase || index}`,
    source: edge.from,
    target: edge.to,
    sourceHandle: isConditionEdge && edgeCase ? edgeCase : undefined,
    type: 'smoothstep',
    animated: true,
    label: edgeCase ? conditionCaseLabel(sourceNode?.data.condition, edgeCase) : '',
    data: edgeCase ? {case: edgeCase} : {}
  }
}

function autoLayoutNodes(nodes, edges) {
  if (!nodes.length) return nodes

  const nodeIDs = new Set(nodes.map(node => node.id))
  const nodeOrder = new Map(nodes.map((node, index) => [node.id, index]))
  const children = new Map(nodes.map(node => [node.id, []]))
  const incomingCounts = new Map(nodes.map(node => [node.id, 0]))
  const depths = new Map(nodes.map(node => [node.id, 0]))

  ;(edges || []).forEach(edge => {
    if (!nodeIDs.has(edge.source) || !nodeIDs.has(edge.target) || edge.source === edge.target) return
    children.get(edge.source).push(edge.target)
    incomingCounts.set(edge.target, (incomingCounts.get(edge.target) || 0) + 1)
  })

  const queue = nodes
    .filter(node => (incomingCounts.get(node.id) || 0) === 0)
    .map(node => node.id)
  const visited = new Set()

  while (queue.length > 0) {
    const nodeID = queue.shift()
    if (visited.has(nodeID)) continue
    visited.add(nodeID)
    const nextDepth = (depths.get(nodeID) || 0) + 1
    ;(children.get(nodeID) || [])
      .sort((left, right) => (nodeOrder.get(left) || 0) - (nodeOrder.get(right) || 0))
      .forEach(childID => {
        depths.set(childID, Math.max(depths.get(childID) || 0, nextDepth))
        const nextIncoming = (incomingCounts.get(childID) || 0) - 1
        incomingCounts.set(childID, nextIncoming)
        if (nextIncoming === 0) queue.push(childID)
      })
  }

  if (visited.size < nodes.length) {
    const fallbackDepth = Math.max(0, ...Array.from(depths.values())) + 1
    nodes.forEach(node => {
      if (visited.has(node.id)) return
      depths.set(node.id, fallbackDepth + (nodeOrder.get(node.id) || 0))
    })
  }

  const layers = new Map()
  nodes.forEach(node => {
    const depth = depths.get(node.id) || 0
    if (!layers.has(depth)) layers.set(depth, [])
    layers.get(depth).push(node)
  })

  const orderedLayers = Array.from(layers.entries())
    .sort(([left], [right]) => left - right)
    .map(([, layerNodes]) => layerNodes.sort((left, right) => (nodeOrder.get(left.id) || 0) - (nodeOrder.get(right.id) || 0)))
  const layerMetrics = orderedLayers.map(layerNodes => {
    const sizes = layerNodes.map(autoLayoutNodeSize)
    const height = sizes.reduce((total, size) => total + size.height, 0) + Math.max(0, sizes.length - 1) * 42
    const width = Math.max(...sizes.map(size => size.width))
    return {sizes, height, width}
  })
  const maxLayerHeight = Math.max(...layerMetrics.map(metric => metric.height), 0)
  const positions = new Map()
  let x = 80

  orderedLayers.forEach((layerNodes, layerIndex) => {
    const metrics = layerMetrics[layerIndex]
    let y = 80 + Math.max(0, (maxLayerHeight - metrics.height) / 2)
    layerNodes.forEach((node, nodeIndex) => {
      const size = metrics.sizes[nodeIndex]
      positions.set(node.id, {x, y})
      y += size.height + 42
    })
    x += metrics.width + 140
  })

  return nodes.map(node => ({
    ...node,
    position: positions.get(node.id) || node.position || {x: 80, y: 80}
  }))
}

function autoLayoutNodeSize(node) {
  if (node.type === 'conditionNode') {
    const branchCount = conditionBranchRows(node.data.condition || defaultCondition()).length
    return {width: 440, height: Math.max(156, 72 + branchCount * 42)}
  }
  if (node.type === 'controlNode') return {width: 250, height: 82}
  return {width: 210, height: 74}
}

function conditionCaseLabel(condition, caseID) {
  if (!caseID) return ''
  if (caseID === 'default') return 'default'
  const item = (condition?.cases || []).find(item => item.id === caseID)
  return item ? (item.name || item.id) : caseID
}

function conditionSummary(condition) {
  if (!condition?.input) return '未选择判断输入'
  const input = compactTemplatePath(condition.input)
  const first = (condition.cases || [])[0]
  if (!first) return input
  const values = (first.values || []).filter(Boolean).join('/')
  return [input, first.operator, values].filter(Boolean).join(' ')
}

function conditionCaseSummary(condition) {
  const cases = condition?.cases || []
  const suffix = condition?.default_case === 'default' ? ' + default' : ''
  if (cases.length === 0) return `未配置分支${suffix}`
  return `${cases.length} 个分支${suffix}：${cases.map(item => item.name || item.id).join(' / ')}`
}

function conditionNodeStatus(condition) {
  const hasInput = Boolean(String(condition?.input || '').trim())
  const cases = condition?.cases || []
  const seen = new Set()
  const casesValid = cases.length > 0 && cases.every(item => {
    const id = String(item.id || '').trim()
    const valid = Boolean(id) && id !== 'default' && !seen.has(id) && conditionOperators.some(operator => operator.value === item.operator)
    seen.add(id)
    return valid
  })
  return hasInput && casesValid
    ? {ready: true, label: '可运行'}
    : {ready: false, label: '配置不完整'}
}

function compactTemplatePath(value) {
  return String(value || '')
    .replace(/^\s*{{\s*\./, '')
    .replace(/\s*}}\s*$/, '')
}

function validateConditionDraft(nodes, edges) {
  const errors = []
  const nodeMap = new Map(nodes.map(node => [node.id, node]))
  nodes.filter(node => node.type === 'conditionNode').forEach(node => {
    const condition = node.data.condition || {}
    if (!String(condition.input || '').trim()) errors.push(`条件节点 ${node.id} 缺少输入来源。`)
    if (!condition.cases || condition.cases.length === 0) errors.push(`条件节点 ${node.id} 至少需要一个 case。`)
    const seen = new Set()
    ;(condition.cases || []).forEach(item => {
      if (!String(item.id || '').trim()) errors.push(`条件节点 ${node.id} 存在空 case ID。`)
      if (item.id === 'default') errors.push(`条件节点 ${node.id} 的 case ID 不能使用保留值 default。`)
      if (seen.has(item.id)) errors.push(`条件节点 ${node.id} 的 case ID 重复：${item.id}`)
      seen.add(item.id)
      if (!conditionOperators.some(operator => operator.value === item.operator)) errors.push(`条件节点 ${node.id} 的 case ${item.id || '-'} 操作符非法。`)
    })
    edges.filter(edge => edge.source === node.id).forEach(edge => {
      const edgeCase = edge.data?.case || edge.sourceHandle || ''
      if (!edgeCase) errors.push(`条件节点 ${node.id} 到 ${edge.target} 的连线缺少 case。`)
      if (edgeCase === 'default' && condition.default_case !== 'default') errors.push(`条件节点 ${node.id} 未启用 default 分支，但到 ${edge.target} 的连线选择了 default。`)
      if (edgeCase && edgeCase !== 'default' && !(condition.cases || []).some(item => item.id === edgeCase)) errors.push(`条件节点 ${node.id} 到 ${edge.target} 的连线引用不存在的 case：${edgeCase}`)
    })
  })
  edges.forEach(edge => {
    const source = nodeMap.get(edge.source)
    if (source?.type !== 'conditionNode' && edge.data?.case) errors.push(`非条件节点 ${edge.source} 的连线不能配置 case。`)
  })
  return errors
}

function validateControlDraft(nodes, edges, tools = []) {
  const errors = []
  const toolMap = new Map((tools || []).map(tool => [tool.id, tool]))
  nodes.filter(node => node.type === 'controlNode').forEach(node => {
    if (node.data.controlType === 'loop') {
      const loop = normalizeLoopConfig(node.data.loop || {})
      if (!loop.tool) errors.push(`循环节点 ${node.id} 请选择循环工具。`)
      if (loop.tool && !toolMap.has(loop.tool)) errors.push(`循环节点 ${node.id} 引用了不存在的工具：${loop.tool}`)
      if (!Number.isInteger(loop.max_iterations) || loop.max_iterations < 1 || loop.max_iterations > 20) errors.push(`循环节点 ${node.id} 的最大循环次数必须在 1 到 20 之间。`)
    }
    if (node.data.controlType === 'parallel' && !edges.some(edge => edge.source === node.id)) {
      errors.push(`并行分支节点 ${node.id} 至少需要一条出边。`)
    }
    if (node.data.controlType === 'join' && !edges.some(edge => edge.target === node.id)) {
      errors.push(`合流节点 ${node.id} 至少需要一条入边。`)
    }
  })
  return errors
}

function normalizeTags(tags) {
  const seen = new Set()
  const out = []
  ;(Array.isArray(tags) ? tags : String(tags || '').split(/[\n,]/)).forEach(item => {
    const tag = String(item || '').trim()
    if (!tag || seen.has(tag)) return
    seen.add(tag)
    out.push(tag)
  })
  return out
}

function findOutOfScopeToolNodes(nodes, tools, scopedCategory) {
  if (!scopedCategory) return []
  const toolMap = new Map((tools || []).map(tool => [tool.id, tool]))
  return nodes
    .filter(node => node.type === 'toolNode' || (node.type === 'controlNode' && node.data.controlType === 'loop'))
    .map(node => {
      const toolID = node.type === 'controlNode' ? node.data.loop?.tool : node.data.tool
      return {node, toolID, tool: toolMap.get(toolID)}
    })
    .filter(item => item.tool && item.tool.category !== scopedCategory)
    .map(item => ({nodeID: item.node.id, toolID: item.toolID, scopeName: scopedCategory}))
}

function defaultParams(parameters) {
  const out = {}
  ;(parameters || []).forEach(param => {
    out[param.name] = param.default === undefined || param.default === null ? '' : param.default
  })
  return out
}

function parseJSONList(value) {
  try {
    const parsed = JSON.parse(value || '[]')
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

function findMissingRequiredNodeParams(nodes, tools) {
  const toolMap = new Map((tools || []).map(tool => [tool.id, tool]))
  const missing = []
  nodes.forEach(node => {
    if (node.type === 'toolNode') {
      const tool = toolMap.get(node.data.tool)
      ;(tool?.parameters || []).forEach(param => {
        if (!param.required) return
        const value = node.data.params?.[param.name]
        if (value === undefined || value === null || String(value).trim() === '') {
          missing.push({nodeID: node.id, toolName: tool.name || tool.id, paramName: param.name})
        }
      })
      return
    }
    if (node.type === 'controlNode' && node.data.controlType === 'loop') {
      const loop = normalizeLoopConfig(node.data.loop || defaultLoop())
      const tool = toolMap.get(loop.tool)
      ;(tool?.parameters || []).forEach(param => {
        if (!param.required) return
        const value = loop.params?.[param.name]
        if (value === undefined || value === null || String(value).trim() === '') {
          missing.push({nodeID: node.id, toolName: tool.name || tool.id, paramName: param.name})
        }
      })
    }
  })
  return missing
}

function formatPreflightMessage(status) {
  const errors = status.errors || []
  if (errors.length === 0) return status.title || '检查通过。'
  return `${status.title || '检查未通过'}\n${errors.map(error => `- ${error}`).join('\n')}`
}

function summarizeAPIResponse(body, fallback) {
  if (body?.data?.valid === false) return readableValidationMessages(body.data).join('\n')
  if (body?.message) return body.message
  if (body?.error) return body.error
  return fallback
}

function readableAPIError(err, fallback) {
  const body = err.body
  if (body?.data) {
    const messages = readableValidationMessages(body.data)
    if (messages.length > 0) return `${fallback}\n${messages.join('\n')}`
  }
  return `${fallback}\n${err.message || String(err)}`
}

function readableValidationMessages(data) {
  const messages = []
  if (typeof data === 'string') messages.push(data)
  ;(data?.errors || data?.warnings || []).forEach(item => messages.push(typeof item === 'string' ? item : JSON.stringify(item)))
  if (data?.message) messages.push(data.message)
  if (data?.error) messages.push(data.error)
  if (data?.valid === false && messages.length === 0) messages.push('后端校验未通过，请检查工作流配置。')
  return messages.map(message => `- ${message}`)
}

function combineWorkflowStepLogs(steps, record) {
  const rendered = []
  ;(record?.steps || []).forEach(step => {
    const parts = [`[${step.id}] ${displayStepType(step.type)} ${step.status}`]
    if (step.type === 'condition') {
      if (step.condition_input !== undefined) parts.push(`条件输入: ${step.condition_input}`)
      if (step.matched_case) parts.push(`命中分支: ${step.matched_case}`)
    } else if (step.type === 'loop') {
      if (step.tool) parts.push(`循环工具: ${step.tool}`)
      if (step.loop_target) parts.push(`兼容目标: ${step.loop_target}`)
      if (step.loop_iterations) parts.push(`循环次数: ${step.loop_iterations}`)
    } else if (step.type === 'parallel' || step.type === 'join') {
      parts.push('编排控制节点已完成')
    }
    if (step.skipped_reason) parts.push(`跳过原因: ${step.skipped_reason}`)
    const stepLogs = steps?.[step.id] || {}
    if (stepLogs.stdout) parts.push(`stdout:\n${stepLogs.stdout}`)
    if (stepLogs.stderr) parts.push(`stderr:\n${stepLogs.stderr}`)
    if (parts.length === 1) parts.push('无日志内容')
    rendered.push(parts.join('\n'))
  })
  if (rendered.length > 0) return rendered.join('\n\n')
  if (!steps || Object.keys(steps).length === 0) return ''
  return Object.entries(steps).map(([id, stepLogs]) => {
    const parts = [`[${id}]`]
    if (stepLogs.stdout) parts.push(`stdout:\n${stepLogs.stdout}`)
    if (stepLogs.stderr) parts.push(`stderr:\n${stepLogs.stderr}`)
    if (parts.length === 1) parts.push('无日志内容')
    return parts.join('\n')
  }).join('\n\n')
}

function displayStepType(type) {
  if (type === 'condition') return '编排节点/条件分支'
  if (type === 'parallel') return '编排节点/并行分支'
  if (type === 'join') return '编排节点/合流'
  if (type === 'loop') return '编排节点/循环'
  return '工具节点'
}

function buildMappingSources(workflowParameters, selectedNodeID, nodes, edges) {
  const sources = []
  ;(workflowParameters || []).forEach(param => {
    if (param?.name) sources.push({label: `工作流参数 / ${param.name}`, value: `{{ .${param.name} }}`})
  })
  upstreamNodeIDs(selectedNodeID, edges).forEach(nodeID => {
    const node = nodes.find(item => item.id === nodeID)
    if (!node) return
    sources.push({label: `${nodeID} / 标准输出 stdout`, value: `{{ .steps.${nodeID}.stdout }}`})
    sources.push({label: `${nodeID} / 错误输出 stderr`, value: `{{ .steps.${nodeID}.stderr }}`})
    Object.keys(node.data.params || {}).forEach(name => {
      sources.push({label: `${nodeID} / 参数 ${name}`, value: `{{ .steps.${nodeID}.params.${name} }}`})
    })
  })
  return sources
}

function upstreamNodeIDs(selectedNodeID, edges) {
  if (!selectedNodeID) return []
  const direct = edges.filter(edge => edge.target === selectedNodeID).map(edge => edge.source)
  return Array.from(new Set(direct)).sort((a, b) => a.localeCompare(b, 'zh-CN'))
}

function tagsForEntries(entries) {
  const tags = new Set()
  entries.forEach(entry => (entry.tags || []).forEach(tag => tags.add(tag)))
  return Array.from(tags).sort((a, b) => a.localeCompare(b, 'zh-CN'))
}

function filterEntries(entries, searchText, activeTag) {
  const keyword = searchText.trim().toLowerCase()
  return entries.filter(entry => {
    const tags = entry.tags || []
    if (activeTag && !tags.includes(activeTag)) return false
    if (!keyword) return true
    return [entry.id, entry.name, entry.description, entry.category, ...tags]
      .filter(Boolean)
      .some(value => String(value).toLowerCase().includes(keyword))
  })
}

function uniqueNodeID(toolID, nodes) {
  const base = toolID.replaceAll('.', '_').replaceAll('-', '_')
  let index = nodes.length + 1
  let id = `${base}_${index}`
  const existing = new Set(nodes.map(node => node.id))
  while (existing.has(id)) {
    index += 1
    id = `${base}_${index}`
  }
  return id
}

async function fetchJSON(path) {
  const res = await fetch(path)
  const body = await res.json()
  if (!res.ok) {
    const err = new Error(body.error || res.statusText)
    err.status = res.status
    err.body = body
    throw err
  }
  return body
}

async function fetchRunDetail(id) {
  return fetchJSON(`/api/runs/${id}`)
}

async function postJSON(path, payload) {
  const res = await fetch(path, {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(payload)
  })
  const body = await res.json()
  if (!res.ok) {
    const err = new Error(body.error || res.statusText)
    err.status = res.status
    err.body = body
    throw err
  }
  return body
}

async function putJSON(path, payload) {
  const res = await fetch(path, {
    method: 'PUT',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(payload)
  })
  const body = await res.json()
  if (!res.ok) {
    const err = new Error(body.error || res.statusText)
    err.status = res.status
    err.body = body
    throw err
  }
  return body
}

async function deleteJSON(path) {
  const res = await fetch(path, {method: 'DELETE'})
  const body = await res.json()
  if (!res.ok) {
    const err = new Error(body.error || res.statusText)
    err.status = res.status
    err.body = body
    throw err
  }
  return body
}

async function postPluginZip(file, replace) {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch(`/api/plugins/upload${replace ? '?replace=true' : ''}`, {
    method: 'POST',
    body: form
  })
  const body = await res.json()
  if (!res.ok) {
    const err = new Error(body.error || res.statusText)
    err.status = res.status
    err.body = body
    throw err
  }
  return body
}

createRoot(document.getElementById('root')).render(<App />)
