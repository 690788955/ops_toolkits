import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {Background, Controls, MiniMap, ReactFlow, reconnectEdge, useEdgesState, useNodesState} from '@xyflow/react'
import {deleteJSON, fetchJSON, fetchRunDetail, postJSON, postRunUploadNodeChunked} from '../api.js'
import {filterEntries, readableAPIError, summarizeAPIResponse} from '../utils.js'
import {allNodeCategories, controlNodeCatalog, controlNodeHelp, controlNodeTitle, countNodesByCategory, flowNodeCategoryID, groupControlsByCategory, isControlReady} from './catalog.js'
import * as workflowModel from './model.js'
import {controlShapeMarker, nodeTypes, normalizeRunStatus, runStatusLabel} from './nodes.jsx'
import {EdgeConfigModal, NodeConfigEditor, NodeConfigModal, ValidationSummary} from './inspectors.jsx'
import {sshHostStatusLabel} from '../SshHostsModal.jsx'
import {CanvasDock, NodePickerPanel} from './picker.jsx'
import FileUploadInput from '../FileUploadInput.jsx'

const MAX_PLATFORM_UPLOAD_BYTES = 20 * 1024 * 1024 * 1024
const REMEMBERED_WORKFLOW_PREFIX = 'ops.workflowEditor.selectedWorkflow'
const UNCATEGORIZED_WORKFLOW_CATEGORY = '__uncategorized__'
// 工作流运行级执行目标：follow_nodes 表示沿用各节点自己的 host 配置。
const RUN_TARGET_FOLLOW = 'follow_nodes'
const RUN_TARGET_LOCAL = 'local'

export default function WorkflowEditor({catalog, activeCategory = '', hosts = [], setResult: setAppResult, refreshCatalog, resultPanel = null}) {
  const [workflow, setWorkflow] = useState(emptyWorkflow())
  const [editorNotice, setEditorNotice] = useState('')
  const setResult = useCallback(payload => {
    if (typeof payload === 'function') {
      setAppResult(payload)
      return
    }
    if (payload?.message) setEditorNotice(payload.message)
    setAppResult(payload)
  }, [setAppResult])
  const workflowOptions = useMemo(() => catalog.workflows || [], [catalog.workflows])
  const toolOptions = useMemo(() => catalog.tools || [], [catalog.tools])
  const [workflowCategoryFilter, setWorkflowCategoryFilter] = useState(activeCategory || '')
  const [selectedWorkflowID, setSelectedWorkflowID] = useState('')
  const [workflowOwnerID, setWorkflowOwnerID] = useState('user.workflows')
  const [workflowOwnerLocked, setWorkflowOwnerLocked] = useState(false)
  const [canvasMode, setCanvasMode] = useState('execute')
  const [workflowAction, setWorkflowAction] = useState('')
  const [savedDraftSignature, setSavedDraftSignature] = useState(() => workflowDraftSignature(emptyWorkflow(), [], [], '', '[]', 'user.workflows'))
  const [executeDetailView, setExecuteDetailView] = useState('canvas')
  const [selectedNodeID, setSelectedNodeID] = useState('')
  const [selectedEdgeID, setSelectedEdgeID] = useState('')
  const [nodeConfigModalOpen, setNodeConfigModalOpen] = useState(false)
  const [edgeConfigModalOpen, setEdgeConfigModalOpen] = useState(false)
  const [workflowParamsText, setWorkflowParamsText] = useState('[]')
  const [runParamsText, setRunParamsText] = useState('{}')
  const [runTarget, setRunTarget] = useState(RUN_TARGET_FOLLOW)
  const [runTargetOverride, setRunTargetOverride] = useState(false)
  const [executeStep, setExecuteStep] = useState('params')
  const [runParamsModalOpen, setRunParamsModalOpen] = useState(false)
  const [nodeParamsText, setNodeParamsText] = useState('{}')
  const [nodeConfigDraft, setNodeConfigDraft] = useState(null)
  const [editorValidation, setEditorValidation] = useState(null)
  const [saveModalOpen, setSaveModalOpen] = useState(false)
  const [saveModalErrors, setSaveModalErrors] = useState([])
  const [saveModalErrorTitle, setSaveModalErrorTitle] = useState('保存前检查未通过')
  const saveModalErrorsRef = useRef(null)
  const [flowInstance, setFlowInstance] = useState(null)
  const canvasCardRef = useRef(null)
  const quickConnectRef = useRef(null)
  const [nodePicker, setNodePicker] = useState({open: false, mode: 'add', position: null, panelPosition: null, connection: null, insertEdge: null})
  const [nodePickerSearchText, setNodePickerSearchText] = useState('')
  const [pendingInsertEdge, setPendingInsertEdge] = useState(null)
  const [nodes, setNodes, onNodesChange] = useNodesState([])
  const [edges, setEdges, onEdgesChange] = useEdgesState([])
  const flowHistory = useFlowHistory(nodes, edges, setNodes, setEdges)
  const [canvasRunState, setCanvasRunState] = useState(() => emptyCanvasRunState())
  const [activeRun, setActiveRun] = useState(null)
  const [lastRun, setLastRun] = useState(null)
  const [rerunModal, setRerunModal] = useState(null)
  const [cancellingRunID, setCancellingRunID] = useState('')
  const [uploadNodeFiles, setUploadNodeFiles] = useState({})
  const [uploadProgress, setUploadProgress] = useState(null)
  const [canvasRunDetail, setCanvasRunDetail] = useState(null)
  const [canvasLogNodeID, setCanvasLogNodeID] = useState('')
  const [canvasLogMinimized, setCanvasLogMinimized] = useState(false)
  const [nodeCategoryFocus, setNodeCategoryFocus] = useState('')
  const canvasRunVersionRef = useRef(0)

  const clearCanvasRunOverlay = useCallback(() => {
    canvasRunVersionRef.current += 1
    setActiveRun(null)
    setLastRun(null)
    setRerunModal(null)
    setCancellingRunID('')
    setUploadProgress(null)
    setExecuteStep('params')
    setCanvasRunDetail(null)
    setCanvasLogNodeID('')
    setCanvasLogMinimized(false)
    setCanvasRunState(current => current.status === 'idle' ? current : emptyCanvasRunState())
  }, [])

  const handleNodesChange = useCallback(changes => {
    if (shouldClearCanvasRunOverlayForNodeChanges(changes)) {
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
    if (!selectedWorkflowID) return
    if (workflowOptions.some(item => item.id === selectedWorkflowID)) return
    setSelectedWorkflowID('')
    setNodes([])
    setEdges([])
    flowHistory.resetHistory([], [])
  }, [selectedWorkflowID, workflowOptions, setNodes, setEdges, flowHistory.resetHistory])

  const selectedNode = useMemo(() => nodes.find(node => node.id === selectedNodeID), [nodes, selectedNodeID])
  const selectedEdge = useMemo(() => edges.find(edge => edge.id === selectedEdgeID), [edges, selectedEdgeID])
  const configNode = nodeConfigDraft || selectedNode
  const toolMap = useMemo(() => new Map((catalog.tools || []).map(tool => [tool.id, tool])), [catalog.tools])
  const selectedTool = useMemo(() => (catalog.tools || []).find(tool => tool.id === configNode?.data.tool), [catalog.tools, configNode])
  const selectedLoopTool = useMemo(() => (catalog.tools || []).find(tool => tool.id === configNode?.data.loop?.tool), [catalog.tools, configNode])
  const nodePickerToolOptions = useMemo(() => filterEntries(toolOptions, nodePickerSearchText, ''), [toolOptions, nodePickerSearchText])
  const workflowOwnerOptions = useMemo(
    () => (catalog.plugins || []).filter(item => item.id && item.id !== 'user.workflows' && !item.disabled),
    [catalog.plugins]
  )
  const workflowOwnerCategories = useMemo(
    () => pluginCategories(catalog, workflowOwnerID),
    [catalog, workflowOwnerID]
  )
  const workflowOwnerCategory = !workflowOwnerLocked && workflowOwnerCategories.length === 1 ? workflowOwnerCategories[0] : null
  const workflowShareOptions = useMemo(
    () => workflowOwnerOptions.filter(item => item.id !== workflowOwnerID),
    [workflowOwnerOptions, workflowOwnerID]
  )
  const workflowCategoryLabels = useMemo(
    () => new Map((catalog.categories || []).map(item => [item.id, item.name || item.id])),
    [catalog.categories]
  )
  const workflowVisibleCategories = useMemo(() => {
    const map = new Map()
    for (const item of workflowOptions) {
      const categories = new Set([workflowCategoryKey(item)])
      for (const pluginID of item.shared_with || []) {
        for (const category of pluginCategories(catalog, pluginID)) categories.add(category.id)
      }
      map.set(item.id, categories)
    }
    return map
  }, [workflowOptions, catalog])
  const workflowCategoryGroups = useMemo(() => {
    const counts = new Map()
    for (const item of workflowOptions) {
      for (const categoryID of workflowVisibleCategories.get(item.id) || []) {
        counts.set(categoryID, (counts.get(categoryID) || 0) + 1)
      }
    }
    return [...counts.entries()]
      .map(([id, count]) => ({id, count, label: workflowCategoryLabel(id, workflowCategoryLabels)}))
      .sort((left, right) => {
        if (left.id === UNCATEGORIZED_WORKFLOW_CATEGORY) return 1
        if (right.id === UNCATEGORIZED_WORKFLOW_CATEGORY) return -1
        return left.label.localeCompare(right.label, 'zh-CN')
      })
  }, [workflowOptions, workflowVisibleCategories, workflowCategoryLabels])
  const filteredWorkflowOptions = useMemo(
    () => workflowCategoryFilter
      ? workflowOptions.filter(item => workflowVisibleCategories.get(item.id)?.has(workflowCategoryFilter))
      : workflowOptions,
    [workflowOptions, workflowCategoryFilter, workflowVisibleCategories]
  )
  const workflowOptionGroups = useMemo(() => {
    const byCategory = new Map()
    for (const item of filteredWorkflowOptions) {
      const keys = workflowCategoryFilter ? [workflowCategoryFilter] : [workflowCategoryKey(item)]
      for (const key of keys) {
        if (!byCategory.has(key)) byCategory.set(key, [])
        byCategory.get(key).push(item)
      }
    }
    return workflowCategoryGroups
      .filter(group => byCategory.has(group.id))
      .map(group => ({...group, items: byCategory.get(group.id)}))
  }, [filteredWorkflowOptions, workflowCategoryFilter, workflowCategoryGroups])
  const selectedWorkflowOutsideFilter = Boolean(workflowCategoryFilter && selectedWorkflowID)
    && !filteredWorkflowOptions.some(item => item.id === selectedWorkflowID)
    && workflowOptions.some(item => item.id === selectedWorkflowID)
  const workflowOwnerLabel = workflowOwnerLocked
    ? '内置工作流'
    : workflowOwnerID === 'user.workflows'
      ? '全局工作流'
      : (workflowOwnerOptions.find(item => item.id === workflowOwnerID)?.name || workflowOwnerID)
  const workflowCategoryText = workflowOwnerCategory
    ? (workflowOwnerCategory.name || workflowOwnerCategory.id)
    : workflowCategoryLabel(workflow.category, workflowCategoryLabels)
  const workflowParameters = useMemo(() => workflowModel.parseJSONList(workflowParamsText), [workflowParamsText])
  const parsedRunParams = useMemo(() => parseJSONObject(runParamsText), [runParamsText])
  const runParams = parsedRunParams.value
  const runHostOptions = useMemo(() => (hosts || []).filter(host => host && host.id), [hosts])
  const nodesWithOwnTarget = useMemo(() => nodes.filter(node => Boolean(node.data?.host)).length, [nodes])
  const runTargetLabel = useMemo(() => {
    if (runTarget === RUN_TARGET_FOLLOW) return '跟随节点配置'
    if (runTarget === RUN_TARGET_LOCAL) return '本机'
    const host = runHostOptions.find(item => item.id === runTarget)
    return host ? `${host.name || host.id}（${host.user}@${host.address}:${host.port}）` : runTarget
  }, [runTarget, runHostOptions])
  const runTargetAppliesTo = runTargetOverride ? nodes.length : Math.max(nodes.length - nodesWithOwnTarget, 0)
  const executableParamGroups = useMemo(() => buildExecutableParamGroups(nodes, toolMap), [nodes, toolMap])
  const executableParamCount = useMemo(() => executableParamGroups.reduce((total, group) => total + group.parameters.length, 0), [executableParamGroups])
  const mappingSources = useMemo(() => workflowModel.buildMappingSources(workflowParameters, selectedNodeID, nodes, edges, catalog.tools || []), [workflowParameters, selectedNodeID, nodes, edges, catalog.tools])
  const selectedNodeSources = mappingSources
  const isEditingCanvas = canvasMode === 'edit'
  const workflowConfirm = effectiveWorkflowConfirm(workflow, selectedWorkflowID, catalog.workflows || [])
  const canDeleteWorkflow = Boolean(selectedWorkflowID) && !workflowOwnerLocked
  const rerunnableRun = activeRun || lastRun
  const activeRunID = rerunnableRun?.id || ''
  const activeRunStatus = normalizeRunStatus(activeRun?.status || lastRun?.status || canvasRunState.status)
  const isActiveRunRunning = activeRunStatus === 'running'
  const canvasLogItem = useMemo(() => findRunLogItem(canvasRunDetail?.logs?.items || [], canvasLogNodeID), [canvasRunDetail, canvasLogNodeID])
  const displayNodes = useMemo(() => buildDisplayNodes(
    nodes,
    canvasRunState,
    catalog.tools || [],
    isEditingCanvas ? openDownstreamNodePicker : null,
    isEditingCanvas,
    isActiveRunRunning ? null : rerunWorkflowNode,
    activeRunID,
    hosts
  ), [nodes, canvasRunState, catalog.tools, isEditingCanvas, isActiveRunRunning, activeRunID, rerunWorkflowNode, hosts])
  const displayEdges = useMemo(() => buildDisplayEdges(edges, displayNodes, canvasRunState), [edges, displayNodes, canvasRunState])
  const categoryCounts = useMemo(() => countNodesByCategory(nodes), [nodes])
  const currentDraftSignature = useMemo(
    () => workflowDraftSignature(workflow, nodes, edges, '', workflowParamsText, workflowOwnerID),
    [workflow, nodes, edges, workflowParamsText, workflowOwnerID]
  )
  const hasUnsavedChanges = currentDraftSignature !== savedDraftSignature

  useEffect(() => {
    setWorkflowCategoryFilter(activeCategory || '')
  }, [activeCategory])

  useEffect(() => {
    if (selectedWorkflowID || nodes.length > 0 || isActiveRunRunning) return
    const rememberedID = readRememberedWorkflowID('')
    if (!rememberedID) return
    if (!workflowOptions.some(item => item.id === rememberedID)) {
      forgetRememberedWorkflowID('', rememberedID)
      return
    }
    loadWorkflow(rememberedID, {silent: true})
  }, [selectedWorkflowID, nodes.length, isActiveRunRunning, workflowOptions])

  useEffect(() => {
    if (!hasUnsavedChanges) return undefined
    function warnBeforeLeave(event) {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', warnBeforeLeave)
    return () => window.removeEventListener('beforeunload', warnBeforeLeave)
  }, [hasUnsavedChanges])

  useEffect(() => {
    if (!saveModalOpen || saveModalErrors.length === 0) return
    saveModalErrorsRef.current?.scrollIntoView?.({block: 'nearest'})
  }, [saveModalOpen, saveModalErrors])

  useEffect(() => {
    if (!selectedEdge) {
      setEdgeConfigModalOpen(false)
    }
  }, [selectedEdge])

  useEffect(() => {
    if (selectedNode) return
    setNodeConfigModalOpen(false)
    setNodeConfigDraft(null)
    setNodeParamsText('{}')
  }, [selectedNode])

  const closeNodeConfigModal = useCallback(() => {
    setNodeConfigModalOpen(false)
    setNodeConfigDraft(null)
  }, [])

  function openNodeConfigModal(nodeID) {
    if (!isEditingCanvas) {
      closeNodePicker()
      return
    }
    const node = nodes.find(item => item.id === nodeID)
    if (!node) return
    setSelectedNodeID(nodeID)
    setSelectedEdgeID('')
    setNodeConfigDraft(cloneNodeConfigDraft(node))
    setNodeParamsText(JSON.stringify(node.data.params || {}, null, 2))
    setNodeConfigModalOpen(true)
    setEdgeConfigModalOpen(false)
    closeNodePicker()
  }

  function handleCanvasNodeClick(nodeID) {
    if (isEditingCanvas) {
      openNodeConfigModal(nodeID)
      return
    }
    closeNodePicker()
    setCanvasLogNodeID(current => {
      if (current === nodeID) return ''
      setCanvasLogMinimized(false)
      return nodeID
    })
  }

  function openEdgeConfigModal(edgeID) {
    if (!isEditingCanvas) {
      closeNodePicker()
      return
    }
    setSelectedEdgeID(edgeID)
    setSelectedNodeID('')
    setNodeConfigModalOpen(false)
    setNodeConfigDraft(null)
    setEdgeConfigModalOpen(true)
    closeNodePicker()
  }

  async function rerunWorkflowNode(nodeID) {
    if (!activeRunID || isActiveRunRunning) {
      setResult({message: '当前没有可重跑的运行任务。'})
      return
    }
    const check = preflightWorkflowDraft('run')
    if (check.errors.length > 0) {
      showEditorValidation(check)
      return
    }
    const baseParams = parseJSONObject(runParamsText).value
    setRerunModal({nodeID, draft: check.draft, baseParams, confirmRequired: Boolean(workflowConfirm?.required || check.draft.confirm?.required)})
  }

  async function submitRerun(nodeID, params) {
    if (!activeRunID || isActiveRunRunning) {
      setResult({message: '当前没有可重跑的运行任务。'})
      return
    }
    const check = preflightWorkflowDraft('run')
    if (check.errors.length > 0) {
      showEditorValidation(check)
      return
    }
    const runVersion = canvasRunVersionRef.current + 1
    canvasRunVersionRef.current = runVersion
    const rerunState = buildRerunCanvasRunState(nodes, edges, canvasRunState, nodeID)
    const rerunTarget = rerunnableRun?.target || workflow.id
    enterExecuteMode()
    setRerunModal(null)
    setResult({message: `正在重跑节点 ${nodeID}...`, run: rerunnableRun || undefined})
    setCanvasRunState(rerunState)
    setActiveRun({id: activeRunID, status: 'running', target: rerunTarget})
    setLastRun({id: activeRunID, status: 'running', target: rerunTarget})
    try {
      const body = await postJSON(`/api/runs/${encodeURIComponent(activeRunID)}/nodes/${encodeURIComponent(nodeID)}/rerun`, {
        workflow: check.draft,
        params,
        confirm: Boolean(workflowConfirm?.required || check.draft.confirm?.required)
      })
      const detail = body?.data || body?.detail || body
      const status = normalizeRunStatus(detail?.record?.status || body?.status || 'running')
      const nextRun = {id: activeRunID, status, target: detail?.record?.target || rerunTarget}
      setActiveRun(nextRun)
      setLastRun(nextRun)
      if (detail?.record) {
        setCanvasRunDetail(detail)
        setCanvasRunState(buildCanvasRunStateFromDetail(detail, nodes))
        setResult(current => ({...current, run: nextRun, detail: {data: detail}}))
      }
      await pollRunDetail(activeRunID, {id: activeRunID, status, target: detail?.record?.target || workflow.id}, runVersion, nodes, state => setCanvasRunState(state), uploadNodeFiles)
    } catch (err) {
      const failedRun = {id: activeRunID, status: 'failed', target: rerunTarget}
      setActiveRun(failedRun)
      setLastRun(failedRun)
      setCanvasRunState(buildCanvasRunStateFromDetail(err.body?.data || err.body, nodes, readableAPIError(err, '节点重跑失败。')))
      setResult({message: readableAPIError(err, '节点重跑失败。'), response: err.body, run: failedRun})
    }
  }

  const selectedNodeKindLabel = configNode?.type === 'conditionNode'
    ? '条件分支'
    : configNode?.type === 'controlNode'
      ? controlNodeTitle(configNode.data.controlType)
      : '工具节点'

  const onConnect = useCallback(
    params => {
      if (!isEditingCanvas) return
      clearCanvasRunOverlay()
      if (quickConnectRef.current) quickConnectRef.current.completed = true
      setEdges(current => appendFlowEdge(current, params, nodes))
    },
    [clearCanvasRunOverlay, isEditingCanvas, nodes, setEdges]
  )

  async function loadWorkflow(id, options = {}) {
    if (!id) return
    if (!options.silent && id !== selectedWorkflowID && !confirmDiscardDraft()) return
    clearCanvasRunOverlay()
    setWorkflowAction('loading')
    if (!options.silent) setResult({message: '加载工作流...'})
    try {
      const body = await fetchJSON(`/api/workflows/${encodeURIComponent(id)}`)
      const config = body.data.Config || body.data.config
      const source = body.data.Source || body.data.source || {}
      const ownerID = source.plugin_id || 'user.workflows'
      const ownerLocked = Boolean(source.type) && source.type !== 'plugin'
      setSaveModalOpen(false)
      setSaveModalErrors([])
      setWorkflow(config)
      setWorkflowOwnerID(ownerID)
      setWorkflowOwnerLocked(ownerLocked)
      setWorkflowParamsText(JSON.stringify(config.parameters || [], null, 2))
      setRunParamsText(JSON.stringify(workflowModel.defaultParams(config.parameters || []), null, 2))
      const workflowNodes = config.nodes || []
      const legacyTargetMap = legacyLoopTargetMap(workflowNodes)
      const legacyTargets = new Set(legacyTargetMap.keys())
      const flowNodes = workflowNodes
        .filter(node => !legacyTargets.has(node.id))
        .map((node, index) => workflowNodeToFlowNode(node, index, removeNode, workflowNodes))
      const flowEdges = remapLegacyLoopEdges(config.edges || [], legacyTargetMap).map((edge, index) => flowEdgeFromWorkflowEdge(edge, index, flowNodes))
      const layoutNodes = workflowModel.autoLayoutNodes(flowNodes, flowEdges)
      setNodes(layoutNodes)
      setEdges(flowEdges)
      flowHistory.resetHistory(layoutNodes, flowEdges)
      setSavedDraftSignature(workflowDraftSignature(config, layoutNodes, flowEdges, '', JSON.stringify(config.parameters || []), ownerID))
      setSelectedWorkflowID(id)
      setSelectedNodeID('')
      setSelectedEdgeID('')
      setNodeConfigModalOpen(false)
      setNodeConfigDraft(null)
      setEdgeConfigModalOpen(false)
      setUploadNodeFiles({})
      closeNodePicker()
      fitCanvasViewAfterLayout()
      rememberWorkflowID('', id)
      if (!options.silent) setResult({message: `已加载工作流 ${id}`})
    } catch (err) {
      if (!options.silent) setResult({message: readableAPIError(err, '加载工作流失败。'), response: err.body})
    } finally {
      setWorkflowAction('')
    }
  }

  function changeWorkflowOwner(nextOwnerID) {
    setWorkflowOwnerID(nextOwnerID)
    const categories = pluginCategories(catalog, nextOwnerID)
    clearCanvasRunOverlay()
    setEditorValidation(null)
    setWorkflow(current => {
      const currentShared = current.shared_with || []
      const sharedWith = currentShared.filter(id => id !== nextOwnerID)
      const nextCategory = nextOwnerID === 'user.workflows'
        ? current.category
        : (categories.find(item => item.id === current.category)?.id || categories[0]?.id || 'global')
      if (nextCategory === current.category && sharedWith.length === currentShared.length) return current
      return {...current, category: nextCategory, shared_with: sharedWith}
    })
  }

  function toggleWorkflowSharedWith(pluginID) {
    if (!pluginID || pluginID === workflowOwnerID) return
    clearCanvasRunOverlay()
    setEditorValidation(null)
    setWorkflow(current => {
      const values = current.shared_with || []
      return {...current, shared_with: values.includes(pluginID) ? values.filter(id => id !== pluginID) : [...values, pluginID]}
    })
  }

  function createWorkflow() {
    if (!confirmDiscardDraft()) return
    clearCanvasRunOverlay()
    setSaveModalOpen(false)
    setSaveModalErrors([])
    const next = emptyWorkflow(workflowCategoryFilter || activeCategory || 'global')
    setCanvasMode('edit')
    setWorkflow(next)
    setWorkflowParamsText('[]')
    setRunParamsText('{}')
    setNodes([])
    setEdges([])
    flowHistory.resetHistory([], [])
    setSelectedWorkflowID('')
    setWorkflowOwnerID('user.workflows')
    setWorkflowOwnerLocked(false)
    setSelectedNodeID('')
    setSelectedEdgeID('')
    setNodeConfigModalOpen(false)
    setNodeConfigDraft(null)
    setEdgeConfigModalOpen(false)
    setEditorValidation(null)
    setSavedDraftSignature(workflowDraftSignature(next, [], [], '', '[]', 'user.workflows'))
    forgetRememberedWorkflowID('')
    closeNodePicker()
    setResult({message: '已创建空白工作流草稿'})
  }

  function confirmDiscardDraft() {
    if (!hasUnsavedChanges) return true
    return window.confirm('当前工作流有未保存的修改。继续后这些修改会丢失，是否继续？')
  }

  function resetRunParamsToDefaults() {
    clearCanvasRunOverlay()
    setRunParamsText(JSON.stringify(workflowModel.defaultParams(workflowParameters), null, 2))
    setEditorValidation(null)
    setResult({message: '已填入工作流默认运行参数。'})
  }

  function updateRunParam(name, value) {
    if (!name) return
    clearCanvasRunOverlay()
    const currentParams = parseJSONObject(runParamsText).value
    setRunParamsText(JSON.stringify({...currentParams, [name]: value}, null, 2))
    setEditorValidation(null)
  }

  function updateExecutableNodeParam(nodeID, name, value) {
    if (!nodeID || !name) return
    clearCanvasRunOverlay()
    setNodes(current => current.map(node => updateNodeExecutableParam(node, nodeID, name, value)))
    setEditorValidation(null)
  }

  function updateUploadNodeFiles(nodeID, mode, fileList) {
    const files = Array.from(fileList || []).filter(Boolean)
    setUploadNodeFiles(current => ({...current, [nodeID]: {mode, files}}))
    setEditorValidation(null)
  }

  function clearUploadNodeFiles(nodeID) {
    setUploadNodeFiles(current => {
      const next = {...current}
      delete next[nodeID]
      return next
    })
    setEditorValidation(null)
  }

  const removeNode = useCallback(id => {
    if (canvasMode !== 'edit') return
    clearCanvasRunOverlay()
    setNodes(current => current.filter(node => node.id !== id))
    setEdges(current => current.filter(edge => edge.source !== id && edge.target !== id))
    setResult({message: `已移除节点 ${id}`})
    setSelectedNodeID(current => current === id ? '' : current)
    setNodeConfigModalOpen(false)
    setNodeConfigDraft(null)
    setEdgeConfigModalOpen(false)
    setSelectedEdgeID('')
  }, [canvasMode, clearCanvasRunOverlay, setEdges, setNodes, setResult])

  function addToolNode(tool, position, options = {}) {
    if (!isEditingCanvas) return
    clearCanvasRunOverlay()
    const nodeID = uniqueNodeID(tool.id, nodes)
    const nextPosition = position || {x: 80 + nodes.length * 220, y: 120 + (nodes.length % 3) * 90}
    const nextNode = newToolFlowNode(tool, nodeID, nextPosition, removeNode)
    addNodeAndMaybeConnect(nextNode, options)
  }

  function addConditionNode(position, options = {}) {
    if (!isEditingCanvas) return
    clearCanvasRunOverlay()
    const nodeID = uniqueNodeID('condition', nodes)
    const nextPosition = position || {x: 80 + nodes.length * 220, y: 120 + (nodes.length % 3) * 90}
    const nextNode = newConditionFlowNode(nodeID, nextPosition, removeNode)
    addNodeAndMaybeConnect(nextNode, options)
  }

  function addControlNode(controlType, position, options = {}) {
    if (!isEditingCanvas) return
    if (controlType === 'condition') {
      addConditionNode(position, options)
      return
    }
    clearCanvasRunOverlay()
    const control = controlNodeCatalog.find(item => item.type === controlType && isControlReady(item))
    if (!control) return
    const nodeID = uniqueNodeID(controlType, nodes)
    const nextPosition = position || {x: 80 + nodes.length * 220, y: 120 + (nodes.length % 3) * 90}
    const nextNode = newControlFlowNode(control, nodeID, nextPosition, removeNode)
    addNodeAndMaybeConnect(nextNode, options)
  }

  function addNodeAndMaybeConnect(nextNode, options = {}) {
    const pendingConnection = options.connection || null
    const insertEdge = options.insertEdge || null
    if (insertEdge && !canInsertIntoEdge(insertEdge, edges, nodes)) {
      setResult({message: '待插入的连线已不存在，请重新选择连线。'})
      closeNodePicker()
      return
    }
    if (pendingConnection?.source && !nodes.some(node => node.id === pendingConnection.source)) {
      setResult({message: '起始节点已不存在，未创建下游节点。'})
      closeNodePicker()
      return
    }
    setNodes(current => [...current, nextNode])
    if (insertEdge) {
      const sourceNodes = [...nodes, nextNode]
      setEdges(current => insertNodeIntoEdge(current, insertEdge, nextNode, sourceNodes))
      setResult({message: `已在连线中插入节点 ${nextNode.id}。`})
    } else if (pendingConnection?.source) {
      const sourceNodes = [...nodes, nextNode]
      setEdges(current => appendFlowEdge(current, {
        source: pendingConnection.source,
        sourceHandle: pendingConnection.sourceHandle || undefined,
        target: nextNode.id,
        targetHandle: undefined
      }, sourceNodes))
      setResult({message: `已添加下游节点 ${nextNode.id} 并建立依赖。`})
    }
    setSelectedNodeID(nextNode.id)
    setSelectedEdgeID('')
    setEditorValidation(null)
    closeNodePicker()
    setPendingInsertEdge(null)
    setNodeConfigModalOpen(false)
    setNodeConfigDraft(null)
    setEdgeConfigModalOpen(false)
    setUploadNodeFiles({})
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

  function openNodePicker(position, options = {}) {
    if (!isEditingCanvas) return
    setSelectedNodeID('')
    setSelectedEdgeID('')
    setNodeConfigModalOpen(false)
    setNodeConfigDraft(null)
    setEdgeConfigModalOpen(false)
    const insertEdge = options.insertEdge || null
    setPendingInsertEdge(insertEdge)
    setNodePicker({
      open: true,
      mode: options.mode || (insertEdge ? 'insert' : options.connection?.source ? 'connect' : 'add'),
      position: position || defaultCanvasInsertPosition(),
      panelPosition: options.panelPosition || null,
      connection: options.connection || null,
      insertEdge
    })
    setNodePickerSearchText('')
  }

  function openNodePickerFromEvent(event) {
    event.stopPropagation()
    const point = eventPoint(event)
    const position = point && flowInstance
      ? flowInstance.screenToFlowPosition(point)
      : defaultCanvasInsertPosition()
    openNodePicker(position, {panelPosition: panelPositionFromPoint(point, canvasCardRef.current)})
  }

  function closeNodePicker() {
    setNodePicker({open: false, mode: 'add', position: null, panelPosition: null, connection: null, insertEdge: null})
    setPendingInsertEdge(null)
  }

  function handleConnectStart(_, params) {
    if (!isEditingCanvas) {
      quickConnectRef.current = null
      return
    }
    if (params?.handleType !== 'source' || !params?.nodeId) {
      quickConnectRef.current = null
      return
    }
    quickConnectRef.current = {
      source: params.nodeId,
      sourceHandle: params.handleId || '',
      completed: false
    }
  }

  function handleConnectEnd(event) {
    if (!isEditingCanvas) {
      quickConnectRef.current = null
      return
    }
    const pending = quickConnectRef.current
    quickConnectRef.current = null
    if (!pending?.source || pending.completed) return
    const point = eventPoint(event)
    const position = point && flowInstance
      ? flowInstance.screenToFlowPosition(point)
      : defaultCanvasInsertPosition()
    openNodePicker(position, {
      panelPosition: panelPositionFromPoint(point, canvasCardRef.current),
      connection: {source: pending.source, sourceHandle: pending.sourceHandle || ''}
    })
  }

  function openDownstreamNodePicker(nodeID, event) {
    event?.stopPropagation()
    if (!isEditingCanvas) return
    const sourceNode = nodes.find(node => node.id === nodeID)
    if (!sourceNode || sourceNode.type === 'conditionNode') return
    const position = downstreamInsertPosition(sourceNode)
    const panelPosition = position && flowInstance
      ? panelPositionFromFlowPosition(position, flowInstance, canvasCardRef.current)
      : null
    openNodePicker(position, {
      mode: 'connect',
      panelPosition,
      connection: {source: nodeID, sourceHandle: ''}
    })
  }

  function handlePaneDoubleClick(event) {
    if (!isEditingCanvas) return
    if (!isPaneDoubleClick(event)) return
    const point = eventPoint(event)
    const position = point && flowInstance
      ? flowInstance.screenToFlowPosition(point)
      : defaultCanvasInsertPosition()
    openNodePicker(position, {
      mode: 'add',
      panelPosition: panelPositionFromPoint(point, canvasCardRef.current)
    })
  }

  function handleEdgeDoubleClick(event, edge) {
    event.stopPropagation()
    if (!isEditingCanvas) return
    const point = eventPoint(event)
    const position = point && flowInstance
      ? flowInstance.screenToFlowPosition(point)
      : edgeMidpoint(edge, nodes)
    openNodePicker(position, {
      mode: 'insert',
      panelPosition: panelPositionFromPoint(point, canvasCardRef.current),
      insertEdge: stripRuntimeEdgeState(edge)
    })
  }

  const onReconnect = useCallback(
    (oldEdge, newConnection) => {
      if (!isEditingCanvas) return
      clearCanvasRunOverlay()
      setEdges(current => normalizeFlowEdges(reconnectEdge(oldEdge, newConnection, current), nodes))
      setEditorValidation(null)
    },
    [clearCanvasRunOverlay, isEditingCanvas, nodes, setEdges]
  )

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
    if (!isEditingCanvas) return
    setNodes(current => workflowModel.autoLayoutNodes(current, edges))
    setResult({message: nodes.length === 0 ? '当前画布没有节点，无需优化排版。' : '已优化工作流排版。'})
    fitCanvasViewAfterLayout()
  }

  function updateDraftNodeData(patch) {
    setNodeConfigDraft(current => current ? {...current, data: {...current.data, ...patch}} : current)
    setEditorValidation(null)
  }

  function updateSelectedNodeName(nextName) {
    updateDraftNodeData({name: nextName})
  }

  function updateSelectedNodeHost(nextHost) {
    updateDraftNodeData({host: nextHost})
  }

  function updateSelectedNodeCondition(nextCondition) {
    updateDraftNodeData({condition: nextCondition})
  }

  function updateSelectedNodeLoop(nextLoop) {
    updateDraftNodeData({loop: normalizeLoopConfig(nextLoop)})
  }

  function updateSelectedNodeUpload(nextUpload) {
    updateDraftNodeData({upload: workflowModel.normalizeUploadConfig(nextUpload)})
  }

  function updateSelectedNodeExtractConfig(nextExtractConfig) {
    updateDraftNodeData({extract_config: workflowModel.normalizeExtractConfig(nextExtractConfig)})
  }

  function updateSelectedNodeHttpRequest(nextHttpRequest) {
    updateDraftNodeData({http: workflowModel.normalizeHttpRequestConfig(nextHttpRequest)})
  }

  function updateSelectedLoopParam(name, value) {
    const loop = normalizeLoopConfig(configNode?.data.loop || defaultLoop())
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
    if (!isEditingCanvas) return
    event.dataTransfer.setData('application/ops-tool', tool.id)
    event.dataTransfer.effectAllowed = 'move'
  }

  function handleControlDragStart(event, control) {
    if (!isEditingCanvas) return
    event.dataTransfer.setData('application/ops-control', control.type)
    event.dataTransfer.effectAllowed = 'move'
  }

  function handleCanvasDragOver(event) {
    if (!isEditingCanvas) return
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
  }

  function handleCanvasDrop(event) {
    if (!isEditingCanvas) return
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
    if (!isEditingCanvas) return
    if (!selectedNodeID) return
    removeNode(selectedNodeID)
  }

  function removeSelectedEdge() {
    if (!isEditingCanvas) return
    if (!selectedEdgeID) return
    clearCanvasRunOverlay()
    setEdges(current => current.filter(edge => edge.id !== selectedEdgeID))
    setResult({message: `已移除依赖 ${selectedEdgeID}`})
    setSelectedEdgeID('')
    setEdgeConfigModalOpen(false)
  }

  function clearSelection() {
    setNodeConfigModalOpen(false)
    setNodeConfigDraft(null)
    setEdgeConfigModalOpen(false)
    setSelectedNodeID('')
    setSelectedEdgeID('')
  }

  function enterExecuteMode() {
    setCanvasMode('execute')
    closeNodePicker()
    setNodeConfigModalOpen(false)
    setNodeConfigDraft(null)
    setEdgeConfigModalOpen(false)
    setSelectedNodeID('')
    setSelectedEdgeID('')
  }

  const undoFlowEdit = useCallback(() => {
    if (!isEditingCanvas) return
    if (!flowHistory.undo()) {
      setEditorNotice('没有可撤销的编辑操作。')
      return
    }
    setSelectedNodeID('')
    setSelectedEdgeID('')
    setNodeConfigModalOpen(false)
    setNodeConfigDraft(null)
    setEdgeConfigModalOpen(false)
    setEditorValidation(null)
    setEditorNotice('已撤销上一步画布编辑。')
  }, [isEditingCanvas, flowHistory.undo])

  const redoFlowEdit = useCallback(() => {
    if (!isEditingCanvas) return
    if (!flowHistory.redo()) {
      setEditorNotice('没有可重做的编辑操作。')
      return
    }
    setSelectedNodeID('')
    setSelectedEdgeID('')
    setNodeConfigModalOpen(false)
    setNodeConfigDraft(null)
    setEdgeConfigModalOpen(false)
    setEditorValidation(null)
    setEditorNotice('已重做上一步画布编辑。')
  }, [isEditingCanvas, flowHistory.redo])

  useEffect(() => {
    function handleHistoryShortcut(event) {
      if (!(event.ctrlKey || event.metaKey)) return
      const key = String(event.key || '').toLowerCase()
      if (key === 's' && isEditingCanvas) {
        event.preventDefault()
        openSaveModal()
        return
      }
      const target = event.target
      const tag = target?.tagName
      if (!isEditingCanvas || tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target?.isContentEditable) return
      if (key === 'z' && !event.shiftKey) {
        event.preventDefault()
        undoFlowEdit()
      } else if (key === 'y' || (key === 'z' && event.shiftKey)) {
        event.preventDefault()
        redoFlowEdit()
      }
    }
    window.addEventListener('keydown', handleHistoryShortcut)
    return () => window.removeEventListener('keydown', handleHistoryShortcut)
  }, [isEditingCanvas, undoFlowEdit, redoFlowEdit, workflowAction])

  function parseNodeParamsText() {
    try {
      const parsed = JSON.parse(nodeParamsText || '{}')
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        setResult({message: '参数 JSON 必须是对象。'})
        return null
      }
      return parsed
    } catch (err) {
      setResult({message: `参数（JSON）无效：${err.message}`})
      return null
    }
  }

  function applyNodeParams() {
    if (!configNode) return false
    const params = parseNodeParamsText()
    if (params === null) return false
    updateSelectedNodeParams(params)
    setResult({message: `已应用节点 ${configNode.id} 的参数草稿，保存配置后生效。`})
    return true
  }

  function updateSelectedNodeParams(nextParams) {
    setNodeConfigDraft(current => current ? {...current, data: {...current.data, params: nextParams}} : current)
    setNodeParamsText(JSON.stringify(nextParams, null, 2))
    setEditorValidation(null)
  }

  function updateMappedParam(name, value) {
    const nextParams = {...(configNode?.data.params || {}), [name]: value}
    updateSelectedNodeParams(nextParams)
  }

  function saveNodeConfig() {
    if (!configNode || !selectedNodeID) {
      closeNodeConfigModal()
      return false
    }
    const data = configNode.data
    let nextParams = data.params
    if (configNode.type === 'toolNode') {
      const parsed = parseNodeParamsText()
      if (parsed === null) return false
      nextParams = parsed
    }
    clearCanvasRunOverlay()
    setNodes(current => current.map(node => {
      if (node.id !== selectedNodeID) return node
      const nextData = {...node.data, name: data.name}
      if (node.type === 'toolNode') {
        nextData.host = data.host || ''
        nextData.params = nextParams || {}
      } else if (node.type === 'conditionNode') {
        nextData.condition = data.condition
      } else if (node.type === 'controlNode') {
        if (data.controlType === 'loop') {
          nextData.host = data.host || ''
          nextData.loop = normalizeLoopConfig(data.loop || defaultLoop())
        }
        if (data.controlType === 'upload') nextData.upload = workflowModel.normalizeUploadConfig(data.upload)
        if (data.controlType === 'extract_config') nextData.extract_config = workflowModel.normalizeExtractConfig(data.extract_config)
        if (data.controlType === 'http_request') nextData.http = workflowModel.normalizeHttpRequestConfig(data.http)
      }
      return {...node, data: nextData}
    }))
    if (configNode.type === 'conditionNode') syncConditionEdgeLabels(selectedNodeID, data.condition)
    setEditorValidation(null)
    closeNodeConfigModal()
    setResult({message: `已保存节点 ${selectedNodeID} 的配置。`})
    return true
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
    const draft = workflowModel.buildWorkflowDraft(workflow, nodes, edges, '', workflowParameters)
    if (!workflowOwnerLocked && workflowOwnerID !== 'user.workflows') {
      const ownerCategories = pluginCategories(catalog, workflowOwnerID)
      draft.category = ownerCategories.find(item => item.id === draft.category)?.id || ownerCategories[0]?.id || 'global'
    }
    const sharedWith = (draft.shared_with || []).filter(id => id && id !== workflowOwnerID && id !== 'user.workflows')
    draft.shared_with = sharedWith.length > 0 ? sharedWith : undefined
    workflowModel.validateConditionDraft(nodes, edges).forEach(error => errors.push(error))
    workflowModel.validateControlDraft(nodes, edges, catalog.tools || []).forEach(error => errors.push(error))
    if (nodes.length > 1 && edges.length === 0 && draft.edges.length === 0) {
      errors.push('多个节点没有连线，无法确定执行顺序。请为条件、并行、合流等控制节点显式连接依赖关系。')
    }
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
    workflowModel.findMissingRequiredNodeParams(nodes, catalog.tools || []).forEach(item => {
      errors.push(`节点 ${item.nodeID}（${item.toolName}）缺少必填参数：${item.paramName}`)
    })
    if (mode === 'run') {
      uploadFlowNodes(nodes).forEach(node => {
        const files = uploadFilesForNode(uploadNodeFiles, node.id)
        const totalSize = uploadFilesTotalSize(files)
        if (files.length === 0) errors.push(`上传节点 ${node.id} 请选择要上传的文件或目录。`)
        if (totalSize > MAX_PLATFORM_UPLOAD_BYTES) errors.push(`上传节点 ${node.id} 文件总大小 ${formatBytes(totalSize)} 超过平台上限 ${formatBytes(MAX_PLATFORM_UPLOAD_BYTES)}。`)
      })
    }
    const title = mode === 'save' ? '保存前检查未通过' : mode === 'run' ? '执行前检查未通过' : '校验前检查未通过'
    return {draft, errors, warnings: [], title}
  }

  function preflightRunParamsStep() {
    const check = preflightWorkflowDraft('params')
    const errors = [...check.errors]
    let runParams = {}
    try {
      const parsed = JSON.parse(runParamsText || '{}')
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        errors.push('执行参数必须是 JSON 对象。')
      } else {
        runParams = parsed
      }
    } catch (err) {
      errors.push(`执行参数 JSON 无效：${err.message}`)
    }
    workflowModel.findMissingRequiredWorkflowParams(check.draft.parameters || [], runParams).forEach(item => {
      errors.push(`工作流参数 ${item.name} 为必填，请先填写。`)
    })
    workflowModel.findMissingRequiredNodeParams(nodes, catalog.tools || []).forEach(item => {
      errors.push(`节点 ${item.nodeID}（${item.toolName}）缺少必填参数：${item.paramName}`)
    })
    return {...check, title: '参数检查未通过', errors}
  }

  async function validateDraft() {
    const check = preflightWorkflowDraft('validate')
    if (check.errors.length > 0) {
      showEditorValidation(check)
      return
    }
    try {
      setWorkflowAction('validating')
      setEditorValidation(null)
      setWorkflow(check.draft)
      const body = await postJSON(`/api/workflows/${check.draft.id || 'draft'}/validate`, {workflow: check.draft})
      setResult({message: summarizeAPIResponse(body, '工作流校验通过。'), response: body})
    } catch (err) {
      setResult({message: readableAPIError(err, '工作流校验失败。'), response: err.body})
    } finally {
      setWorkflowAction('')
    }
  }

  function openSaveModal() {
    if (workflowAction) return
    setSaveModalErrorTitle('保存前检查未通过')
    setSaveModalErrors([])
    setSaveModalOpen(true)
  }

  async function saveDraft() {
    const check = preflightWorkflowDraft('save')
    if (check.errors.length > 0) {
      setSaveModalErrorTitle('保存前检查未通过')
      setSaveModalErrors(check.errors)
      return
    }
    setSaveModalErrors([])
    try {
      setWorkflowAction('saving')
      setEditorValidation(null)
      setWorkflow(check.draft)
      const payload = {workflow: check.draft}
      if (!workflowOwnerLocked) payload.owner_plugin_id = workflowOwnerID
      const body = await postJSON(`/api/workflows/${check.draft.id}/save`, payload)
      const source = body.data?.Source || body.data?.source
      if (source) {
        setWorkflowOwnerID(source.plugin_id || 'user.workflows')
        setWorkflowOwnerLocked(Boolean(source.type) && source.type !== 'plugin')
      }
      setSelectedWorkflowID(check.draft.id)
      setSavedDraftSignature(workflowDraftSignature(check.draft, nodes, edges, '', workflowParamsText, source?.plugin_id || workflowOwnerID))
      rememberWorkflowID('', check.draft.id)
      setSaveModalOpen(false)
      setResult({message: summarizeAPIResponse(body, '工作流保存成功。'), response: body})
      refreshCatalog({keepCategory: true}).catch(() => {})
    } catch (err) {
      const message = readableAPIError(err, '工作流保存失败。')
      setSaveModalErrorTitle('工作流保存失败')
      setSaveModalErrors([message])
      setResult({message, response: err.body})
    } finally {
      setWorkflowAction('')
    }
  }

  async function deleteWorkflow() {
    const id = selectedWorkflowID
    if (!id) return
    const label = workflow.name || id
    if (!window.confirm(`确认删除工作流「${label}」？\n\n该操作会删除归属插件目录中的工作流配置，且不可撤销。`)) return
    try {
      setWorkflowAction('deleting')
      const body = await deleteJSON(`/api/workflows/${encodeURIComponent(id)}`)
      clearCanvasRunOverlay()
      const next = emptyWorkflow()
      setCanvasMode('edit')
      setWorkflow(next)
      setWorkflowParamsText('[]')
      setRunParamsText('{}')
      setNodes([])
      setEdges([])
      flowHistory.resetHistory([], [])
      setSelectedWorkflowID('')
      setWorkflowOwnerID('user.workflows')
      setWorkflowOwnerLocked(false)
      forgetRememberedWorkflowID('', id)
      setSelectedNodeID('')
      setSelectedEdgeID('')
      setNodeConfigModalOpen(false)
      setNodeConfigDraft(null)
      setEdgeConfigModalOpen(false)
      setEditorValidation(null)
      setSavedDraftSignature(workflowDraftSignature(next, [], [], '', '[]', 'user.workflows'))
      setUploadNodeFiles({})
      closeNodePicker()
      await refreshCatalog({keepCategory: true})
      setResult({message: summarizeAPIResponse(body, '工作流已删除。'), response: body})
    } catch (err) {
      setResult({message: readableAPIError(err, '工作流删除失败。'), response: err.body})
    } finally {
      setWorkflowAction('')
    }
  }

  async function runDraft() {
    if (isActiveRunRunning) {
      setResult({message: `当前工作流正在运行：${activeRunID || '等待运行记录 ID'}，请先等待完成或取消运行。`, run: activeRun || undefined})
      return
    }
    enterExecuteMode()
    const check = preflightWorkflowDraft('run')
    if (check.errors.length > 0) {
      clearCanvasRunOverlay()
      showEditorValidation(check)
      return
    }
    const confirmRequired = Boolean(workflowConfirm?.required || check.draft.confirm?.required)
    if (confirmRequired && !window.confirm(workflowConfirm?.message || check.draft.confirm?.message || '该工作流需要确认后执行，是否继续？')) {
      setResult({message: '已取消执行工作流。'})
      return
    }
    const runVersion = canvasRunVersionRef.current + 1
    canvasRunVersionRef.current = runVersion
    setUploadProgress(null)
    const runNodesSnapshot = nodes
    setCanvasRunState(buildRunningCanvasRunState(runNodesSnapshot, check.draft.edges))
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
      const uploadNodeFilesSnapshot = uploadNodeFiles
      setResult({message: '执行工作流...', uploadProgress: null})
      setExecuteDetailView('logs')
      const body = await postJSON(`/api/workflows/${check.draft.id}/run?async=true`, {
        params: runParams,
        workflow: check.draft,
        confirm: confirmRequired,
        target: runTarget === RUN_TARGET_FOLLOW ? undefined : runTarget,
        override_nodes: runTarget === RUN_TARGET_FOLLOW ? false : runTargetOverride
      })
      setRunParamsModalOpen(false)
      if (body.id) {
        setActiveRun({id: body.id, status: body.status || 'running', target: body.target || check.draft.id})
        await pollRunDetail(body.id, body, runVersion, runNodesSnapshot, applyRunOverlay, uploadNodeFilesSnapshot)
        return
      }
      setActiveRun(null)
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
        setCanvasRunDetail(detail)
        applyRunOverlay(buildCanvasRunStateFromDetail(detail, runNodesSnapshot, readableAPIError(err, '工作流执行失败。')))
      } else {
        applyRunOverlay(buildFailedCanvasRunState(runNodesSnapshot, readableAPIError(err, '工作流执行失败。')))
      }
      setActiveRun(runID ? {id: runID, status: body.status || 'failed'} : null)
      setResult(current => ({...current, message: readableAPIError(err, '工作流执行失败。'), response: body, detail: detail ? {data: detail} : undefined, run: runID ? {id: runID, status: body.status || 'failed'} : undefined}))
    }
  }

  function prepareRunStep() {
    if (isActiveRunRunning) {
      setResult({message: `当前工作流正在运行：${activeRunID || '等待运行记录 ID'}，请先等待完成或取消运行。`, run: activeRun || undefined})
      return
    }
    enterExecuteMode()
    const check = preflightRunParamsStep()
    if (check.errors.length > 0) {
      clearCanvasRunOverlay()
      showEditorValidation(check)
      return
    }
    setEditorValidation(null)
    setExecuteStep('target')
    setResult({message: '参数已确认，请选择执行机器。'})
  }

  function prepareRunConfirmStep() {
    if (isActiveRunRunning) return
    const check = preflightRunParamsStep()
    if (check.errors.length > 0) {
      clearCanvasRunOverlay()
      showEditorValidation(check)
      return
    }
    setEditorValidation(null)
    setExecuteStep('run')
    setResult({message: `执行机器已确认：${runTargetLabel}。`})
  }

  function openRunParamsModal() {
    if (isActiveRunRunning) {
      setResult({message: `当前工作流正在运行：${activeRunID || '等待运行记录 ID'}，请先等待完成或取消运行。`, run: activeRun || undefined})
      return
    }
    enterExecuteMode()
    setExecuteStep('params')
    setRunParamsModalOpen(true)
  }

  function runWorkflowDockAction() {
    openRunParamsModal()
  }

  async function cancelActiveRun() {
    if (!activeRunID || cancellingRunID) return
    setCancellingRunID(activeRunID)
    setResult({message: `正在取消运行任务 ${activeRunID}...`, run: activeRun})
    try {
      const body = await postJSON(`/api/runs/${encodeURIComponent(activeRunID)}/cancel`, {})
      setResult({message: `已发送取消请求：${activeRunID}`, response: body, run: activeRun})
    } catch (err) {
      setResult({message: readableAPIError(err, '取消运行任务失败。'), response: err.body, run: activeRun})
    } finally {
      setCancellingRunID('')
    }
  }

  async function pollRunDetail(runID, initialRun, runVersion, runNodesSnapshot, applyRunOverlay, uploadNodeFilesSnapshot = {}) {
    const maxMissingAttempts = 8
    let missingAttempts = 0
    const submittedUploadNodes = new Set()
    while (canvasRunVersionRef.current === runVersion) {
      try {
        const detail = await fetchRunDetail(runID)
        const detailData = detail?.data || detail
        if (detailData?.record) {
          const status = normalizeRunStatus(detailData.record.status || initialRun?.status || 'running')
          const nextRun = {id: runID, status, target: detailData.record.target || initialRun?.target}
          setCanvasRunDetail(detailData)
          applyRunOverlay(buildCanvasRunStateFromDetail(detailData, runNodesSnapshot))
          setActiveRun(nextRun)
          setLastRun(nextRun)
          setResult(current => ({...current, run: {...initialRun, ...nextRun}, detail: {data: detailData}}))
          if (status === 'running') {
            await submitWaitingUploadNodes(runID, detailData, runNodesSnapshot, uploadNodeFilesSnapshot, submittedUploadNodes, setResult, setUploadProgress)
          }
          if (status !== 'running') return detail
        }
      } catch (err) {
        missingAttempts += 1
        if (missingAttempts >= maxMissingAttempts) throw err
      }
      await sleep(1000)
    }
    return null
  }

  const quickToolOptions = toolOptions.slice(0, 4)
  const draftCheck = preflightWorkflowDraft(isEditingCanvas ? 'save' : 'run')
  const draftStatus = draftCheck.errors.length > 0
    ? {kind: 'warning', title: `${draftCheck.errors.length} 项待完善`, detail: draftCheck.errors[0]}
    : hasUnsavedChanges
      ? {kind: 'changed', title: '有未保存修改', detail: '结构检查已通过，保存后可供其他用户加载。'}
      : {kind: 'ready', title: selectedWorkflowID ? '工作流已保存' : '草稿已就绪', detail: '结构检查已通过，可以保存或进入执行模式。'}
  const modeSwitch = (
    <div className="workflowModeSwitch" role="group" aria-label="工作流画布模式">
      <button className={isEditingCanvas ? 'primary' : 'secondary'} type="button" onClick={() => setCanvasMode('edit')} disabled={isActiveRunRunning} title={isActiveRunRunning ? '运行期间不能修改工作流' : undefined}>编辑模式</button>
      <button className={!isEditingCanvas ? 'primary' : 'secondary'} type="button" onClick={enterExecuteMode}>执行模式</button>
    </div>
  )
  function renderWorkflowOption(item) {
    const owner = item.source?.plugin_id
    const ownerLabel = owner && owner !== 'user.workflows' ? ` · ${item.source.plugin_name || owner}` : ''
    const sharedOnly = Boolean(workflowCategoryFilter) && workflowCategoryKey(item) !== workflowCategoryFilter
    const sharedLabel = sharedOnly ? ` · 共享自 ${item.source?.plugin_name || item.source?.plugin_id || '其它插件'}` : ''
    return <option key={item.id} value={item.id}>{item.name || item.id}{ownerLabel}{sharedLabel}</option>
  }
  const workflowLoader = (
    <section className="workflowSelection" aria-labelledby="workflow-selection-title">
      <div className="workflowSelectionHeader">
        <div>
          <strong id="workflow-selection-title">打开已有工作流</strong>
          <span>{workflowOptions.length === 0
            ? '还没有已保存工作流'
            : workflowCategoryFilter
              ? `当前分类 ${workflowCategoryLabel(workflowCategoryFilter, workflowCategoryLabels)}：${filteredWorkflowOptions.length} / 共 ${workflowOptions.length} 个工作流`
              : `共 ${workflowOptions.length} 个工作流可加载`}</span>
        </div>
        <span className={selectedWorkflowID ? 'workflowSelectionState active' : 'workflowSelectionState'}>
          {selectedWorkflowID ? '已加载' : '未选择'}
        </span>
      </div>
      <label className="workflowSelectionField">
        <span>工作流分类</span>
        <select
          aria-label="按分类筛选工作流"
          value={workflowCategoryFilter}
          disabled={Boolean(workflowAction) || isActiveRunRunning || workflowCategoryGroups.length === 0}
          onChange={event => setWorkflowCategoryFilter(event.target.value)}
        >
          <option value="">全部分类（{workflowOptions.length}）</option>
          {workflowCategoryGroups.map(group => (
            <option key={group.id} value={group.id}>{group.label}（{group.count}）</option>
          ))}
        </select>
      </label>
      <label className="workflowSelectionField">
        <span>选择要加载的工作流</span>
        <select
          aria-label="选择要加载的工作流"
          value={selectedWorkflowID}
          disabled={Boolean(workflowAction) || isActiveRunRunning || (filteredWorkflowOptions.length === 0 && !selectedWorkflowOutsideFilter)}
          onChange={event => loadWorkflow(event.target.value)}
        >
          <option value="">
            {workflowAction === 'loading'
              ? '正在加载...'
              : workflowOptions.length === 0
                ? '暂无可加载工作流'
                : filteredWorkflowOptions.length === 0 && !selectedWorkflowOutsideFilter
                  ? '当前分类暂无工作流'
                  : '请选择一个工作流'}
          </option>
          {selectedWorkflowOutsideFilter && (
            <optgroup label="当前已加载（不在所选分类）">
              {renderWorkflowOption(workflowOptions.find(item => item.id === selectedWorkflowID))}
            </optgroup>
          )}
          {workflowOptionGroups.map(group => (
            <optgroup key={group.id} label={group.label}>
              {group.items.map(renderWorkflowOption)}
            </optgroup>
          ))}
        </select>
      </label>
      <small className="workflowSelectionHint">
        {selectedWorkflowID
          ? '已加载到当前画布，可继续编辑或执行。'
          : workflowOptions.length > 0 && filteredWorkflowOptions.length === 0
            ? '当前分类暂无工作流，可切换分类或查看全部分类。'
            : '没有合适的工作流？点击下方“新建”开始。'}
      </small>
    </section>
  )
  const addControlNodeGrid = groupControlsByCategory(controlNodeCatalog).map(({category, controls}) => (
    <div key={category.id} className={`canvasHoverToolbarSection cat-${category.id}`} title={category.description}>
      <span>
        <i className={`catDot ${category.id}`} aria-hidden="true" />
        {category.label}
      </span>
      <div className="canvasHoverToolbarGrid">
        {controls.map(control => {
          const disabled = !isControlReady(control)
          return (
            <button
              key={control.type}
              type="button"
              className={disabled ? 'planned' : undefined}
              draggable={!disabled}
              disabled={disabled}
              aria-disabled={disabled}
              onDragStart={disabled ? undefined : event => handleControlDragStart(event, control)}
              onClick={disabled ? undefined : () => addControlNode(control.type)}
              title={`${control.title}\n${control.secondary}\n${control.help || control.description}${disabled ? '\n规划中' : ''}`}
            >
              <span className={`paletteShape ${control.shape} cat-${control.category}`} data-symbol={controlShapeMarker(control.type)} aria-hidden="true" />
              <span>{control.title}</span>
            </button>
          )
        })}
      </div>
    </div>
  ))
  const addToolNodeGrid = (
    <div className="canvasHoverToolbarGrid tools">
      {quickToolOptions.map(tool => (
        <button key={tool.id} type="button" draggable onDragStart={event => handleToolDragStart(event, tool)} onClick={() => addToolNode(tool)} title={`${tool.name || tool.id}\n${tool.id}${tool.description ? `\n${tool.description}` : ''}`}>
          <span className="paletteShape tool" aria-hidden="true" />
          <span>{tool.name || tool.id}</span>
        </button>
      ))}
      {quickToolOptions.length === 0 && <button type="button" onClick={() => openNodePicker()}>搜索插件工具</button>}
    </div>
  )
  const canvasCategoryLegend = isEditingCanvas && nodes.length > 0 ? (
    <div className="canvasCategoryLegend nodrag nopan" onMouseDown={event => event.stopPropagation()} role="group" aria-label="按节点分类高亮">
      <button
        type="button"
        className={nodeCategoryFocus ? '' : 'active'}
        aria-pressed={!nodeCategoryFocus}
        title="显示全部节点"
        onClick={() => setNodeCategoryFocus('')}
      >
        全部<span className="categoryLegendCount">{nodes.length}</span>
      </button>
      {allNodeCategories.map(category => {
        const count = categoryCounts[category.id] || 0
        const active = nodeCategoryFocus === category.id
        return (
          <button
            key={category.id}
            type="button"
            className={`${active ? 'active' : ''}${count === 0 ? ' empty' : ''}`.trim()}
            disabled={count === 0}
            aria-pressed={active}
            title={count === 0
              ? `${category.label}：当前画布没有这类节点`
              : `${category.description}。当前 ${count} 个，点击仅高亮这一类，再点取消`}
            onClick={() => setNodeCategoryFocus(active ? '' : category.id)}
          >
            <i className={`catDot ${category.id}`} aria-hidden="true" />
            {category.label}
            <span className="categoryLegendCount">{count}</span>
          </button>
        )
      })}
    </div>
  ) : null
  const canvasHoverToolbar = isEditingCanvas ? (
    <section className="canvasHoverToolbar nodrag nopan" onMouseDown={event => event.stopPropagation()} aria-label="画布节点工具栏">
      <div className="canvasHoverToolbarPanel">
        <div className="canvasHoverToolbarHeader">
          <strong>添加节点</strong>
          <button type="button" onClick={() => openNodePicker()}>搜索全部</button>
        </div>
        {addControlNodeGrid}
        <div className="canvasHoverToolbarSection cat-tool">
          <span>
            <i className="catDot tool" aria-hidden="true" />
            插件工具
          </span>
          {addToolNodeGrid}
        </div>
      </div>
      <button type="button" className="canvasHoverToolbarTrigger" aria-label="添加节点工具栏" title="悬停展开快捷节点；拖动节点卡片到画布添加">
        <span aria-hidden="true">+</span>
        <span>添加节点</span>
      </button>
    </section>
  ) : null
  const editModeSidebar = (
    <section className="card editorToolbar workflowSidebar editModeSidebar">
      <div className="cardHeader">
        <h3>工作流编排</h3>
        <span>编辑模式 · {nodes.length} 节点 / {edges.length} 依赖</span>
      </div>
      {modeSwitch}
      {editorValidation?.errors?.length > 0 && <ValidationSummary status={editorValidation} />}
      <div className="form compact">
        {workflowLoader}
        <div className="workflowSidebarStatus" title={`${workflow.name || workflow.id || '未命名工作流'} · ${workflowOwnerLabel} · ${workflowCategoryText}`}>
          <strong>{workflow.name || workflow.id || '未命名工作流'}</strong>
          <span>
            {workflowOwnerLabel} · {workflowCategoryText}
            {(workflow.shared_with || []).length > 0 ? ` · 共享 ${workflow.shared_with.length} 个插件` : ''}
          </span>
        </div>
        <div className={`workflowDraftStatus ${draftStatus.kind}`} role="status" aria-live="polite">
          <strong>{draftStatus.title}</strong>
          <span>{draftStatus.detail}</span>
        </div>
        {editorNotice && <div className="workflowEditorNotice" role="status" aria-live="polite">{editorNotice}</div>}
        <div className="buttonRow">
          <button className="secondary" type="button" onClick={createWorkflow} disabled={Boolean(workflowAction)}>新建</button>
          <button className="secondary" type="button" onClick={validateDraft} disabled={Boolean(workflowAction)}>{workflowAction === 'validating' ? '校验中...' : '校验'}</button>
          <button className="primary" type="button" onClick={openSaveModal} disabled={Boolean(workflowAction)} title="保存（Ctrl+S）">保存</button>
          <button className="secondary danger" type="button" onClick={deleteWorkflow} disabled={!canDeleteWorkflow || Boolean(workflowAction)} title={workflowOwnerLocked ? '内置工作流不支持在此删除' : undefined}>删除工作流</button>
        </div>
        <div className="buttonRow">
          <button className="secondary" type="button" onClick={undoFlowEdit} disabled={!flowHistory.canUndo} title="撤销（Ctrl+Z）">撤销</button>
          <button className="secondary" type="button" onClick={redoFlowEdit} disabled={!flowHistory.canRedo} title="重做（Ctrl+Y 或 Ctrl+Shift+Z）">重做</button>
        </div>
        <div className="buttonRow">
          <button className="secondary danger" type="button" onClick={removeSelectedNode} disabled={!selectedNode}>删除节点</button>
          <button className="secondary danger" type="button" onClick={removeSelectedEdge} disabled={!selectedEdge}>删除依赖</button>
          <button className="secondary" type="button" onClick={clearSelection} disabled={!selectedNode && !selectedEdge}>取消选择</button>
        </div>
      </div>
    </section>
  )
  const executeModeSidebar = (
    <section className="card editorToolbar workflowSidebar executeModeSidebar">
      <div className="cardHeader">
        <h3>工作流执行</h3>
        <span title={`执行模式 · ${nodes.length} 节点 / ${edges.length} 依赖`}>执行模式 · {nodes.length} 节点 / {edges.length} 依赖</span>
      </div>
      {modeSwitch}
      {editorValidation?.errors?.length > 0 && <ValidationSummary status={editorValidation} />}
      <div className="form compact">
        {workflowLoader}
        <div className="workflowSidebarStatus" title={workflow.name || workflow.id || '未选择工作流'}>
          <strong>{workflow.name || workflow.id || '未选择工作流'}</strong>
        </div>
        <div className={isActiveRunRunning ? 'workflowSidebarStatus running' : 'workflowSidebarStatus'} title={activeRunID ? `运行 ID：${activeRunID}` : '执行后可在这里取消当前画布发起的运行任务。'}>
          <strong>{isActiveRunRunning ? '运行中' : activeRunID ? runStatusLabel(activeRunStatus) : '未运行'}</strong>
        </div>
        <div
          className="workflowSidebarStatus"
          title={runTarget === RUN_TARGET_FOLLOW ? '运行时沿用每个节点自己配置的执行目标。' : runTargetOverride ? '运行时会把所有节点改到该机器执行。' : '运行时只对未单独指定执行目标的节点生效。'}
        >
          <strong>执行机器：{runTargetLabel}</strong>
        </div>
        {nodes.length === 0 ? (
          <div className="workflowEmptyStart">
            <span>先创建工作流并添加至少一个节点，之后即可在此执行。</span>
            <button className="primary" type="button" onClick={createWorkflow}>新建工作流</button>
          </div>
        ) : isActiveRunRunning ? (
          <button className="secondary danger" type="button" onClick={cancelActiveRun} disabled={cancellingRunID === activeRunID} aria-label="取消当前工作流运行">
            {cancellingRunID === activeRunID ? '取消中' : '取消运行'}
          </button>
        ) : (
          <button className="primary" type="button" onClick={runWorkflowDockAction} aria-label="从执行面板运行工作流">
            运行工作流
          </button>
        )}
      </div>
    </section>
  )
  const workflowRunParamsContent = (
    <div className="form compact">
      {executeStep === 'params' ? (
        <>
          {workflowParameters.length + executableParamCount === 0 && (
            <div className="empty small">当前工作流无需参数。</div>
          )}
          {workflowParameters.length > 0 && (
            <div className="workflowRunParamSection">
              <strong title="工作流级运行参数">工作流参数</strong>
              {workflowParameters.map((param, index) => (
                <label key={param.name || index} className={param.required ? 'requiredField' : ''}>
                  <span title={param.description || param.name || `参数 ${index + 1}`}>{param.description || param.name || `参数 ${index + 1}`}{param.required ? ' *' : ''}</span>
                  {param.type === 'bool' ? (
                    <input
                      type="checkbox"
                      checked={parseBoolParamValue(runParams[param.name])}
                      onChange={event => updateRunParam(param.name, event.target.checked)}
                      disabled={!param.name || isActiveRunRunning}
                    />
                  ) : param.type === 'file' ? (
                    <FileUploadInput
                      value={formatRunParamInputValue(runParams[param.name])}
                      onChange={value => updateRunParam(param.name, value)}
                      disabled={!param.name || isActiveRunRunning}
                    />
                  ) : Array.isArray(param.options) && param.options.length > 0 ? (
                    <select
                      value={formatRunParamInputValue(runParams[param.name])}
                      onChange={event => updateRunParam(param.name, event.target.value)}
                      disabled={!param.name || isActiveRunRunning}
                    >
                      {param.options.map(opt => (
                        <option key={opt} value={opt}>{opt || '（默认）'}</option>
                      ))}
                    </select>
                  ) : (
                    <input
                      value={formatRunParamInputValue(runParams[param.name])}
                      placeholder={param.name || '参数名'}
                      onChange={event => updateRunParam(param.name, event.target.value)}
                      disabled={!param.name || isActiveRunRunning}
                    />
                  )}
                </label>
              ))}
            </div>
          )}
          {executableParamGroups.map(group => (
            <div className="workflowRunParamGroup" key={group.key}>
              <div className="workflowRunParamGroupHeader">
                <strong>{group.title}</strong>
                <span>{group.subtitle}</span>
              </div>
              {group.parameters.map(param => (
                <label key={`${group.key}:${param.name}`} className={param.required ? 'requiredField' : ''}>
                  <span title={param.description || param.name}>{param.description || param.name}{param.required ? ' *' : ''}</span>
                  {param.type === 'bool' ? (
                    <input
                      type="checkbox"
                      checked={parseBoolParamValue(group.params[param.name])}
                      onChange={event => updateExecutableNodeParam(group.nodeID, param.name, event.target.checked)}
                      disabled={isActiveRunRunning}
                    />
                  ) : param.type === 'file' ? (
                    <FileUploadInput
                      value={formatRunParamInputValue(group.params[param.name])}
                      onChange={value => updateExecutableNodeParam(group.nodeID, param.name, value)}
                      disabled={isActiveRunRunning}
                    />
                  ) : Array.isArray(param.options) && param.options.length > 0 ? (
                    <select
                      value={formatRunParamInputValue(group.params[param.name])}
                      onChange={event => updateExecutableNodeParam(group.nodeID, param.name, event.target.value)}
                      disabled={isActiveRunRunning}
                    >
                      <option value="">手动输入 / 不设置</option>
                      {param.options.map(opt => (
                        <option key={opt} value={opt}>{opt || '（默认）'}</option>
                      ))}
                    </select>
                  ) : (
                    <input
                      value={formatRunParamInputValue(group.params[param.name])}
                      placeholder={param.default || param.name}
                      onChange={event => updateExecutableNodeParam(group.nodeID, param.name, event.target.value)}
                      disabled={isActiveRunRunning}
                    />
                  )}
                </label>
              ))}
            </div>
          ))}
          {parsedRunParams.invalid && (
            <div className="empty small" title="高级 JSON 当前无效，逐项输入会以空对象重新生成参数。">高级 JSON 当前无效。</div>
          )}
          <div className="buttonRow">
            <button className="secondary" type="button" onClick={resetRunParamsToDefaults}>填入默认参数</button>
          </div>
          <details className="advancedParams workflowRunParamsAdvanced">
            <summary>高级 JSON</summary>
            <textarea
              className="workflowRunParamsInput"
              value={runParamsText}
              onChange={event => setRunParamsText(event.target.value)}
              placeholder='{"host":"127.0.0.1"}'
            />
          </details>
        </>
      ) : executeStep === 'target' ? (
        <>
          <div className="workflowRunParamSection">
            <strong>选择执行机器</strong>
            <div className="workflowTargetOptions" role="radiogroup" aria-label="工作流执行机器">
              <button
                type="button"
                role="radio"
                aria-checked={runTarget === RUN_TARGET_FOLLOW}
                className={runTarget === RUN_TARGET_FOLLOW ? 'workflowTargetOption active' : 'workflowTargetOption'}
                disabled={isActiveRunRunning}
                onClick={() => setRunTarget(RUN_TARGET_FOLLOW)}
              >
                <strong>跟随节点配置</strong>
                <small>每个节点使用自己配置的执行目标，未单独配置的在本机执行。</small>
              </button>
              <button
                type="button"
                role="radio"
                aria-checked={runTarget === RUN_TARGET_LOCAL}
                className={runTarget === RUN_TARGET_LOCAL ? 'workflowTargetOption active' : 'workflowTargetOption'}
                disabled={isActiveRunRunning}
                onClick={() => setRunTarget(RUN_TARGET_LOCAL)}
              >
                <strong>本机</strong>
                <small>在这台机器上执行，不经过 SSH 操作机。</small>
              </button>
              {runHostOptions.map(host => (
                <button
                  key={host.id}
                  type="button"
                  role="radio"
                  aria-checked={runTarget === host.id}
                  className={runTarget === host.id ? 'workflowTargetOption active' : 'workflowTargetOption'}
                  disabled={isActiveRunRunning || !host.enabled}
                  onClick={() => setRunTarget(host.id)}
                  title={host.enabled ? `${host.name || host.id} · ${host.user}@${host.address}:${host.port}` : `操作机 ${host.name || host.id} 已停用`}
                >
                  <strong>{host.name || host.id}</strong>
                  <small>{host.user}@{host.address}:{host.port} · {sshHostStatusLabel(host.status)}{host.enabled ? '' : ' · 已停用'}</small>
                </button>
              ))}
            </div>
            {runHostOptions.length === 0 && (
              <div className="empty small">还没有配置 SSH 操作机，目前只能选择本机或跟随节点配置。</div>
            )}
          </div>
          {runTarget !== RUN_TARGET_FOLLOW && (
            <div className="workflowRunParamSection">
              <label className="workflowTargetOverride">
                <input
                  type="checkbox"
                  checked={runTargetOverride}
                  disabled={isActiveRunRunning}
                  onChange={event => setRunTargetOverride(event.target.checked)}
                />
                <span>覆盖节点已配置的执行目标</span>
              </label>
              <div className="workflowSidebarHint">
                <span>
                  {runTargetOverride
                    ? `全部 ${nodes.length} 个节点都会改到「${runTargetLabel}」执行，节点自身配置的机器被忽略。`
                    : `仅对未单独指定执行目标的节点生效（${nodesWithOwnTarget} 个节点已单独指定，不受影响）。`}
                </span>
              </div>
            </div>
          )}
        </>
      ) : (
        <>
          <div className="workflowRunParamSection">
            <strong>运行确认</strong>
            <div className="workflowSidebarHint">
              <strong>{workflow.name || workflow.id || '未选择工作流'}</strong>
              <span>{workflowParameters.length + executableParamCount} 项参数 · {nodes.length} 个节点 · {edges.length} 条依赖</span>
            </div>
            <div className="workflowSidebarHint">
              <strong>执行机器：{runTargetLabel}</strong>
              <span>
                {runTarget === RUN_TARGET_FOLLOW
                  ? `沿用节点配置，${Math.max(nodes.length - nodesWithOwnTarget, 0)} 个节点在本机执行`
                  : runTargetOverride
                    ? `全部 ${nodes.length} 个节点都会改到该目标执行`
                    : `${runTargetAppliesTo} 个未指定执行目标的节点会改到该目标执行`}
              </span>
            </div>
          </div>
          {uploadFlowNodes(nodes).length > 0 && (
            <div className="workflowRunParamSection">
              <strong>上传文件</strong>
              {uploadFlowNodes(nodes).map(node => (
                <UploadNodeFilePicker
                  key={node.id}
                  node={node}
                  state={uploadStateForNode(uploadNodeFiles, node.id)}
                  summary={uploadFileSummary(uploadNodeFiles, node.id)}
                  disabled={isActiveRunRunning}
                  onFilesChange={(mode, files) => updateUploadNodeFiles(node.id, mode, files)}
                  onClear={() => clearUploadNodeFiles(node.id)}
                />
              ))}
            </div>
          )}
          <details className="advancedParams workflowRunParamsAdvanced">
            <summary>查看运行参数 JSON</summary>
            <textarea className="workflowRunParamsInput" value={runParamsText} readOnly />
          </details>
        </>
      )}
    </div>
  )
  const executeViewSwitch = !isEditingCanvas ? (
    <div className="workflowExecuteViewActions">
      <div className="segmentedControl" role="group" aria-label="工作流执行详情视图">
        <button type="button" className={executeDetailView === 'logs' ? 'active' : ''} onClick={() => setExecuteDetailView('logs')}>日志</button>
        <button type="button" className={executeDetailView === 'canvas' ? 'active' : ''} onClick={() => setExecuteDetailView('canvas')}>画布</button>
      </div>
    </div>
  ) : null
  const workflowCanvas = (
    <section className={`card canvasCard${nodeCategoryFocus ? ` cat-focus-${nodeCategoryFocus}` : ''}`} ref={canvasCardRef} onDragOver={handleCanvasDragOver} onDrop={handleCanvasDrop} onDoubleClick={handlePaneDoubleClick}>
      {executeViewSwitch}
      <ReactFlow
        nodes={displayNodes}
        edges={displayEdges}
        nodeTypes={nodeTypes}
        onNodesChange={handleNodesChange}
        onEdgesChange={handleEdgesChange}
        onConnect={onConnect}
        onConnectStart={handleConnectStart}
        onConnectEnd={handleConnectEnd}
        onReconnect={onReconnect}
        onNodeClick={(_, node) => handleCanvasNodeClick(node.id)}
        onEdgeClick={(_, edge) => openEdgeConfigModal(edge.id)}
        onEdgeDoubleClick={handleEdgeDoubleClick}
        onPaneClick={() => { clearSelection(); closeNodePicker() }}
        onInit={setFlowInstance}
        nodesDraggable={isEditingCanvas}
        nodesConnectable={isEditingCanvas}
        edgesReconnectable={isEditingCanvas}
        elementsSelectable={isEditingCanvas}
        deleteKeyCode={isEditingCanvas ? 'Backspace' : null}
        zoomOnDoubleClick={false}
        fitView
      >
        <MiniMap />
        <Controls />
        <Background />
        {isEditingCanvas && nodes.length === 0 && !nodePicker.open && (
          <div className="canvasEmptyCallout nodrag nopan" onMouseDown={event => event.stopPropagation()}>
            <strong>从添加节点开始编排</strong>
            <span>悬停画布底部工具栏，或拖出连线后选择下游节点。</span>
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
            panelPosition={nodePicker.panelPosition}
            canvasElement={canvasCardRef.current}
            mode={nodePicker.mode}
            connection={nodePicker.connection}
            insertEdge={nodePicker.insertEdge || pendingInsertEdge}
            onAddTool={tool => addToolNode(tool, nodePicker.position, {connection: nodePicker.connection, insertEdge: nodePicker.insertEdge || pendingInsertEdge})}
            onAddControl={controlType => addControlNode(controlType, nodePicker.position, {connection: nodePicker.connection, insertEdge: nodePicker.insertEdge || pendingInsertEdge})}
            onClose={closeNodePicker}
          />
        )}
        {canvasHoverToolbar}
        {canvasCategoryLegend}
        {!isEditingCanvas && canvasLogNodeID && (
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
        <CanvasDock
          onZoomIn={() => zoomCanvas('in')}
          onZoomOut={() => zoomCanvas('out')}
          onFitView={fitCanvasView}
          onAutoLayout={optimizeCanvasLayout}
          onRunWorkflow={runWorkflowDockAction}
          runDisabled={isActiveRunRunning}
        />
      </ReactFlow>
    </section>
  )
  const executeResultPanel = !isEditingCanvas && resultPanel ? (
    <div className="workflowExecuteResultPane">
      {executeViewSwitch}
      {resultPanel}
    </div>
  ) : null
  return (
    <div className={isEditingCanvas ? 'editorLayout editModeLayout' : 'editorLayout executeModeLayout'}>
      {isEditingCanvas ? editModeSidebar : executeModeSidebar}

      {isEditingCanvas || executeDetailView === 'canvas' ? workflowCanvas : null}
      {!isEditingCanvas && executeDetailView === 'logs' ? executeResultPanel : null}

      {saveModalOpen && (
        <div className="modalBackdrop" onClick={() => { if (workflowAction !== 'saving') setSaveModalOpen(false) }}>
          <section className="modal workflowSaveModal" role="dialog" aria-modal="true" aria-label="保存工作流" onClick={event => event.stopPropagation()}>
            <div className="modalHeader">
              <div>
                <h3>保存工作流</h3>
                <p>{workflowOwnerLabel} · {workflowCategoryText} · {nodes.length} 节点 / {edges.length} 依赖</p>
              </div>
              <button type="button" className="modalClose" onClick={() => setSaveModalOpen(false)} disabled={workflowAction === 'saving'}>×</button>
            </div>
            {saveModalErrors.length > 0 && (
              <div ref={saveModalErrorsRef}>
                <ValidationSummary status={{title: saveModalErrorTitle, errors: saveModalErrors}} />
              </div>
            )}
            <div className="form compact">
              <label>
                <span>工作流 ID</span>
                <input
                  value={workflow.id || ''}
                  readOnly={Boolean(selectedWorkflowID)}
                  disabled={Boolean(workflowAction)}
                  onChange={event => { clearCanvasRunOverlay(); setWorkflow({...workflow, id: event.target.value}) }}
                  placeholder="demo.my-flow"
                />
                <small className="workflowSelectionHint">
                  {selectedWorkflowID ? '工作流 ID 是唯一标识，保存后不可修改。' : '保存后作为唯一标识，不能与已有工作流重复。'}
                </small>
              </label>
              <label>
                <span>名称</span>
                <input value={workflow.name || ''} disabled={Boolean(workflowAction)} onChange={event => { clearCanvasRunOverlay(); setWorkflow({...workflow, name: event.target.value}) }} placeholder="工作流名称" />
              </label>
              <label>
                <span>描述</span>
                <textarea
                  className="workflowDescriptionInput"
                  value={workflow.description || ''}
                  disabled={Boolean(workflowAction)}
                  onChange={event => { clearCanvasRunOverlay(); setWorkflow({...workflow, description: event.target.value}) }}
                  placeholder="工作流描述，可分多行记录用途、前置条件或注意事项"
                />
              </label>
              <label>
                <span>归属</span>
                <select
                  value={workflowOwnerLocked ? '__builtin__' : workflowOwnerID}
                  disabled={workflowOwnerLocked || Boolean(workflowAction)}
                  onChange={event => changeWorkflowOwner(event.target.value)}
                >
                  {workflowOwnerLocked ? (
                    <option value="__builtin__">内置工作流（不可迁移）</option>
                  ) : (
                    <>
                      <option value="user.workflows">全局工作流</option>
                      {workflowOwnerOptions.map(item => <option key={item.id} value={item.id}>{item.name || item.id}（{item.id}）</option>)}
                    </>
                  )}
                </select>
                <small className="workflowSelectionHint">
                  {workflowOwnerLocked
                    ? '内置工作流保存在原配置文件，不能修改归属。'
                    : workflowOwnerID === 'user.workflows'
                      ? '保存在 user.workflows 目录，不绑定业务插件，可在所有分类中管理；不随业务插件 ZIP 导出。'
                      : `保存后 YAML 写入插件 ${workflowOwnerID} 的 workflows/ 目录，导出该插件 ZIP 时自动携带；分类跟随该插件。`}
                </small>
              </label>
              {!workflowOwnerLocked && workflowOwnerID !== 'user.workflows' && workflowOwnerCategories.length > 1 ? (
                <label>
                  <span>插件分类</span>
                  <select
                    value={workflow.category || workflowOwnerCategories[0].id}
                    disabled={Boolean(workflowAction)}
                    onChange={event => { clearCanvasRunOverlay(); setWorkflow({...workflow, category: event.target.value}) }}
                  >
                    {workflowOwnerCategories.map(item => <option key={item.id} value={item.id}>{item.name || item.id}</option>)}
                    {workflow.category && !workflowOwnerCategories.some(item => item.id === workflow.category) && (
                      <option value={workflow.category}>{workflow.category}</option>
                    )}
                  </select>
                  <small className="workflowSelectionHint">归属插件声明了多个分类，只能在其声明的范围内选择。</small>
                </label>
              ) : workflowOwnerLocked || workflowOwnerID === 'user.workflows' ? (
                <label>
                  <span>分类</span>
                  <select
                    value={workflow.category || 'global'}
                    disabled={Boolean(workflowAction)}
                    onChange={event => { clearCanvasRunOverlay(); setWorkflow({...workflow, category: event.target.value}) }}
                  >
                    {(catalog.categories || []).map(item => <option key={item.id} value={item.id}>{item.name || item.id}</option>)}
                    <option value="global">全局（跨分类）</option>
                    {workflow.category && workflow.category !== 'global' && !(catalog.categories || []).some(item => item.id === workflow.category) && (
                      <option value={workflow.category}>{workflow.category}</option>
                    )}
                  </select>
                  <small className="workflowSelectionHint">
                    {workflowOwnerID === 'user.workflows'
                      ? '全局工作流没有归属插件，分类仅用于筛选与分组。'
                      : '用于工作流管理的分类筛选，不影响可添加的工具。'}
                  </small>
                </label>
              ) : null}
              {!workflowOwnerLocked && workflowShareOptions.length > 0 && (
                <div className="workflowShareField">
                  <span>共享给（可选）</span>
                  <div className="workflowShareOptions">
                    {workflowShareOptions.map(item => {
                      const active = (workflow.shared_with || []).includes(item.id)
                      return (
                        <button
                          key={item.id}
                          type="button"
                          className={active ? 'workflowShareChip active' : 'workflowShareChip'}
                          aria-pressed={active}
                          disabled={Boolean(workflowAction)}
                          onClick={() => toggleWorkflowSharedWith(item.id)}
                          title={item.id}
                        >
                          {item.name || item.id}
                        </button>
                      )
                    })}
                  </div>
                  <small className="workflowSelectionHint">共享仅让该工作流出现在所选插件的分类下；文件仍保存在归属插件目录，不复制、不随对方插件 ZIP 导出。</small>
                </div>
              )}
              <details className="workflowSaveAdvanced">
                <summary>高级设置{workflowParameters.length > 0 ? `（已配置 ${workflowParameters.length} 个参数）` : ''}</summary>
                <label>
                  <span>工作流参数定义 JSON</span>
                  <textarea
                    className="workflowRunParamsInput"
                    value={workflowParamsText}
                    disabled={Boolean(workflowAction)}
                    onChange={event => {
                      clearCanvasRunOverlay()
                      setWorkflowParamsText(event.target.value)
                      setEditorValidation(null)
                    }}
                    placeholder='[{"name":"host","type":"string","required":true,"default":"127.0.0.1"}]'
                  />
                </label>
              </details>
            </div>
            <div className="modalFooter">
              <button className="secondary" type="button" onClick={() => setSaveModalOpen(false)} disabled={workflowAction === 'saving'}>取消</button>
              <button className="primary" type="button" onClick={saveDraft} disabled={Boolean(workflowAction)}>{workflowAction === 'saving' ? '保存中...' : '保存'}</button>
            </div>
          </section>
        </div>
      )}

      {runParamsModalOpen && (
        <div className="modalBackdrop" onClick={() => setRunParamsModalOpen(false)}>
          <section className="modal workflowRunParamsPanel workflowRunParamsModal" role="dialog" aria-modal="true" aria-label="填写运行参数" onClick={event => event.stopPropagation()}>
            <div className="modalHeader">
              <div>
                <h3>{executeStep === 'params' ? '填写运行参数' : executeStep === 'target' ? '选择执行机器' : '确认运行'}</h3>
                <p>步骤 {executeStep === 'params' ? '1' : executeStep === 'target' ? '2' : '3'} / 3 · {workflowParameters.length + executableParamCount} 项参数 · {nodes.length} 个节点 · 执行机器：{runTargetLabel}</p>
              </div>
              <button type="button" className="modalClose" onClick={() => setRunParamsModalOpen(false)}>×</button>
            </div>
            {workflowRunParamsContent}
            <div className="modalFooter workflowRunActionRow">
              {executeStep === 'params' ? (
                <>
                  <button className="secondary" type="button" onClick={validateDraft}>校验</button>
                  <button className="primary" type="button" onClick={prepareRunStep} disabled={isActiveRunRunning}>下一步</button>
                </>
              ) : executeStep === 'target' ? (
                <>
                  <button className="secondary" type="button" onClick={() => setExecuteStep('params')} disabled={isActiveRunRunning}>上一步</button>
                  <button className="primary" type="button" onClick={prepareRunConfirmStep} disabled={isActiveRunRunning}>下一步</button>
                </>
              ) : (
                <>
                  <button className="secondary" type="button" onClick={() => setExecuteStep('target')} disabled={isActiveRunRunning}>上一步</button>
                  <button
                    className="primary"
                    type="button"
                    onClick={runDraft}
                    disabled={isActiveRunRunning}
                  >
                    开始运行
                  </button>
                  {isActiveRunRunning && (
                    <button className="secondary danger" type="button" onClick={cancelActiveRun} disabled={cancellingRunID === activeRunID}>
                      {cancellingRunID === activeRunID ? '取消中' : '取消运行'}
                    </button>
                  )}
                </>
              )}
            </div>
          </section>
        </div>
      )}

      {isEditingCanvas && nodeConfigModalOpen && configNode && (
        <NodeConfigModal
          node={configNode}
          kindLabel={selectedNodeKindLabel}
          onClose={closeNodeConfigModal}
          onSave={saveNodeConfig}
        >
          <NodeConfigEditor
            node={configNode}
            tool={selectedTool}
            hosts={hosts}
            sources={selectedNodeSources}
            paramsText={nodeParamsText}
            setParamsText={setNodeParamsText}
            onNameChange={updateSelectedNodeName}
            onConditionChange={updateSelectedNodeCondition}
            onLoopChange={updateSelectedNodeLoop}
            onUploadChange={updateSelectedNodeUpload}
            onExtractConfigChange={updateSelectedNodeExtractConfig}
            onHttpChange={updateSelectedNodeHttpRequest}
            onHostChange={updateSelectedNodeHost}
            loopTool={selectedLoopTool}
            nodes={nodes}
            tools={toolOptions}
            onParamChange={updateMappedParam}
            onLoopParamChange={updateSelectedLoopParam}
            onApplyParams={applyNodeParams}
          />
        </NodeConfigModal>
      )}

      {isEditingCanvas && edgeConfigModalOpen && selectedEdge && (
        <EdgeConfigModal
          edge={selectedEdge}
          sourceNode={nodes.find(node => node.id === selectedEdge.source)}
          onCaseChange={updateSelectedEdgeCase}
          onClose={() => setEdgeConfigModalOpen(false)}
        />
      )}

      {rerunModal && (
        <RerunParamsModal
          nodeID={rerunModal.nodeID}
          params={rerunModal.baseParams}
          confirmRequired={rerunModal.confirmRequired}
          onClose={() => setRerunModal(null)}
          onSubmit={params => submitRerun(rerunModal.nodeID, params)}
        />
      )}
    </div>
  )
}

export function emptyCanvasRunState() {
  return {
    status: 'idle',
    nodes: {},
    edges: {},
    conditionMatches: {},
    message: ''
  }
}

export function RerunParamsModal({nodeID, params, confirmRequired, onClose, onSubmit}) {
  const [paramsText, setParamsText] = useState(() => JSON.stringify(params || {}, null, 2))
  const [error, setError] = useState('')

  function submit() {
    try {
      const parsed = JSON.parse(paramsText || '{}')
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        setError('运行参数必须是 JSON 对象。')
        return
      }
      setError('')
      onSubmit(parsed)
    } catch (err) {
      setError(`运行参数 JSON 无效：${err.message}`)
    }
  }

  return (
    <div className="modalBackdrop" onClick={onClose}>
      <div className="modal rerunParamsModal" onClick={event => event.stopPropagation()}>
        <div className="modalHeader">
          <div>
            <h3>重跑参数</h3>
            <p>从节点 {nodeID} 开始重跑，可先调整本次运行参数。</p>
          </div>
          <button type="button" className="modalClose" onClick={onClose}>×</button>
        </div>
        <div className="form compact">
          {confirmRequired && <div className="warningText">该工作流需要确认，提交后会按当前确认状态重跑。</div>}
          <label>
            <span>运行参数 JSON</span>
            <textarea
              className="workflowRunParamsInput"
              value={paramsText}
              onChange={event => {
                setParamsText(event.target.value)
                setError('')
              }}
              placeholder='{"host":"127.0.0.1"}'
            />
          </label>
          {error && <div className="validationSummary error"><strong>{error}</strong></div>}
        </div>
        <div className="modalFooter">
          <button type="button" className="secondary" onClick={onClose}>取消</button>
          <button type="button" className="primary" onClick={submit}>开始重跑</button>
        </div>
      </div>
    </div>
  )
}

export function shouldClearCanvasRunOverlayForNodeChanges(changes) {
  return (changes || []).some(change => change.type === 'remove')
}

function sleep(ms) {
  return new Promise(resolve => window.setTimeout(resolve, ms))
}

function workflowCategoryKey(item) {
  return String(item?.category || '').trim() || UNCATEGORIZED_WORKFLOW_CATEGORY
}

function pluginCategories(catalog, ownerID) {
  const plugin = (catalog?.plugins || []).find(item => item.id === ownerID)
  return (plugin?.categories || []).filter(item => item && item.id)
}

function workflowCategoryLabel(category, labels) {
  if (category === UNCATEGORIZED_WORKFLOW_CATEGORY) return '未分类'
  if (category === 'global') return '全局'
  return labels.get(category) || category
}

function rememberedWorkflowKey(category) {
  return `${REMEMBERED_WORKFLOW_PREFIX}:${category || 'global'}`
}

function readRememberedWorkflowID(category) {
  try {
    return window.localStorage?.getItem(rememberedWorkflowKey(category)) || ''
  } catch {
    return ''
  }
}

function rememberWorkflowID(category, id) {
  if (!id) return
  try {
    window.localStorage?.setItem(rememberedWorkflowKey(category), id)
  } catch {
    // Ignore storage failures; workflow selection still works for this session.
  }
}

function forgetRememberedWorkflowID(category, id = '') {
  try {
    const key = rememberedWorkflowKey(category)
    if (id && window.localStorage?.getItem(key) !== id) return
    window.localStorage?.removeItem(key)
  } catch {
    // Ignore storage failures; stale selections are non-critical.
  }
}

function parseJSONObject(value) {
  try {
    const parsed = JSON.parse(value || '{}')
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {value: {}, invalid: true}
    }
    return {value: parsed, invalid: false}
  } catch {
    return {value: {}, invalid: true}
  }
}

function formatRunParamInputValue(value) {
  if (value === undefined || value === null) return ''
  return typeof value === 'string' ? value : String(value)
}

function parseBoolParamValue(value) {
  return value === true || value === 'true' || value === '1' || value === 'yes' || value === 'on'
}

function uploadFlowNodes(nodes) {
  return (nodes || []).filter(node => node.type === 'controlNode' && node.data?.controlType === 'upload')
}

function uploadStateForNode(uploadNodeFiles, nodeID) {
  const value = uploadNodeFiles?.[nodeID]
  if (!value) return {mode: 'files', files: []}
  if (Array.isArray(value)) return {mode: 'files', files: value.filter(Boolean)}
  return {
    mode: value.mode === 'directory' ? 'directory' : 'files',
    files: Array.isArray(value.files) ? value.files.filter(Boolean) : []
  }
}

function uploadFilesForNode(uploadNodeFiles, nodeID) {
  return uploadStateForNode(uploadNodeFiles, nodeID).files
}

function uploadFileSummary(uploadNodeFiles, nodeID) {
  const files = uploadFilesForNode(uploadNodeFiles, nodeID)
  if (files.length === 0) return '未选择'
  const totalSize = uploadFilesTotalSize(files)
  return `${files.length} 个文件 · ${formatBytes(totalSize)}`
}

function uploadFilesTotalSize(files) {
  return (files || []).reduce((total, file) => total + Number(file.size || 0), 0)
}

async function submitWaitingUploadNodes(runID, detailData, nodes, uploadNodeFilesSnapshot, submittedUploadNodes, setResult, setUploadProgress) {
  const uploadNodes = new Map(uploadFlowNodes(nodes).map(node => [node.id, node]))
  const waitingSteps = (detailData?.record?.steps || []).filter(step => step?.type === 'upload' && normalizeRunStatus(step.status) === 'waiting')
  for (const step of waitingSteps) {
    if (submittedUploadNodes.has(step.id)) continue
    const node = uploadNodes.get(step.id)
    if (!node) continue
    const files = uploadFilesForNode(uploadNodeFilesSnapshot, step.id)
    if (files.length === 0) {
      throw new Error(`上传节点 ${step.id} 缺少已选择的文件或目录。`)
    }
    submittedUploadNodes.add(step.id)
    const targetDir = workflowModel.normalizeUploadConfig(node.data.upload || {}).target_dir
    try {
      setUploadProgress(uploadProgressState(step.id, files, 0, 'uploading', '准备上传'))
      await postRunUploadNodeChunked(runID, step.id, files, targetDir, progress => {
        const percent = progress.total > 0 ? Math.floor(progress.uploaded / progress.total * 100) : 0
        const nextProgress = uploadProgressState(step.id, files, progress.uploaded, 'uploading', progress.fileName || '')
        setUploadProgress(nextProgress)
        setResult({
          message: `上传节点 ${step.id} 正在上传：${formatBytes(progress.uploaded)} / ${formatBytes(progress.total)} (${percent}%)`,
          uploadProgress: nextProgress,
          detail: {data: detailData},
          run: {id: runID, status: 'running'}
        })
      })
      const doneProgress = uploadProgressState(step.id, files, uploadFilesTotalSize(files), 'succeeded', '上传完成')
      setUploadProgress(doneProgress)
      setResult({
        message: `上传节点 ${step.id} 上传完成：${formatBytes(doneProgress.uploaded)} / ${formatBytes(doneProgress.total)}`,
        uploadProgress: doneProgress,
        detail: {data: detailData},
        run: {id: runID, status: 'running'}
      })
    } catch (err) {
      submittedUploadNodes.delete(step.id)
      setUploadProgress(current => current?.nodeID === step.id ? {...current, status: 'failed', label: '上传失败'} : current)
      const message = readableAPIError(err, `上传节点 ${step.id} 上传失败。`)
      throw Object.assign(new Error(message), {body: err.body})
    }
  }
}

function uploadProgressState(nodeID, files, uploaded, status, label) {
  const total = uploadFilesTotalSize(files)
  const percent = total > 0 ? Math.min(100, Math.floor(uploaded / total * 100)) : 0
  return {
    nodeID,
    uploaded,
    total,
    percent,
    status,
    label,
    fileCount: files.length
  }
}

function UploadProgressInline({progress}) {
  return (
    <div className={`uploadProgressInline uploadProgress${capitalizeProgressStatus(progress.status)}`}>
      <div className="uploadProgressMeta">
        <span>{progress.label || '上传中'}</span>
        <span>{formatBytes(progress.uploaded)} / {formatBytes(progress.total)} · {progress.percent}%</span>
      </div>
      <div className="uploadProgressTrack" role="progressbar" aria-valuenow={progress.percent} aria-valuemin="0" aria-valuemax="100">
        <span style={{width: `${progress.percent}%`}} />
      </div>
    </div>
  )
}

function UploadNodeFilePicker({node, state, summary, disabled, onFilesChange, onClear}) {
  const fileInputRef = useRef(null)
  const directoryInputRef = useRef(null)
  const upload = workflowModel.normalizeUploadConfig(node.data?.upload || {})
  const modeLabel = state.mode === 'directory' ? '目录上传' : '文件上传'
  const targetLabel = upload.target_dir ? `uploads/${upload.target_dir}` : '默认 uploads 目录'
  return (
    <div className="workflowUploadPicker">
      <div className="workflowSidebarHint">
        <strong>{node.data?.name || node.id}</strong>
        <span>{modeLabel} · {summary}</span>
        <span>目标：{targetLabel}</span>
      </div>
      {state.files.length > 0 && (
        <div className="uploadSelectionMeta" title={uploadSelectionTitle(state.files)}>
          <span>{state.files.length} 个条目</span>
          <span>{formatBytes(uploadFilesTotalSize(state.files))}</span>
        </div>
      )}
      <div className="buttonRow">
        <button type="button" className="secondary" disabled={disabled} onClick={() => fileInputRef.current?.click()}>选择文件</button>
        <button type="button" className="secondary" disabled={disabled} onClick={() => directoryInputRef.current?.click()}>选择目录</button>
        <button type="button" className="secondary" disabled={disabled || state.files.length === 0} onClick={onClear}>清空</button>
      </div>
      <input
        ref={fileInputRef}
        className="hiddenFileInput"
        type="file"
        multiple
        disabled={disabled}
        onChange={event => onFilesChange('files', event.target.files)}
      />
      <input
        ref={directoryInputRef}
        className="hiddenFileInput"
        type="file"
        multiple
        webkitdirectory=""
        disabled={disabled}
        onChange={event => onFilesChange('directory', event.target.files)}
      />
    </div>
  )
}

function uploadSelectionTitle(files) {
  return files.slice(0, 12).map(file => file.webkitRelativePath || file.name).join('\n')
}

export function CanvasNodeLogPanel({nodeID, item, run, minimized, onMinimize, onRestore, onClose}) {
  const stdout = item?.stdout || ''
  const stderr = item?.stderr || run?.error || run?.skippedReason || ''
  const title = item?.title || nodeID
  const status = item?.status || run?.status || 'waiting'
  if (minimized) {
    return (
      <section className="canvasNodeLogPanel minimized nodrag nopan" onMouseDown={event => event.stopPropagation()}>
        <div className="canvasNodeLogHeader">
          <div>
            <strong>{title}</strong>
            <span>{runStatusLabel(status)}</span>
          </div>
          <div className="canvasNodeLogActions">
            <button type="button" className="secondary" onClick={onRestore}>展开</button>
            <button type="button" className="secondary" onClick={onClose}>关闭</button>
          </div>
        </div>
      </section>
    )
  }
  return (
    <section className="canvasNodeLogPanel nodrag nopan" onMouseDown={event => event.stopPropagation()}>
      <div className="canvasNodeLogHeader">
        <div>
          <strong>{title}</strong>
          <span>{runStatusLabel(status)}</span>
        </div>
        <div className="canvasNodeLogActions">
          <button type="button" className="secondary" onClick={onMinimize}>最小化</button>
          <button type="button" className="secondary" onClick={onClose}>关闭</button>
        </div>
      </div>
      {!item && !stdout && !stderr && (
        <div className="empty small">当前节点暂无日志，运行开始或节点完成后会显示。</div>
      )}
      {stdout && <InlineLogBlock title="标准输出" value={stdout} />}
      {stderr && <InlineLogBlock title="错误输出" value={stderr} />}
      {item?.children?.length > 0 && (
        <div className="canvasNodeLogChildren">
          {item.children.map(child => (
            <details key={child.id}>
              <summary>{child.title || child.id}</summary>
              {child.stdout && <InlineLogBlock title="标准输出" value={child.stdout} />}
              {child.stderr && <InlineLogBlock title="错误输出" value={child.stderr} />}
            </details>
          ))}
        </div>
      )}
    </section>
  )
}

function InlineLogBlock({title, value}) {
  return (
    <div className="canvasInlineLogBlock">
      <h4>{title}</h4>
      <pre>{value || '无日志内容'}</pre>
    </div>
  )
}

export function findRunLogItem(items, nodeID) {
  for (const item of items || []) {
    if (item?.id === nodeID) return item
    const child = findRunLogItem(item?.children || [], nodeID)
    if (child) return child
  }
  return null
}

function capitalizeProgressStatus(status) {
  const value = String(status || 'uploading')
  return value.charAt(0).toUpperCase() + value.slice(1)
}

function formatBytes(value) {
  const size = Number(value || 0)
  if (size >= 1024 * 1024 * 1024) return `${(size / 1024 / 1024 / 1024).toFixed(1)} GB`
  if (size >= 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`
  if (size >= 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${size} B`
}

function buildExecutableParamGroups(nodes, toolMap) {
  return (nodes || []).flatMap(node => {
    if (node.type === 'toolNode') {
      const tool = toolMap.get(node.data?.tool)
      const parameters = tool?.parameters || []
      if (parameters.length === 0) return []
      return [{
        key: node.id,
        nodeID: node.id,
        title: node.data?.name || node.id,
        subtitle: tool?.name || node.data?.tool || '工具节点',
        parameters,
        params: node.data?.params || {}
      }]
    }
    if (node.type === 'controlNode' && node.data?.controlType === 'loop') {
      const loop = normalizeLoopConfig(node.data.loop || defaultLoop())
      const tool = toolMap.get(loop.tool)
      const parameters = tool?.parameters || []
      if (parameters.length === 0) return []
      return [{
        key: `${node.id}:loop`,
        nodeID: node.id,
        title: node.data?.name || node.id,
        subtitle: `循环工具：${tool?.name || loop.tool}`,
        parameters,
        params: loop.params || {}
      }]
    }
    return []
  })
}

function updateNodeExecutableParam(node, nodeID, name, value) {
  if (node.id !== nodeID) return node
  if (node.type === 'toolNode') {
    return {...node, data: {...node.data, params: {...(node.data.params || {}), [name]: value}}}
  }
  if (node.type === 'controlNode' && node.data?.controlType === 'loop') {
    const loop = normalizeLoopConfig(node.data.loop || defaultLoop())
    return {
      ...node,
      data: {
        ...node.data,
        loop: {...loop, params: {...(loop.params || {}), [name]: value}}
      }
    }
  }
  return node
}

function buildRunningCanvasRunState(nodes, edges = []) {
  const overlay = emptyCanvasRunState()
  overlay.status = 'running'
  overlay.message = '工作流执行中'
  ;(nodes || []).forEach(node => {
    overlay.nodes[node.id] = {
      status: 'waiting',
      label: runStatusLabel('waiting'),
      title: '等待中'
    }
  })
  findEntryNodes(nodes, edges).forEach(node => {
    overlay.nodes[node.id] = {
      status: 'running',
      label: runStatusLabel('running'),
      title: '执行中'
    }
  })
  return overlay
}

function findEntryNodes(nodes, edges = []) {
  const nodeIDs = new Set((nodes || []).map(node => node.id))
  const incoming = new Set()
  ;(edges || []).forEach(edge => {
    const source = edge.source || edge.from
    const target = edge.target || edge.to
    if (!nodeIDs.has(source) || !nodeIDs.has(target) || source === target) return
    incoming.add(target)
  })
  const entries = (nodes || []).filter(node => !incoming.has(node.id))
  return entries.length > 0 ? entries : (nodes || []).slice(0, 1)
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

function buildRerunCanvasRunState(nodes, edges, currentState, nodeID) {
  const overlay = {
    ...emptyCanvasRunState(),
    status: 'running',
    message: `正在从节点 ${nodeID} 开始重跑。`,
    nodes: {...(currentState?.nodes || {})},
    conditionMatches: {...(currentState?.conditionMatches || {})}
  }
  reachableNodeIDs(nodeID, edges).forEach(id => {
    overlay.nodes[id] = {
      ...(overlay.nodes[id] || {}),
      status: 'running',
      label: runStatusLabel('running'),
      title: id === nodeID ? '正在重跑该节点' : '等待上游重跑完成'
    }
  })
  return overlay
}

function reachableNodeIDs(startID, edges) {
  if (!startID) return new Set()
  const children = new Map()
  ;(edges || []).forEach(edge => {
    if (!children.has(edge.source)) children.set(edge.source, [])
    children.get(edge.source).push(edge.target)
  })
  const out = new Set()
  const queue = [startID]
  while (queue.length > 0) {
    const current = queue.shift()
    if (out.has(current)) continue
    out.add(current)
    ;(children.get(current) || []).forEach(child => queue.push(child))
  }
  return out
}

export function buildCanvasRunStateFromDetail(detail, nodes, fallbackError = '') {
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
      overlay.nodes[node.id] = overlay.nodes[node.id] || {status: 'waiting', label: runStatusLabel('waiting'), title: '等待中'}
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

function appendFlowEdge(edges, params, nodes) {
  const isDuplicate = (edges || []).some(edge => (
    edge.source === params.source &&
    edge.target === params.target &&
    (edge.sourceHandle || null) === (params.sourceHandle || null) &&
    (edge.targetHandle || null) === (params.targetHandle || null)
  ))
  if (isDuplicate) return edges
  const sourceNode = (nodes || []).find(node => node.id === params.source)
  const edgeCase = edgeCaseFromHandle(sourceNode, params.sourceHandle)
  const label = edgeCase ? conditionCaseLabel(sourceNode?.data.condition, edgeCase) : ''
  return [
    ...(edges || []),
    {
      ...params,
      id: `${params.source}-${params.target}-${params.sourceHandle || 'source'}-${params.targetHandle || 'target'}-${Date.now()}`,
      type: 'smoothstep',
      animated: true,
      label,
      data: edgeCase ? {case: edgeCase} : {}
    }
  ]
}

function canInsertIntoEdge(insertEdge, edges, nodes) {
  if (!insertEdge?.id) return false
  const edge = (edges || []).find(item => item.id === insertEdge.id)
  if (!edge) return false
  const nodeIDs = new Set((nodes || []).map(node => node.id))
  return nodeIDs.has(edge.source) && nodeIDs.has(edge.target)
}

function insertNodeIntoEdge(edges, insertEdge, nextNode, nodes) {
  const currentEdge = (edges || []).find(edge => edge.id === insertEdge.id)
  if (!currentEdge) return edges
  const sourceParams = {
    source: currentEdge.source,
    sourceHandle: currentEdge.sourceHandle || currentEdge.data?.case || undefined,
    target: nextNode.id,
    targetHandle: undefined
  }
  const targetParams = {
    source: nextNode.id,
    sourceHandle: undefined,
    target: currentEdge.target,
    targetHandle: currentEdge.targetHandle || undefined
  }
  const remainingEdges = (edges || []).filter(edge => edge.id !== currentEdge.id)
  return appendFlowEdge(appendFlowEdge(remainingEdges, sourceParams, nodes), targetParams, nodes)
}

function normalizeFlowEdges(edges, nodes) {
  return (edges || []).map(edge => {
    const sourceNode = (nodes || []).find(node => node.id === edge.source)
    const edgeCase = edgeCaseFromHandle(sourceNode, edge.sourceHandle)
    const label = edgeCase ? conditionCaseLabel(sourceNode?.data.condition, edgeCase) : ''
    return {
      ...edge,
      label,
      data: edgeCase ? {...(edge.data || {}), case: edgeCase} : {}
    }
  })
}

function stripRuntimeEdgeState(edge) {
  if (!edge) return null
  const {className, selected, data, ...rest} = edge
  const {run, ...cleanData} = data || {}
  return {
    ...rest,
    data: cleanData
  }
}

function downstreamInsertPosition(node) {
  const position = node?.position || {x: 80, y: 120}
  const size = autoLayoutNodeSize(node)
  return {
    x: position.x + size.width + 120,
    y: position.y + Math.max(0, size.height / 2 - 36)
  }
}

function edgeMidpoint(edge, nodes) {
  const sourceNode = (nodes || []).find(node => node.id === edge?.source)
  const targetNode = (nodes || []).find(node => node.id === edge?.target)
  if (!sourceNode || !targetNode) return {x: 80, y: 120}
  return {
    x: (sourceNode.position.x + targetNode.position.x) / 2,
    y: (sourceNode.position.y + targetNode.position.y) / 2
  }
}

function panelPositionFromFlowPosition(position, flowInstance, element) {
  if (!position || !flowInstance || !element) return null
  const point = flowInstance.flowToScreenPosition(position)
  return panelPositionFromPoint(point, element)
}

function eventPoint(event) {
  const source = event?.changedTouches?.[0] || event?.touches?.[0] || event
  if (typeof source?.clientX !== 'number' || typeof source?.clientY !== 'number') return null
  return {x: source.clientX, y: source.clientY}
}

function panelPositionFromPoint(point, element) {
  if (!point || !element) return null
  const bounds = element.getBoundingClientRect()
  return {
    x: point.x - bounds.left,
    y: point.y - bounds.top
  }
}

function isPaneDoubleClick(event) {
  const target = event?.target
  if (!target?.closest) return true
  return Boolean(target.closest('.react-flow__pane')) &&
    !target.closest('.react-flow__node') &&
    !target.closest('.react-flow__edge') &&
    !target.closest('.nodrag') &&
    !target.closest('button, input, textarea, select')
}

function pickerPanelStyle(position, element) {
  if (!position) return undefined
  const viewportWidth = element?.clientWidth || window.innerWidth || position.x
  const viewportHeight = element?.clientHeight || window.innerHeight || position.y
  const x = Math.max(220, Math.min(position.x, Math.max(220, viewportWidth - 220)))
  const y = Math.max(120, Math.min(position.y, Math.max(120, viewportHeight - 120)))
  return {
    left: `${x}px`,
    top: `${y}px`
  }
}

function toolPickerMeta(tool) {
  const paramCount = (tool.parameters || []).length
  const source = tool.source?.plugin_name || tool.source?.plugin_id || tool.category || '插件工具'
  return `${source} · ${paramCount} 参数`
}

function toolSourceLabel(tool) {
  if (!tool) return '未知工具'
  if (tool.source?.type === 'plugin') return tool.source.plugin_name || tool.source.plugin_id || '插件工具'
  return tool.category || '插件工具'
}

function toolParamStatus(tool, params = {}) {
  if (!tool) {
    return {
      total: 0,
      configured: 0,
      required: 0,
      missingRequired: 0,
      toolMissing: true,
      label: '工具未注册，无法检查参数',
      title: '参数状态：工具未注册，无法检查参数定义'
    }
  }
  const parameters = tool.parameters || []
  const total = parameters.length
  const required = parameters.filter(param => param.required).length
  const configured = parameters.filter(param => hasConfiguredParamValue(params?.[param.name])).length
  const missingRequired = parameters.filter(param => param.required && !hasConfiguredParamValue(params?.[param.name])).length
  const label = total === 0
    ? '无需参数'
    : missingRequired > 0
      ? `参数 ${configured}/${total} · 缺 ${missingRequired} 必填`
      : `参数 ${configured}/${total} · 必填已就绪`
  return {
    total,
    configured,
    required,
    missingRequired,
    toolMissing: false,
    label,
    title: `参数状态：共 ${total} 个，已配置 ${configured} 个，必填 ${required} 个，缺失必填 ${missingRequired} 个`
  }
}

function hasConfiguredParamValue(value) {
  if (value === undefined || value === null) return false
  if (Array.isArray(value)) return value.length > 0
  if (typeof value === 'object') return Object.keys(value).length > 0
  return String(value).trim() !== ''
}

export function buildDisplayNodes(nodes, runState, tools = [], onAddDownstream = null, canRemove = true, onRerun = null, activeRunID = '', hosts = []) {
  const overlayNodes = runState?.nodes || {}
  const toolMap = new Map((tools || []).map(tool => [tool.id, tool]))
  const hostMap = new Map((hosts || []).map(host => [host.id, host]))
  return (nodes || []).map(node => {
    const tool = node.type === 'toolNode' ? toolMap.get(node.data?.tool) : null
    const nodeRun = overlayNodes[node.id] || null
    const hostID = node.data?.host || ''
    const host = hostID ? hostMap.get(hostID) : null
    const hostLabel = hostID ? `操作机 ${host?.name || hostID}${host?.enabled === false ? '（已停用）' : ''}` : ''
    const categoryID = flowNodeCategoryID(node)
    return {
      ...node,
      className: [node.className, 'cat-node', `cat-${categoryID}`].filter(Boolean).join(' '),
      data: {
        ...node.data,
        categoryID,
        run: nodeRun,
        hostLabel,
        onRemove: canRemove ? node.data?.onRemove : null,
        onRerun: onRerun && activeRunID ? onRerun : null,
        rerunDisabled: runState?.status === 'running',
        ...(node.type !== 'conditionNode' && onAddDownstream ? {onAddDownstream} : {}),
        ...(node.type === 'toolNode' ? {
          paramStatus: toolParamStatus(tool, node.data?.params || {}),
          toolMeta: {
            sourceLabel: toolSourceLabel(tool)
          }
        } : {})
      }
    }
  })
}

export function buildDisplayEdges(edges, displayNodes, runState) {
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

function extractRunDetailFromResponse(body) {
  if (!body) return null
  if (body.record) return body
  if (body.data?.record) return body.data
  if (body.detail?.record) return body.detail
  if (body.data?.detail?.record) return body.data.detail
  return null
}

function emptyWorkflow(category = 'global') {
  return {
    id: '',
    name: '',
    description: '',
    version: '1.0.0',
    category,
    tags: [],
    shared_with: [],
    parameters: [],
    nodes: [],
    edges: [],
    confirm: {required: false}
  }
}

function effectiveWorkflowConfirm(workflow, selectedWorkflowID, workflowOptions = []) {
  if (workflow?.confirm?.required) return workflow.confirm
  const selected = (workflowOptions || []).find(item => item.id === selectedWorkflowID || item.id === workflow?.id)
  return selected?.confirm || workflow?.confirm || {required: false}
}

function newToolFlowNode(tool, id, position, onRemove) {
  return {
    id,
    type: 'toolNode',
    data: {
      tool: tool.id,
      name: tool.name || tool.id,
      params: workflowModel.defaultParams(tool.parameters || []),
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
  if (control.type === 'upload') data.upload = workflowModel.defaultUpload()
  if (control.type === 'extract_config') data.extract_config = workflowModel.defaultExtractConfig()
  if (control.type === 'http_request') data.http = workflowModel.defaultHttpRequest()
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

function defaultUpload() {
  return workflowModel.defaultUpload()
}

function defaultExtractConfig() {
  return workflowModel.defaultExtractConfig()
}

function normalizeUploadConfig(upload) {
  return workflowModel.normalizeUploadConfig(upload)
}

function normalizeExtractConfig(extract) {
  return workflowModel.normalizeExtractConfig(extract)
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

export function legacyLoopTargetMap(workflowNodes) {
  const nodeIDs = new Set((workflowNodes || []).map(node => node.id))
  const targetMap = new Map()
  ;(workflowNodes || []).forEach(node => {
    const nodeType = node.type || (node.tool ? 'tool' : node.loop ? 'loop' : '')
    const targetID = nodeType === 'loop' && !node.loop?.tool ? String(node.loop?.target || '') : ''
    if (targetID && nodeIDs.has(targetID)) targetMap.set(targetID, node.id)
  })
  return targetMap
}

export function remapLegacyLoopEdges(edges, targetMap) {
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

export function workflowNodeToFlowNode(node, index, onRemove, workflowNodes = []) {
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
  if (nodeType === 'parallel' || nodeType === 'join' || nodeType === 'loop' || nodeType === 'upload' || nodeType === 'extract_config' || nodeType === 'http_request') {
    const control = controlNodeCatalog.find(item => item.type === nodeType) || {type: nodeType, title: controlNodeTitle(nodeType)}
    const flowNode = newControlFlowNode(control, node.id, {x: 80 + index * 220, y: 120 + (index % 3) * 90}, onRemove)
    flowNode.data.name = node.name || node.id
    if (nodeType === 'loop') {
      flowNode.data.host = node.host || ''
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
    if (nodeType === 'upload') flowNode.data.upload = normalizeUploadConfig(node.upload || defaultUpload())
    if (nodeType === 'extract_config') flowNode.data.extract_config = normalizeExtractConfig(node.extract_config || defaultExtractConfig())
    if (nodeType === 'http_request') flowNode.data.http = workflowModel.normalizeHttpRequestConfig(node.http || workflowModel.defaultHttpRequest())
    return flowNode
  }
  return {
    id: node.id,
    type: 'toolNode',
    data: {
      tool: node.tool,
      name: node.name || node.id,
      host: node.host || '',
      params: node.params || {},
      on_failure: node.on_failure || 'stop',
      onRemove
    },
    position: {x: 80 + index * 220, y: 120 + (index % 3) * 90}
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

export function flowEdgeFromWorkflowEdge(edge, index, nodes = []) {
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
  if (node.type === 'controlNode') return {width: 250, height: 104}
  return {width: 210, height: 74}
}

function conditionCaseLabel(condition, caseID) {
  if (!caseID) return ''
  if (caseID === 'default') return 'default'
  const item = (condition?.cases || []).find(item => item.id === caseID)
  return item ? (item.name || item.id) : caseID
}

function parseJSONList(value) {
  try {
    const parsed = JSON.parse(value || '[]')
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

function formatPreflightMessage(status) {
  const errors = status.errors || []
  if (errors.length === 0) return status.title || '检查通过。'
  return `${status.title || '检查未通过'}\n${errors.map(error => `- ${error}`).join('\n')}`
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

function cloneNodeConfigDraft(node) {
  return {
    ...node,
    data: {
      ...node.data,
      params: node.data.params ? clonePlainValue(node.data.params) : node.data.params,
      condition: node.data.condition ? clonePlainValue(node.data.condition) : node.data.condition,
      loop: node.data.loop ? clonePlainValue(node.data.loop) : node.data.loop,
      upload: node.data.upload ? clonePlainValue(node.data.upload) : node.data.upload,
      extract_config: node.data.extract_config ? clonePlainValue(node.data.extract_config) : node.data.extract_config,
      http: node.data.http ? clonePlainValue(node.data.http) : node.data.http
    }
  }
}

function clonePlainValue(value) {
  try {
    return JSON.parse(JSON.stringify(value))
  } catch {
    return value
  }
}

const FLOW_HISTORY_LIMIT = 60
const FLOW_HISTORY_SETTLE_MS = 320

function useFlowHistory(nodes, edges, setNodes, setEdges) {
  const [availability, setAvailability] = useState({canUndo: false, canRedo: false})
  const stateRef = useRef({past: [], future: [], last: null, pending: null, timer: null, skip: false, skipTimer: null})
  const nodesRef = useRef(nodes)
  const edgesRef = useRef(edges)
  nodesRef.current = nodes
  edgesRef.current = edges

  const syncAvailability = useCallback(() => {
    const state = stateRef.current
    const canUndo = state.past.length > 0
    const canRedo = state.future.length > 0
    setAvailability(current => current.canUndo === canUndo && current.canRedo === canRedo ? current : {canUndo, canRedo})
  }, [])

  const commitPending = useCallback(() => {
    const state = stateRef.current
    if (!state.pending) return
    state.past.push(state.pending)
    if (state.past.length > FLOW_HISTORY_LIMIT) state.past.shift()
    state.future = []
    state.pending = null
  }, [])

  const flushPending = useCallback(() => {
    const state = stateRef.current
    if (state.timer) {
      window.clearTimeout(state.timer)
      state.timer = null
    }
    if (!state.pending) return
    commitPending()
    state.last = {nodes: nodesRef.current, edges: edgesRef.current}
    syncAvailability()
  }, [commitPending, syncAvailability])

  useEffect(() => {
    const state = stateRef.current
    const current = {nodes, edges}
    if (!state.last) {
      state.last = current
      return
    }
    if (state.skip) {
      state.skip = false
      state.last = current
      return
    }
    if (state.last.nodes === nodes && state.last.edges === edges) return
    if (!state.pending) state.pending = state.last
    if (state.timer) window.clearTimeout(state.timer)
    state.timer = window.setTimeout(() => {
      const latest = stateRef.current
      commitPending()
      latest.last = {nodes: nodesRef.current, edges: edgesRef.current}
      latest.timer = null
      syncAvailability()
    }, FLOW_HISTORY_SETTLE_MS)
  }, [nodes, edges, commitPending, syncAvailability])

  useEffect(() => () => {
    const state = stateRef.current
    if (state.timer) window.clearTimeout(state.timer)
    if (state.skipTimer) window.clearTimeout(state.skipTimer)
  }, [])

  const restore = useCallback(snapshot => {
    const state = stateRef.current
    state.skip = true
    if (state.skipTimer) window.clearTimeout(state.skipTimer)
    state.skipTimer = window.setTimeout(() => {
      stateRef.current.skip = false
    }, 0)
    state.last = snapshot
    setNodes(snapshot.nodes)
    setEdges(snapshot.edges)
  }, [setNodes, setEdges])

  const undo = useCallback(() => {
    flushPending()
    const state = stateRef.current
    if (state.past.length === 0) return false
    const snapshot = state.past.pop()
    state.future.push({nodes: nodesRef.current, edges: edgesRef.current})
    if (state.future.length > FLOW_HISTORY_LIMIT) state.future.shift()
    restore(snapshot)
    syncAvailability()
    return true
  }, [flushPending, restore, syncAvailability])

  const redo = useCallback(() => {
    flushPending()
    const state = stateRef.current
    if (state.future.length === 0) return false
    const snapshot = state.future.pop()
    state.past.push({nodes: nodesRef.current, edges: edgesRef.current})
    if (state.past.length > FLOW_HISTORY_LIMIT) state.past.shift()
    restore(snapshot)
    syncAvailability()
    return true
  }, [flushPending, restore, syncAvailability])

  const resetHistory = useCallback((nextNodes, nextEdges) => {
    const state = stateRef.current
    if (state.timer) window.clearTimeout(state.timer)
    if (state.skipTimer) window.clearTimeout(state.skipTimer)
    state.past = []
    state.future = []
    state.pending = null
    state.timer = null
    state.skip = false
    state.skipTimer = null
    state.last = {
      nodes: nextNodes === undefined ? nodesRef.current : nextNodes,
      edges: nextEdges === undefined ? edgesRef.current : nextEdges
    }
    syncAvailability()
  }, [syncAvailability])

  return {canUndo: availability.canUndo, canRedo: availability.canRedo, undo, redo, resetHistory}
}

function workflowDraftSignature(workflow, nodes, edges, fallbackCategory, workflowParamsText, ownerID = '') {
  let parameters = []
  try {
    const parsed = JSON.parse(workflowParamsText || '[]')
    if (Array.isArray(parsed)) parameters = parsed
  } catch {
    return JSON.stringify({ownerID, workflow, nodes: (nodes || []).map(node => ({id: node.id, type: node.type, data: node.data})), edges, workflowParamsText})
  }
  return JSON.stringify({ownerID, draft: workflowModel.buildWorkflowDraft(workflow, nodes, edges, fallbackCategory, parameters)})
}
