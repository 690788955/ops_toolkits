import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createRoot } from 'react-dom/client'
import '@xyflow/react/dist/style.css'
import './styles.css'
import * as yaml from 'js-yaml'
import {deleteJSON, fetchJSON, fetchRunDetail, postJSON, postPluginZip, putJSON} from './api.js'
import {combineWorkflowStepLogs, filterEntries, readableAPIError, summarizeAPIResponse, tagsForEntries} from './utils.js'

const WorkflowEditor = React.lazy(() => import('./workflow/WorkflowEditor.jsx'))

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
  const [resultExpanded, setResultExpanded] = useState(false)
  const runVersionRef = useRef(0)

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

  const toolEntries = useMemo(() => {
    if (!catalog || categoryDisabled) return []
    const source = catalog.tools || []
    if (!activeCategory) return source
    return source.filter(item => item.category === activeCategory)
  }, [catalog, activeCategory, categoryDisabled])

  const availableTags = useMemo(() => tagsForEntries(toolEntries), [toolEntries])

  const entries = useMemo(() => {
    return filterEntries(toolEntries, searchText, activeTag)
  }, [toolEntries, searchText, activeTag])

  function resetResult() {
    runVersionRef.current += 1
    setResultExpanded(false)
    setResult({message: '等待执行...'})
  }

  function selectEntry(entry) {
    setSelected({...entry, kind: 'tool'})
    const next = {}
    ;(entry.parameters || []).forEach(param => {
      next[param.name] = param.default === undefined || param.default === null ? '' : String(param.default)
    })
    setParams(next)
    resetResult()
  }

  async function runSelected() {
    if (!selected) return
    const needsConfirm = selected.confirm?.required
    if (needsConfirm && !window.confirm(selected.confirm.message || '该操作需要确认，是否继续？')) return
    runVersionRef.current += 1
    setResult({message: '执行中...'})
    try {
      const body = await postJSON(`/api/tools/${selected.id}/run`, {params, confirm: Boolean(needsConfirm)})
      if (body.id) {
        setResult({run: body, detail: await fetchRunDetail(body.id)})
        return
      }
      setResult({message: summarizeAPIResponse(body, '执行请求已提交。'), response: body})
    } catch (err) {
      setResult({message: readableAPIError(err, '执行失败。'), response: err.body})
    }
  }

  useEffect(() => {
    if (!resultExpanded) return undefined
    function handleKeyDown(event) {
      if (event.key === 'Escape') setResultExpanded(false)
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [resultExpanded])

  const resultPanelHeader = (
    <div className="cardHeader resultPanelHeader">
      <h3>执行结果</h3>
      <button className="secondary iconButton logExpandButton" type="button" aria-label="放大查看日志" title="放大查看日志" onClick={() => setResultExpanded(true)}>
        <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
          <path d="M8 3H3v5h2V5h3V3Zm13 0h-5v2h3v3h2V3ZM5 16H3v5h5v-2H5v-3Zm16 0h-2v3h-3v2h5v-5Z" />
        </svg>
      </button>
    </div>
  )

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
            <p>{category?.description || '跨分类选择工具或维护工作流'}</p>
          </div>
          <div className="topbarActions">
            <div className="hint">可视化工作流编排</div>
            <button className="settingsButton" type="button" title="全局环境" aria-label="全局环境" onClick={() => setGlobalEnvOpen(true)}>ENV</button>
            <button className="settingsButton" type="button" title="平台设置" aria-label="平台设置" onClick={() => setPlatformSettingsOpen(true)}>SET</button>
          </div>
        </header>

        <div className="tabs">
          <button className={activeTab === 'tools' ? 'tab active' : 'tab'} onClick={() => { setActiveTab('tools'); setSelected(null); setActiveTag(''); resetResult() }}>工具</button>
          <button className={activeTab === 'workflows' ? 'tab active' : 'tab'} onClick={() => { setActiveTab('workflows'); setSelected(null); setActiveTag(''); resetResult() }}>工作流</button>
          {hasConfigTab && <button className={activeTab === 'config' ? 'tab active' : 'tab'} onClick={() => { setActiveTab('config'); setSelected(null); setActiveTag(''); setConfigSelectedPlugin(null); resetResult() }}>配置</button>}
          <button className={activeTab === 'runs' ? 'tab active' : 'tab'} onClick={() => { setActiveTab('runs'); setSelected(null); setActiveTag(''); resetResult() }}>运行记录</button>
        </div>

        {activeTab === 'workflows' ? (
          <React.Suspense fallback={<section className="card canvasCard"><div className="empty">正在加载工作流画布...</div></section>}>
            <WorkflowEditor
              catalog={catalog}
              activeCategory={activeCategory}
              setResult={setResult}
              refreshCatalog={refreshCatalog}
              resultPanel={(
                <section className="card resultCard workflowResultCard">
                  {resultPanelHeader}
                  <ResultView result={result} />
                </section>
              )}
            />
          </React.Suspense>
        ) : activeTab === 'runs' ? (
          <RunHistoryPanel />
        ) : (activeTab === 'config' && hasConfigTab) ? (
          <ConfigPanel catalog={catalog} activeCategory={activeCategory} configSelectedPlugin={configSelectedPlugin} setConfigSelectedPlugin={setConfigSelectedPlugin} refreshCatalog={refreshCatalog} />
        ) : (
          <RunPanel entries={entries} totalEntries={toolEntries.length} selected={selected} params={params} setParams={setParams} selectEntry={selectEntry} runSelected={runSelected} searchText={searchText} setSearchText={setSearchText} activeTag={activeTag} setActiveTag={setActiveTag} availableTags={availableTags} />
        )}

        {activeTab === 'tools' && (
          <section className="card resultCard">
            {resultPanelHeader}
            <ResultView result={result} />
          </section>
        )}
      </main>
      {resultExpanded && activeTab !== 'config' && (
        <div className="modalBackdrop" onMouseDown={event => { if (event.target === event.currentTarget) setResultExpanded(false) }}>
          <section className="modal resultExpandedModal" role="dialog" aria-modal="true" aria-label="执行结果详情">
            <div className="modalHeader">
              <div>
                <h3>执行结果</h3>
                <p>放大查看当前运行日志和完整响应。</p>
              </div>
              <button className="modalClose" type="button" aria-label="关闭" onClick={() => setResultExpanded(false)}>×</button>
            </div>
            <ResultView result={result} expanded />
          </section>
        </div>
      )}
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

function RunHistoryPanel() {
  const [runs, setRuns] = useState([])
  const [selectedRunID, setSelectedRunID] = useState('')
  const [selectedDetail, setSelectedDetail] = useState(null)
  const [statusFilter, setStatusFilter] = useState('')
  const [message, setMessage] = useState('正在加载运行记录...')
  const [loading, setLoading] = useState(false)

  async function loadRuns() {
    setLoading(true)
    setMessage('正在加载运行记录...')
    try {
      const body = await fetchJSON('/api/runs/')
      const items = body.data?.runs || []
      setRuns(items)
      setMessage(items.length ? `已加载 ${items.length} 条运行记录。` : '暂无运行记录。')
    } catch (err) {
      setMessage(readableAPIError(err, '加载运行记录失败。'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadRuns()
  }, [])

  const visibleRuns = useMemo(() => {
    if (!statusFilter) return runs
    return runs.filter(item => item.status === statusFilter)
  }, [runs, statusFilter])

  async function selectRun(runID) {
    setSelectedRunID(runID)
    setSelectedDetail(null)
    setMessage(`正在读取运行记录 ${runID}...`)
    try {
      const body = await fetchRunDetail(runID)
      setSelectedDetail(body.data)
      setMessage(`已加载运行记录 ${runID}。`)
    } catch (err) {
      setMessage(readableAPIError(err, '读取运行详情失败。'))
    }
  }

  const statusOptions = [
    ['', '全部'],
    ['failed', '失败'],
    ['running', '运行中'],
    ['succeeded', '成功']
  ]

  return (
    <div className="runHistoryLayout">
      <section className="card runHistoryListCard">
        <div className="cardHeader">
          <h3>运行记录</h3>
          <button className="secondary" type="button" disabled={loading} onClick={loadRuns}>刷新</button>
        </div>
        <div className="tagFilters runStatusFilters">
          {statusOptions.map(([value, label]) => (
            <button key={value || 'all'} className={statusFilter === value ? 'tagChip active' : 'tagChip'} onClick={() => setStatusFilter(value)}>{label}</button>
          ))}
        </div>
        <div className="runHistoryList">
          {visibleRuns.map(item => (
            <button key={item.id} className={selectedRunID === item.id ? 'runHistoryItem active' : 'runHistoryItem'} onClick={() => selectRun(item.id)}>
              <div>
                <strong>{item.target || item.id}</strong>
                <span>{item.id}</span>
              </div>
              <small className={`runStatusText ${item.status || 'unknown'}`}>{runStatusText(item.status)}</small>
              <em>{formatRunTime(item.started_at)}</em>
            </button>
          ))}
          {visibleRuns.length === 0 && <div className="empty">没有匹配的运行记录。</div>}
        </div>
        <pre className="modalResult compactResult">{message}</pre>
      </section>
      <section className="card runHistoryDetailCard">
        <div className="cardHeader">
          <h3>记录详情</h3>
          {selectedRunID && <a className="secondary" href={`/api/runs/${encodeURIComponent(selectedRunID)}/support.zip`}>导出支持包</a>}
        </div>
        {selectedDetail ? (
          <RunDetail detail={selectedDetail} run={{id: selectedRunID, status: selectedDetail.record?.status}} />
        ) : (
          <div className="empty">选择一条运行记录查看日志和支持包。</div>
        )}
      </section>
    </div>
  )
}

function runStatusText(status) {
  if (status === 'succeeded') return '成功'
  if (status === 'failed') return '失败'
  if (status === 'running') return '运行中'
  if (status === 'skipped') return '跳过'
  return status || '未知'
}

function formatRunTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
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
  const [pluginActionID, setPluginActionID] = useState('')
  const [target, setTarget] = useState('windows/amd64')
  const plugins = useMemo(() => [...(catalog?.plugins || [])].sort((left, right) => String(left.id || '').localeCompare(String(right.id || ''), 'zh-CN')), [catalog])
  const [goos, goarch] = target.split('/')
  const targets = [
    ['windows/amd64', 'Windows amd64'],
    ['windows/arm64', 'Windows arm64'],
    ['linux/amd64', 'Linux amd64'],
    ['linux/arm64', 'Linux arm64'],
    ['darwin/amd64', 'macOS amd64'],
    ['darwin/arm64', 'macOS arm64']
  ]
  const runtimeBase = item => `/api/plugins/${encodeURIComponent(item.id)}/runtime`

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
      <div className="modal pluginManagerModal" onClick={event => event.stopPropagation()}>
        <div className="modalHeader">
          <div>
            <h3>插件管理</h3>
            <p>上传插件、下载模板，并管理本机已安装插件。</p>
          </div>
          <button className="modalClose" onClick={onClose}>×</button>
        </div>
        <div className="pluginModalActions">
          <section className="pluginQuickActions">
            <a className="primary downloadTemplate" href="/api/dev/toolkit.zip">下载插件模板</a>
            <a className="secondary downloadTemplate" href="/api/plugins/user-workflows.zip">导出用户工作流</a>
          </section>
          <section className="pluginUploadBox">
            <label>
              <span>安装插件 ZIP</span>
              <input type="file" accept=".zip,application/zip" onChange={event => { setFile(event.target.files?.[0] || null); setState({message: '已选择插件 ZIP，点击上传开始安装。'}) }} />
            </label>
            <div className="buttonRow">
              <button className="primary" disabled={!file || uploading} onClick={() => uploadPlugin(false)}>上传</button>
              {state.duplicate && <button className="secondary" disabled={uploading} onClick={() => uploadPlugin(true)}>确认更新</button>}
            </div>
          </section>
          <section className="pluginInstalledListSection">
            <div className="pluginInstalledHeader">
              <div>
                <strong>已安装插件</strong>
                <span>{plugins.length} 个插件，删除前需要先禁用。</span>
              </div>
              <label className="runtimeTargetSelect compact">
                <span>运行包平台</span>
                <select value={target} onChange={event => setTarget(event.target.value)}>
                  {targets.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                </select>
              </label>
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
                  <div className="pluginItemActions">
                    {!item.disabled && (
                      <div className="pluginExportLinks">
                        <a className="secondary" href={`/api/plugins/${encodeURIComponent(item.id)}.zip`}>插件 ZIP</a>
                        <a className="secondary" href={`${runtimeBase(item)}.zip?goos=${encodeURIComponent(goos)}&goarch=${encodeURIComponent(goarch)}`}>运行包 ZIP</a>
                      </div>
                    )}
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
  const [cleanupKeep, setCleanupKeep] = useState(20)
  const [cleanupRunning, setCleanupRunning] = useState(false)
  const [cleanupMessage, setCleanupMessage] = useState('点击“预览清理”查看会被清理的旧运行记录。')
  const [formData, setFormData] = useState({
    appName: '',
    appDescription: '',
    appVersion: '',
    serverEnabled: true,
    serverHost: '127.0.0.1',
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
            serverHost: config.server?.host || '127.0.0.1',
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
        serverHost: config.server?.host || '127.0.0.1',
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

  async function cleanupRuns(dryRun) {
    const keep = Math.max(0, Number(cleanupKeep) || 0)
    if (!dryRun) {
      const ok = window.confirm(`将清理旧的工具/工作流运行记录，仅保留最近 ${keep} 条。该操作不可撤销，是否继续？`)
      if (!ok) return
    }
    setCleanupRunning(true)
    setCleanupMessage(dryRun ? '正在预览可清理的运行记录...' : '正在清理旧运行记录...')
    try {
      const body = await postJSON('/api/runs/cleanup', {keep, dry_run: dryRun})
      const result = body.data || {}
      const ids = (result.deleted_ids || []).slice(0, 8).join('\n')
      const suffix = result.deleted > 8 ? `\n... 另有 ${result.deleted - 8} 条` : ''
      const action = dryRun ? '可清理' : '已清理'
      setCleanupMessage(`${action} ${result.deleted || 0} / ${result.total || 0} 条运行记录，保留最近 ${result.keep ?? keep} 条。${ids ? `\n${ids}${suffix}` : ''}`)
    } catch (err) {
      setCleanupMessage(readableAPIError(err, dryRun ? '预览清理失败。' : '清理运行记录失败。'))
    } finally {
      setCleanupRunning(false)
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
                  <input type="text" value={formData.serverHost} disabled={loading || saving} placeholder="127.0.0.1" onChange={e => updateFormField('serverHost', e.target.value)} />
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
                <div className="runCleanupBox">
                  <div>
                    <strong>运行日志清理</strong>
                    <span>清理工具和工作流的旧运行记录，支持包导出目录会保留。</span>
                  </div>
                  <label className="inlineNumberField">
                    <span>保留最近</span>
                    <input type="number" min="0" value={cleanupKeep} disabled={loading || saving || cleanupRunning} onChange={e => setCleanupKeep(e.target.value)} />
                    <span>条</span>
                  </label>
                  <div className="buttonRow compactButtonRow">
                    <button className="secondary" type="button" disabled={loading || saving || cleanupRunning} onClick={() => cleanupRuns(true)}>预览清理</button>
                    <button className="secondary danger" type="button" disabled={loading || saving || cleanupRunning} onClick={() => cleanupRuns(false)}>清理旧记录</button>
                  </div>
                  <pre className="runCleanupResult" aria-live="polite">{cleanupMessage}</pre>
                </div>
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

function ResultView({result, expanded = false}) {
  const className = expanded ? 'resultView resultViewExpanded' : 'resultView'
  if (typeof result === 'string') {
    return <div className={className}><pre>{result}</pre></div>
  }
  if (result?.detail?.data) {
    return <div className={className}><RunDetail detail={result.detail.data} run={result.run} /></div>
  }
  if (result?.response) {
    return <div className={className}><MessageWithDetails message={result.message} details={result.response} /></div>
  }
  return <div className={className}><pre>{result?.message || '暂无结果'}</pre></div>
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
  const runID = record.id || run?.id
  return (
    <div className="runDetail">
      <div className="runSummary">
        <span>运行 ID：{runID}</span>
        <span>状态：{record.status || run?.status}</span>
        <span>目标：{record.target || '-'}</span>
      </div>
      {runID && (
        <div className="buttonRow">
          <a className="secondary" href={`/api/runs/${encodeURIComponent(runID)}/support.zip`}>导出支持包</a>
        </div>
      )}
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

function RunPanel({entries, totalEntries, selected, params, setParams, selectEntry, runSelected, searchText, setSearchText, activeTag, setActiveTag, availableTags}) {
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const paginationEnabled = true
  const totalPages = paginationEnabled ? Math.max(1, Math.ceil(entries.length / pageSize)) : 1
  const safePage = Math.min(currentPage, totalPages)
  const visibleEntries = paginationEnabled ? entries.slice((safePage - 1) * pageSize, safePage * pageSize) : entries

  useEffect(() => {
    setCurrentPage(1)
  }, [searchText, activeTag, totalEntries])

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
          <h3>工具列表</h3>
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
          {entries.length === 0 && <div className="empty">没有匹配的工具。</div>}
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
          {selected && <span>工具</span>}
        </div>
        {!selected ? <div className="empty">请先选择一个工具。</div> : (
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

createRoot(document.getElementById('root')).render(<App />)
