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
    preview: ['输入：选择上游输出', '分支：case1 / case2 / default'],
    help: '适合根据巡检结果、返回文本、参数值做分流',
    enabled: true
  },
  {
    type: 'parallel',
    title: '并行分支',
    secondary: 'Parallel',
    description: '将后续任务拆分为多个并行执行路径',
    capabilities: ['多路径', '并发执行', '规划中'],
    preview: ['输入：一个上游节点', '输出：多个并行分支'],
    help: '规划中：后续用于同时执行多组独立运维动作',
    enabled: false
  },
  {
    type: 'merge',
    title: '合流',
    secondary: 'Merge',
    description: '等待多个分支汇聚后继续执行',
    capabilities: ['分支汇聚', '等待策略', '规划中'],
    preview: ['输入：多个分支', '输出：继续流程'],
    help: '规划中：后续用于明确分支汇聚和等待语义',
    enabled: false
  },
  {
    type: 'loop',
    title: '循环',
    secondary: 'Loop',
    description: '按条件重复执行一段流程',
    capabilities: ['重复执行', '退出条件', '规划中'],
    preview: ['输入：循环范围', '退出：满足条件后继续'],
    help: '规划中：后续用于批量或重试类流程控制',
    enabled: false
  }
]

const nodeTypes = {toolNode: ToolNode, conditionNode: ConditionNode}

function ToolNode({id, data, selected}) {
  return (
    <div className={selected ? 'toolNode selected' : 'toolNode'}>
      <Handle type="target" position={Position.Left} />
      <button className="nodeDelete nodrag nopan" onMouseDown={event => event.stopPropagation()} onClick={event => { event.stopPropagation(); data.onRemove(id) }} title="删除节点">×</button>
      <strong>{data.name || id}</strong>
      <span>{data.tool}</span>
      <Handle type="source" position={Position.Right} />
    </div>
  )
}


function ConditionNode({id, data, selected}) {
  const condition = data.condition || defaultCondition()
  const status = conditionNodeStatus(condition)
  const branches = conditionBranchRows(condition)
  return (
    <div className={selected ? 'conditionNode selected' : 'conditionNode'}>
      <Handle type="target" position={Position.Left} />
      <div className="conditionDiamond" aria-hidden="true"><span>分</span></div>
      <button className="nodeDelete nodrag nopan" onMouseDown={event => event.stopPropagation()} onClick={event => { event.stopPropagation(); data.onRemove(id) }} title="删除节点">×</button>
      <div className="conditionInfoCard">
        <div className="conditionNodeHeader">
          <span>条件分支</span>
          <strong>{data.name || id}</strong>
        </div>
        <div className="conditionInputSummary">{conditionSummary(condition)}</div>
        <small>{conditionCaseSummary(condition)}</small>
        <div className={status.ready ? 'conditionStatus ready' : 'conditionStatus warning'}>{status.label}</div>
      </div>
      <div className="conditionBranchList" aria-label="条件分支出口">
        {branches.map(branch => (
          <div key={branch.key} className={`conditionBranchRow ${branch.kind}${branch.disabled ? ' disabled' : ''}`}>
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

  async function refreshCatalog(options = {}) {
    const body = await fetchJSON('/api/catalog')
    const data = body.data
    setCatalog(data)
    if (options.keepCategory) return data
    setActiveCategory(current => current || '')
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

  const sourceEntries = useMemo(() => {
    if (!catalog) return []
    const source = activeTab === 'tools' ? catalog.tools || [] : catalog.workflows || []
    if (!activeCategory) return source
    return source.filter(item => item.category === activeCategory)
  }, [catalog, activeCategory, activeTab])

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
          {(catalog.categories || []).map(item => (
            <button
              key={item.id}
              className={item.id === activeCategory ? 'category active' : 'category'}
              onClick={() => { setActiveCategory(item.id); setSelected(null); setActiveTag(''); resetResult() }}
            >
              <span>{item.name || item.id}</span>
              <small>{item.description}</small>
            </button>
          ))}
        </div>
        <button className="pluginAction" onClick={() => setPluginModalOpen(true)} title="插件管理">+</button>
      </aside>

      <main className="content">
        <header className="topbar">
          <div>
            <h2>{category?.name || '全局工作流'}</h2>
            <p>{category?.description || '跨分类选择工具、工作流或打开编排器'}</p>
          </div>
          <div className="hint">可视化工作流编排</div>
        </header>

        <div className="tabs">
          <button className={activeTab === 'tools' ? 'tab active' : 'tab'} onClick={() => { setActiveTab('tools'); setSelected(null); setActiveTag(''); resetResult() }}>工具</button>
          <button className={activeTab === 'workflows' ? 'tab active' : 'tab'} onClick={() => { setActiveTab('workflows'); setSelected(null); setActiveTag(''); resetResult() }}>工作流</button>
          <button className={activeTab === 'editor' ? 'tab active' : 'tab'} onClick={() => { setActiveTab('editor'); setSelected(null); setActiveTag(''); resetResult() }}>编排器</button>
        </div>

        {activeTab === 'editor' ? (
          <WorkflowEditor catalog={catalog} activeCategory={activeCategory} setResult={setResult} refreshCatalog={refreshCatalog} />
        ) : (
          <RunPanel activeTab={activeTab} entries={entries} totalEntries={sourceEntries.length} selected={selected} params={params} setParams={setParams} selectEntry={selectEntry} runSelected={runSelected} searchText={searchText} setSearchText={setSearchText} activeTag={activeTag} setActiveTag={setActiveTag} availableTags={availableTags} />
        )}

        <section className="card resultCard">
          <div className="cardHeader">
            <h3>执行结果</h3>
          </div>
          <ResultView result={result} />
        </section>
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
        />
      )}
    </div>
  )
}

function PluginManagerModal({catalog, state, setState, onClose, onUploaded}) {
  const [file, setFile] = useState(null)
  const [uploading, setUploading] = useState(false)
  const exportablePlugins = useMemo(() => [...(catalog?.plugins || [])].sort((left, right) => String(left.id || '').localeCompare(String(right.id || ''), 'zh-CN')), [catalog])

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

  return (
    <div className="modalBackdrop" onClick={onClose}>
      <div className="modal" onClick={event => event.stopPropagation()}>
        <div className="modalHeader">
          <div>
            <h3>插件管理</h3>
            <p>下载插件模板或上传一个插件 ZIP 包，底部提供插件导出等其他选项。</p>
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
            <section className="pluginExportPanel">
              <div>
                <strong>导出已安装插件</strong>
                <span>每个插件将下载为可再次上传安装的标准 ZIP 包。</span>
              </div>
              <div className="pluginExportList">
                {exportablePlugins.map(item => (
                  <div className="pluginExportItem" key={item.id}>
                    <div>
                      <strong>{item.name || item.id}</strong>
                      <span>{item.id}@{item.version || '-'}</span>
                      {item.description && <small>{item.description}</small>}
                    </div>
                    <a className="secondary" href={`/api/plugins/${encodeURIComponent(item.id)}.zip`}>导出</a>
                  </div>
                ))}
                {exportablePlugins.length === 0 && <div className="empty small">当前没有可导出的已安装插件。</div>}
              </div>
            </section>
          </section>
        </div>
        <pre className="modalResult">{state.response ? JSON.stringify(state.response, null, 2) : state.message}</pre>
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
          {entries.map(entry => (
            <button key={entry.id} className={selected?.id === entry.id ? 'entry active' : 'entry'} onClick={() => selectEntry(entry)}>
              <strong>{entry.name || entry.id}</strong>
              <span>{entry.description || '暂无描述'}</span>
              <em>{entry.id}</em>
              <TagList tags={entry.tags || []} />
            </button>
          ))}
          {entries.length === 0 && <div className="empty">没有匹配的{activeTab === 'tools' ? '工具' : '工作流'}。</div>}
        </div>
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
  const workflowOptions = useMemo(() => {
    return (catalog.workflows || []).filter(workflow => !activeCategory || workflow.category === activeCategory)
  }, [catalog, activeCategory])
  const workflowScope = workflowScopeCategory(workflow.category)
  const scopedCategory = workflowScope === 'global' ? '' : workflowScope
  const toolOptions = useMemo(() => {
    return (catalog.tools || []).filter(tool => !scopedCategory || tool.category === scopedCategory)
  }, [catalog, scopedCategory])
  const workflowTagOptions = useMemo(() => tagsForEntries([...(catalog.workflows || []), workflow]), [catalog.workflows, workflow])
  const [selectedWorkflowID, setSelectedWorkflowID] = useState('')
  const [selectedNodeID, setSelectedNodeID] = useState('')
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

  useEffect(() => {
    setWorkflow(prev => ({...prev, category: activeCategory || 'global'}))
    setEditorActiveTag('')
    setEditorValidation(null)
  }, [activeCategory])

  const [selectedEdgeID, setSelectedEdgeID] = useState('')
  const selectedNode = useMemo(() => nodes.find(node => node.id === selectedNodeID), [nodes, selectedNodeID])
  const selectedEdge = useMemo(() => edges.find(edge => edge.id === selectedEdgeID), [edges, selectedEdgeID])
  const selectedTool = useMemo(() => (catalog.tools || []).find(tool => tool.id === selectedNode?.data.tool), [catalog.tools, selectedNode])
  const editorAvailableTags = useMemo(() => tagsForEntries(toolOptions), [toolOptions])
  const editorToolOptions = useMemo(() => filterEntries(toolOptions, editorSearchText, editorActiveTag), [toolOptions, editorSearchText, editorActiveTag])
  const nodePickerToolOptions = useMemo(() => filterEntries(toolOptions, nodePickerSearchText, ''), [toolOptions, nodePickerSearchText])
  const workflowParameters = useMemo(() => parseJSONList(workflowParamsText), [workflowParamsText])
  const mappingSources = useMemo(() => buildMappingSources(workflowParameters, selectedNodeID, nodes, edges), [workflowParameters, selectedNodeID, nodes, edges])

  useEffect(() => {
    if (!selectedNode) {
      setNodeParamsText('{}')
      return
    }
    setNodeParamsText(JSON.stringify(selectedNode.data.params || {}, null, 2))
  }, [selectedNode])

  const onConnect = useCallback(
    params => setEdges(current => {
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
    }),
    [nodes, setEdges]
  )

  async function loadWorkflow(id) {
    if (!id) return
    setResult({message: '加载工作流...'})
    try {
      const body = await fetchJSON(`/api/workflows/${id}`)
      const config = body.data.Config || body.data.config
      setWorkflow(config)
      setWorkflowParamsText(JSON.stringify(config.parameters || [], null, 2))
      setRunParamsText(JSON.stringify(defaultParams(config.parameters || []), null, 2))
      const flowNodes = (config.nodes || []).map((node, index) => workflowNodeToFlowNode(node, index, removeNode))
      setNodes(flowNodes)
      setEdges((config.edges || []).map((edge, index) => flowEdgeFromWorkflowEdge(edge, index, flowNodes)))
      setSelectedWorkflowID(id)
      setSelectedNodeID('')
      setSelectedEdgeID('')
      closeNodePicker()
      setResult({message: `已加载工作流 ${id}`})
    } catch (err) {
      setResult({message: String(err)})
    }
  }

  function createWorkflow() {
    const next = emptyWorkflow(activeCategory)
    setWorkflow(next)
    setWorkflowParamsText('[]')
    setRunParamsText('{}')
    setNodes([])
    setEdges([])
    setSelectedWorkflowID('')
    setSelectedNodeID('')
    setSelectedEdgeID('')
    closeNodePicker()
    setResult({message: '已创建空白工作流草稿'})
  }

  const removeNode = useCallback(id => {
    setNodes(current => current.filter(node => node.id !== id))
    setEdges(current => current.filter(edge => edge.source !== id && edge.target !== id))
    setResult({message: `已移除节点 ${id}`})
    setSelectedNodeID(current => current === id ? '' : current)
    setSelectedEdgeID('')
  }, [setEdges, setNodes, setResult])

  function addToolNode(tool, position) {
    const nodeID = uniqueNodeID(tool.id, nodes)
    const nextNode = newToolFlowNode(tool, nodeID, position || {x: 80 + nodes.length * 220, y: 120 + (nodes.length % 3) * 90}, removeNode)
    setNodes(current => [...current, nextNode])
    setSelectedNodeID(nodeID)
    setSelectedEdgeID('')
    setEditorValidation(null)
    closeNodePicker()
  }

  function addConditionNode(position) {
    const nodeID = uniqueNodeID('condition', nodes)
    const nextNode = newConditionFlowNode(nodeID, position || {x: 80 + nodes.length * 220, y: 120 + (nodes.length % 3) * 90}, removeNode)
    setNodes(current => [...current, nextNode])
    setSelectedNodeID(nodeID)
    setSelectedEdgeID('')
    setEditorValidation(null)
    closeNodePicker()
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

  function updateSelectedNodeName(nextName) {
    setNodes(current => current.map(node => node.id === selectedNodeID ? {...node, data: {...node.data, name: nextName}} : node))
  }

  function updateSelectedNodeCondition(nextCondition) {
    setNodes(current => current.map(node => node.id === selectedNodeID ? {...node, data: {...node.data, condition: nextCondition}} : node))
    syncConditionEdgeLabels(selectedNodeID, nextCondition)
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
    if (controlType === 'condition') {
      addConditionNode(position)
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
    setEdges(current => current.filter(edge => edge.id !== selectedEdgeID))
    setResult({message: `已移除依赖 ${selectedEdgeID}`})
    setSelectedEdgeID('')
  }

  function clearSelection() {
    setSelectedNodeID('')
    setSelectedEdgeID('')
  }

  function updateWorkflowCategory(value) {
    if (activeCategory && value !== activeCategory) return
    setWorkflow(current => ({...current, category: workflowScopeCategory(value)}))
    setEditorValidation(null)
  }

  function updateWorkflowTags(nextTags) {
    setWorkflow(current => ({...current, tags: normalizeTags(nextTags)}))
    setEditorValidation(null)
  }

  function applyNodeParams() {
    if (!selectedNodeID) return
    try {
      updateSelectedNodeParams(JSON.parse(nodeParamsText || '{}'))
      setResult({message: `已更新节点 ${selectedNodeID} 参数`})
    } catch (err) {
      setResult({message: `参数（JSON） 无效: ${err.message}`})
    }
  }

  function updateSelectedNodeParams(nextParams) {
    setNodes(current => current.map(node => node.id === selectedNodeID ? {...node, data: {...node.data, params: nextParams}} : node))
    setNodeParamsText(JSON.stringify(nextParams, null, 2))
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
    if (activeCategory && draft.category !== activeCategory) errors.push(`当前分类上下文只能保存为 ${activeCategory} 工作流。`)
    if (!String(draft.id || '').trim()) errors.push('请先填写工作流 ID。')
    if (!String(draft.name || '').trim()) errors.push('请先填写工作流名称。')
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
      showEditorValidation(check)
      return
    }
    try {
      setEditorValidation(null)
      let runParams = {}
      try {
        runParams = JSON.parse(runParamsText || '{}')
      } catch (err) {
        const status = {title: '执行前检查未通过', errors: [`执行参数 JSON 无效：${err.message}`], warnings: []}
        showEditorValidation(status)
        return
      }
      setResult({message: '执行工作流...'})
      const body = await postJSON(`/api/workflows/${check.draft.id}/run`, {params: runParams})
      if (body.id) {
        setResult({run: body, detail: await fetchRunDetail(body.id)})
        return
      }
      setResult({message: summarizeAPIResponse(body, '工作流已提交执行。'), response: body})
    } catch (err) {
      setResult({message: readableAPIError(err, '工作流执行失败。'), response: err.body})
    }
  }

  return (
    <div className="editorLayout">
      <section className="card editorToolbar">
        <div className="cardHeader">
          <h3>工作流编排器</h3>
          <span>{nodes.length} 节点 / {edges.length} 依赖</span>
        </div>
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
            <input value={workflow.id || ''} onChange={event => setWorkflow({...workflow, id: event.target.value})} placeholder="demo.my-flow" />
          </label>
          <label>
            <span>名称</span>
            <input value={workflow.name || ''} onChange={event => setWorkflow({...workflow, name: event.target.value})} placeholder="工作流名称" />
          </label>
          <label>
            <span>描述</span>
            <input value={workflow.description || ''} onChange={event => setWorkflow({...workflow, description: event.target.value})} placeholder="工作流描述" />
          </label>
          <label>
            <span>分类 / 范围</span>
            <select value={workflow.category || 'global'} onChange={event => updateWorkflowCategory(event.target.value)} disabled={Boolean(activeCategory)}>
              {!activeCategory && <option value="global">全局工作流（可选择全部工具）</option>}
              {(catalog.categories || [])
                .filter(item => !activeCategory || item.id === activeCategory)
                .map(item => <option key={item.id} value={item.id}>{item.name || item.id}（仅当前分类工具）</option>)}
            </select>
          </label>
          <WorkflowTagsEditor tags={workflow.tags || []} availableTags={workflowTagOptions} onChange={updateWorkflowTags} />
          <label>
            <span>工作流参数（JSON）</span>
            <textarea className="smallTextarea" value={workflowParamsText} onChange={event => setWorkflowParamsText(event.target.value)} />
          </label>
          <label>
            <span>执行参数（JSON）</span>
            <textarea className="smallTextarea" value={runParamsText} onChange={event => setRunParamsText(event.target.value)} />
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
                <button key={tool.id} type="button" className="paletteItem" draggable onDragStart={event => handleToolDragStart(event, tool)} onClick={() => addToolNode(tool)}>
                  <span>{tool.name || tool.id}</span>
                  <em>{tool.id}</em>
                  <small>拖到画布，或点击添加</small>
                  <TagList tags={tool.tags || []} />
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
                  onDragStart={event => {
                    if (disabled) return
                    handleControlDragStart(event, control)
                  }}
                  onClick={() => control.type === 'condition' && addConditionNode()}
                  title={control.help}
                >
                  <div className="controlIcon" aria-hidden="true">◇</div>
                  <div className="controlContent">
                    <div className="controlTitle"><strong>{control.title}</strong><span>{control.secondary}</span>{disabled && <b>规划中</b>}</div>
                    <p>{control.description}</p>
                    <div className="capabilityChips">
                      {control.capabilities.map(item => <span key={item}>{item}</span>)}
                    </div>
                    <div className="controlPreview">
                      {control.preview.map(item => <small key={item}>{item}</small>)}
                    </div>
                    <em>{control.help}</em>
                  </div>
                </button>
              )
            })}
          </div>
        )}
      </section>

      <section className="card canvasCard" ref={canvasCardRef} onDragOver={handleCanvasDragOver} onDrop={handleCanvasDrop}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          onNodeClick={(_, node) => { setSelectedNodeID(node.id); setSelectedEdgeID(''); closeNodePicker() }}
          onEdgeClick={(_, edge) => { setSelectedEdgeID(edge.id); setSelectedNodeID(''); closeNodePicker() }}
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
              onAddCondition={() => addConditionNode(nodePicker.position)}
              onClose={closeNodePicker}
            />
          )}
          <CanvasDock
            onZoomIn={() => zoomCanvas('in')}
            onZoomOut={() => zoomCanvas('out')}
            onFitView={fitCanvasView}
            onAddNode={() => openNodePicker()}
            onRunWorkflow={runDraft}
          />
        </ReactFlow>
      </section>

      <section className="card nodeInspector">
        <div className="cardHeader">
          <h3>节点参数</h3>
          {selectedNode && <span>{selectedNode.id}</span>}
          {selectedEdge && <span>{selectedEdge.source} → {selectedEdge.target}</span>}
        </div>
        {editorValidation?.errors?.length > 0 && <ValidationSummary status={editorValidation} />}
        {selectedEdge && <EdgeInspector edge={selectedEdge} sourceNode={nodes.find(node => node.id === selectedEdge.source)} onCaseChange={updateSelectedEdgeCase} />}
        {!selectedNode ? (!selectedEdge && <div className="empty small">点击画布节点后编辑参数，点击连线后可删除依赖。</div>) : (
          <div className="form compact">
            {selectedNode.type !== 'conditionNode' ? (
              <>
                <label>
                  <span>节点标识</span>
                  <input value={selectedNode.id} disabled />
                </label>
                <label>
                  <span>工具标识</span>
                  <input value={selectedNode.data.tool} disabled />
                </label>
                <ParamMappingEditor tool={selectedTool} params={selectedNode.data.params || {}} sources={mappingSources} onChange={updateMappedParam} />
                <details className="advancedParams">
                  <summary>高级 JSON 编辑</summary>
                  <label>
                    <span>参数（JSON）</span>
                    <textarea value={nodeParamsText} onChange={event => setNodeParamsText(event.target.value)} />
                  </label>
                  <div className="empty small">可引用上游节点：{"{{ .steps.节点ID.stdout }}"} 或 {"{{ .steps.节点ID.params.参数名 }}"}</div>
                  <button className="secondary" onClick={applyNodeParams}>应用参数</button>
                </details>
              </>
            ) : (
              <ConditionEditor node={selectedNode} sources={mappingSources} onNameChange={updateSelectedNodeName} onChange={updateSelectedNodeCondition} />
            )}
          </div>
        )}
      </section>
    </div>
  )
}

function NodePickerPanel({searchText, setSearchText, tools, totalTools, onAddTool, onAddCondition, onClose}) {
  const enabledControl = controlNodeCatalog.find(control => control.type === 'condition')
  const keyword = searchText.trim().toLowerCase()
  const showCondition = !keyword || [enabledControl?.title, enabledControl?.secondary, enabledControl?.description, enabledControl?.help]
    .filter(Boolean)
    .some(value => String(value).toLowerCase().includes(keyword))
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
        {showCondition && (
          <div className="nodePickerSection">
            <span>编排节点</span>
            <button type="button" className="nodePickerItem control" onClick={onAddCondition}>
              <b>{enabledControl?.title || '条件分支'}</b>
              <small>{enabledControl?.secondary || 'Switch / Case'} · 根据结果选择后续分支</small>
            </button>
          </div>
        )}
        <div className="nodePickerSection">
          <span>插件工具 · {tools.length} / {totalTools}</span>
          <div className="nodePickerList">
            {tools.map(tool => (
              <button key={tool.id} type="button" className="nodePickerItem" onClick={() => onAddTool(tool)}>
                <b>{tool.name || tool.id}</b>
                <small>{tool.id}</small>
              </button>
            ))}
            {tools.length === 0 && <div className="empty small">没有匹配的插件工具。</div>}
          </div>
        </div>
      </div>
    </div>
  )
}

function CanvasDock({onZoomIn, onZoomOut, onFitView, onAddNode, onRunWorkflow}) {
  return (
    <div className="canvasDock nodrag nopan" onMouseDown={event => event.stopPropagation()}>
      <button type="button" onClick={onZoomOut} title="缩小">−</button>
      <button type="button" onClick={onZoomIn} title="放大">+</button>
      <button type="button" onClick={onFitView}>适配视图</button>
      <button type="button" onClick={onAddNode}>添加节点</button>
      <button type="button" className="canvasDockPrimary" onClick={onRunWorkflow}>运行工作流</button>
    </div>
  )
}

function EdgeInspector({edge, sourceNode, onCaseChange}) {
  if (sourceNode?.type !== 'conditionNode') {
    return <div className="empty small">已选择依赖线，可在上方点击“删除依赖”。</div>
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

function ParamMappingEditor({tool, params, sources, onChange}) {
  const parameters = tool?.parameters || []
  if (!tool) return <div className="empty small">未找到当前节点工具定义。</div>
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

function workflowNodeToFlowNode(node, index, onRemove) {
  const nodeType = node.type || (node.tool ? 'tool' : 'condition')
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
    .filter(node => node.type !== 'conditionNode')
    .map(node => ({node, tool: toolMap.get(node.data.tool)}))
    .filter(item => item.tool && item.tool.category !== scopedCategory)
    .map(item => ({nodeID: item.node.id, toolID: item.node.data.tool, scopeName: scopedCategory}))
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
    if (node.type === 'conditionNode') return
    const tool = toolMap.get(node.data.tool)
    ;(tool?.parameters || []).forEach(param => {
      if (!param.required) return
      const value = node.data.params?.[param.name]
      if (value === undefined || value === null || String(value).trim() === '') {
        missing.push({nodeID: node.id, toolName: tool.name || tool.id, paramName: param.name})
      }
    })
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
