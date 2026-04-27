import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { createRoot } from 'react-dom/client'
import {
  addEdge,
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

const nodeTypes = {toolNode: ToolNode}

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

function App() {
  const [catalog, setCatalog] = useState(null)
  const [activeCategory, setActiveCategory] = useState('')
  const [activeTab, setActiveTab] = useState('tools')
  const [selected, setSelected] = useState(null)
  const [params, setParams] = useState({})
  const [searchText, setSearchText] = useState('')
  const [activeTag, setActiveTag] = useState('')
  const [result, setResult] = useState({message: '等待执行...'})

  async function refreshCatalog(options = {}) {
    const body = await fetchJSON('/api/catalog')
    const data = body.data
    setCatalog(data)
    if (options.keepCategory) return data
    setActiveCategory(current => current || data.categories?.[0]?.id || '')
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
    if (!catalog || !activeCategory) return []
    const source = activeTab === 'tools' ? catalog.tools || [] : catalog.workflows || []
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
      setResult({message: JSON.stringify(body, null, 2)})
    } catch (err) {
      setResult({message: String(err)})
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
      </aside>

      <main className="content">
        <a className="downloadKit floatingDownload" href="/api/dev/toolkit.zip">下载工具开发包</a>
        <header className="topbar">
          <div>
            <h2>{category?.name || '未选择分类'}</h2>
            <p>{category?.description || '选择工具、工作流或打开编排器'}</p>
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
  return <pre>{result?.message || '暂无结果'}</pre>
}

function RunDetail({detail, run}) {
  const record = detail.record || {}
  const logs = detail.logs || {}
  return (
    <div className="runDetail">
      <div className="runSummary">
        <span>运行 ID：{record.id || run?.id}</span>
        <span>状态：{record.status || run?.status}</span>
        <span>目标：{record.target || '-'}</span>
      </div>
      <LogBlock title="标准输出" value={logs.stdout} />
      <LogBlock title="错误输出" value={logs.stderr} />
      {logs.steps && Object.entries(logs.steps).map(([id, stepLogs]) => (
        <div key={id} className="stepLogs">
          <h4>节点日志：{id}</h4>
          <LogBlock title="标准输出" value={stepLogs.stdout} />
          <LogBlock title="错误输出" value={stepLogs.stderr} />
        </div>
      ))}
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
  const toolOptions = useMemo(() => {
    return (catalog.tools || []).filter(tool => !activeCategory || tool.category === activeCategory)
  }, [catalog, activeCategory])
  const workflowOptions = useMemo(() => {
    return (catalog.workflows || []).filter(workflow => !activeCategory || workflow.category === activeCategory)
  }, [catalog, activeCategory])
  const [workflow, setWorkflow] = useState(emptyWorkflow(activeCategory))
  const [selectedWorkflowID, setSelectedWorkflowID] = useState('')
  const [selectedNodeID, setSelectedNodeID] = useState('')
  const [workflowParamsText, setWorkflowParamsText] = useState('[]')
  const [runParamsText, setRunParamsText] = useState('{}')
  const [nodeParamsText, setNodeParamsText] = useState('{}')
  const [flowInstance, setFlowInstance] = useState(null)
  const [nodes, setNodes, onNodesChange] = useNodesState([])
  const [edges, setEdges, onEdgesChange] = useEdgesState([])

  useEffect(() => {
    setWorkflow(prev => ({...prev, category: prev.category || activeCategory}))
  }, [activeCategory])

  const [selectedEdgeID, setSelectedEdgeID] = useState('')
  const selectedNode = useMemo(() => nodes.find(node => node.id === selectedNodeID), [nodes, selectedNodeID])
  const selectedEdge = useMemo(() => edges.find(edge => edge.id === selectedEdgeID), [edges, selectedEdgeID])
  const selectedTool = useMemo(() => (catalog.tools || []).find(tool => tool.id === selectedNode?.data.tool), [catalog.tools, selectedNode])
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
    params => setEdges(current => addEdge({...params, type: 'smoothstep', animated: true}, current)),
    [setEdges]
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
      setNodes((config.nodes || []).map((node, index) => workflowNodeToFlowNode(node, index, removeNode)))
      setEdges((config.edges || []).map(edge => ({id: `${edge.from}-${edge.to}`, source: edge.from, target: edge.to, type: 'smoothstep', animated: true})))
      setSelectedWorkflowID(id)
      setSelectedNodeID('')
      setSelectedEdgeID('')
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
  }

  function handleToolDragStart(event, tool) {
    event.dataTransfer.setData('application/ops-tool', tool.id)
    event.dataTransfer.effectAllowed = 'move'
  }

  function handleCanvasDragOver(event) {
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
  }

  function handleCanvasDrop(event) {
    event.preventDefault()
    const toolID = event.dataTransfer.getData('application/ops-tool')
    const tool = toolOptions.find(item => item.id === toolID)
    if (!tool || !flowInstance) return
    addToolNode(tool, flowInstance.screenToFlowPosition({x: event.clientX, y: event.clientY}))
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

  function currentDraft() {
    const workflowParameters = JSON.parse(workflowParamsText || '[]')
    if (!Array.isArray(workflowParameters)) {
      throw new Error('工作流参数必须是 JSON 数组')
    }
    return buildWorkflowDraft(workflow, nodes, edges, activeCategory, workflowParameters)
  }

  async function validateDraft() {
    try {
      const draft = currentDraft()
      setWorkflow(draft)
      const body = await postJSON(`/api/workflows/${draft.id || 'draft'}/validate`, {workflow: draft})
      setResult({message: JSON.stringify(body, null, 2)})
    } catch (err) {
      setResult({message: String(err)})
    }
  }

  async function saveDraft() {
    try {
      const draft = currentDraft()
      setWorkflow(draft)
      const body = await postJSON(`/api/workflows/${draft.id}/save`, {workflow: draft})
      setSelectedWorkflowID(draft.id)
      await refreshCatalog({keepCategory: true})
      setResult({message: JSON.stringify(body, null, 2)})
    } catch (err) {
      setResult({message: String(err)})
    }
  }

  async function runDraft() {
    try {
      const draft = currentDraft()
      const runParams = JSON.parse(runParamsText || '{}')
      if (!draft.id) throw new Error('请先填写工作流 ID')
      setResult({message: '执行工作流...'})
      const body = await postJSON(`/api/workflows/${draft.id}/run`, {params: runParams})
      if (body.id) {
        setResult({run: body, detail: await fetchRunDetail(body.id)})
        return
      }
      setResult({message: JSON.stringify(body, null, 2)})
    } catch (err) {
      setResult({message: String(err)})
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
            <span>工作流参数（JSON）</span>
            <textarea className="smallTextarea" value={workflowParamsText} onChange={event => setWorkflowParamsText(event.target.value)} />
          </label>
          <label>
            <span>执行参数（JSON）</span>
            <textarea className="smallTextarea" value={runParamsText} onChange={event => setRunParamsText(event.target.value)} />
          </label>
        </div>
        <div className="toolPalette">
          <strong>工具节点</strong>
          {toolOptions.map(tool => (
            <button key={tool.id} className="paletteItem" draggable onDragStart={event => handleToolDragStart(event, tool)} onClick={() => addToolNode(tool)}>
              <span>{tool.name || tool.id}</span>
              <em>{tool.id}</em>
              <small>拖到画布，或点击添加</small>
            </button>
          ))}
        </div>
      </section>

      <section className="card canvasCard" onDragOver={handleCanvasDragOver} onDrop={handleCanvasDrop}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          onNodeClick={(_, node) => { setSelectedNodeID(node.id); setSelectedEdgeID('') }}
          onEdgeClick={(_, edge) => { setSelectedEdgeID(edge.id); setSelectedNodeID('') }}
          onPaneClick={clearSelection}
          onInit={setFlowInstance}
          fitView
        >
          <MiniMap />
          <Controls />
          <Background />
        </ReactFlow>
      </section>

      <section className="card nodeInspector">
        <div className="cardHeader">
          <h3>节点参数</h3>
          {selectedNode && <span>{selectedNode.id}</span>}
          {selectedEdge && <span>{selectedEdge.source} → {selectedEdge.target}</span>}
        </div>
        {selectedEdge && <div className="empty small">已选择依赖线，可在左侧点击“删除依赖”。</div>}
        {!selectedNode ? (!selectedEdge && <div className="empty small">点击画布节点后编辑参数，点击连线后可删除依赖。</div>) : (
          <div className="form compact">
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
          </div>
        )}
      </section>
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
    category: category || '',
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
      params: defaultParams(tool.parameters || {}),
      onRemove
    },
    position
  }
}

function workflowNodeToFlowNode(node, index, onRemove) {
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

function buildWorkflowDraft(workflow, nodes, edges, category, parameters) {
  return {
    ...workflow,
    category: workflow.category || category || '',
    parameters: parameters || workflow.parameters || [],
    nodes: nodes.map(node => ({
      id: node.id,
      name: node.data.name || node.id,
      tool: node.data.tool,
      params: node.data.params || {},
      on_failure: node.data.on_failure || 'stop'
    })),
    edges: edges.map(edge => ({from: edge.source, to: edge.target}))
  }
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
  if (!res.ok) throw new Error(body.error || res.statusText)
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
  if (!res.ok) throw new Error(body.error || res.statusText)
  return body
}

createRoot(document.getElementById('root')).render(<App />)
