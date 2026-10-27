import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {Background, MiniMap, ReactFlow, ViewportPortal, reconnectEdge, useEdgesState, useNodesState} from '@xyflow/react'
import {deleteJSON, fetchJSON, fetchPendingRuns, fetchRunDetail, postJSON, postRunUploadNodeChunked, resolvePendingRun, retryPendingRunCheck, retryRunNotification, rollbackPendingRun} from '../api.js'
import {filterEntries, readableAPIError, summarizeAPIResponse} from '../utils.js'
import {allNodeCategories, controlNodeCatalog, controlNodeTitle, countNodesByCategory, flowNodeCategoryID, isControlReady} from './catalog.js'
import * as workflowModel from './model.js'
import {nodeTypes, normalizeRunStatus, runStatusLabel} from './nodes.jsx'
import {EdgeConfigModal, NodeConfigEditor, NodeConfigModal, ValidationSummary} from './inspectors.jsx'
import {sshHostStatusLabel} from '../SshHostsModal.jsx'
import {CanvasDock, NodePickerPanel} from './picker.jsx'
import WorkflowTransferModal from './WorkflowTransferModal.jsx'
import FileUploadInput from '../FileUploadInput.jsx'
import {DRAFT_WINDOW_SAVED_KEY, clearDraftWindowSaved, clearDraftWindowTransfer, draftWindowSavedRefValue, draftWindowURL, formatDraftWindowTime, readDraftWindowSaved, readDraftWindowTransfer, writeDraftWindowSaved, writeDraftWindowTransfer} from './draftWindow.js'

const MAX_PLATFORM_UPLOAD_BYTES = 20 * 1024 * 1024 * 1024
const RECENT_WORKFLOW_KEY = 'ops.workflowEditor.recentWorkflows'
const RECENT_WORKFLOW_LIMIT = 5
const UNCATEGORIZED_WORKFLOW_CATEGORY = '__uncategorized__'
// 带入的草稿在服务端没有对应版本，用不可能与真实签名相等的哨兵值标记“有未保存修改”。
const DRAFT_WINDOW_TRANSFER_SIGNATURE = '__draftWindowTransfer__'
// 工作流运行级执行目标：follow_nodes 表示沿用各节点自己的 host 配置。
const RUN_TARGET_FOLLOW = 'follow_nodes'
const RUN_TARGET_LOCAL = 'local'

export default function WorkflowEditor({catalog, activeCategory = '', hosts = [], setResult: setAppResult, refreshCatalog, openPluginManager, resultPanel = null, draftWindow = false}) {
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
  const [workflowSearchText, setWorkflowSearchText] = useState('')
  const [workflowNameMenuOpen, setWorkflowNameMenuOpen] = useState(false)
  const [workflowMoreOpen, setWorkflowMoreOpen] = useState(false)
  const [recentWorkflowIDs, setRecentWorkflowIDs] = useState(readRecentWorkflowIDs)
  const [saveModalSettings, setSaveModalSettings] = useState(false)
  const [selectedWorkflowID, setSelectedWorkflowID] = useState('')
  const [workflowOwnerID, setWorkflowOwnerID] = useState('user.workflows')
  const [workflowOwnerLocked, setWorkflowOwnerLocked] = useState(false)
  const [workflowAction, setWorkflowAction] = useState('')
  const [savedDraftSignature, setSavedDraftSignature] = useState(() => {
    const initial = emptyWorkflow()
    return workflowDraftSignature(initial, [], [], '', '[]', 'user.workflows', initial.stages)
  })
  const [selectedNodeID, setSelectedNodeID] = useState('')
  const [selectedEdgeID, setSelectedEdgeID] = useState('')
  const [nodeConfigModalOpen, setNodeConfigModalOpen] = useState(false)
  const [edgeConfigModalOpen, setEdgeConfigModalOpen] = useState(false)
  const [workflowParamsText, setWorkflowParamsText] = useState('[]')
  const [runParamsText, setRunParamsText] = useState('{}')
  const [runTarget, setRunTarget] = useState(RUN_TARGET_FOLLOW)
  const [runTargetOverride, setRunTargetOverride] = useState(false)
  const [runSetupOpen, setRunSetupOpen] = useState(false)
  const [runHistoryOpen, setRunHistoryOpen] = useState(false)
  const [workflowTransferOpen, setWorkflowTransferOpen] = useState(false)
  const [nodeParamsText, setNodeParamsText] = useState('{}')
  const [nodeConfigDraft, setNodeConfigDraft] = useState(null)
  const [editorValidation, setEditorValidation] = useState(null)
  const [saveModalOpen, setSaveModalOpen] = useState(false)
  const [saveModalErrors, setSaveModalErrors] = useState([])
  const [saveModalErrorTitle, setSaveModalErrorTitle] = useState('保存前检查未通过')
  const [mixedStageNotice, setMixedStageNotice] = useState(null)
  const saveModalErrorsRef = useRef(null)
  const pendingWorkflowSelectRef = useRef('')
  const [flowInstance, setFlowInstance] = useState(null)
  const canvasCardRef = useRef(null)
  const runDockNavRef = useRef(null)
  const [stageLayerSize, setStageLayerSize] = useState(null)
  const quickConnectRef = useRef(null)
  const [nodePicker, setNodePicker] = useState({open: false, mode: 'add', position: null, panelPosition: null, connection: null, insertEdge: null, stageId: ''})
  const [nodePickerSearchText, setNodePickerSearchText] = useState('')
  const [pendingInsertEdge, setPendingInsertEdge] = useState(null)
  const [batchStageMenuOpen, setBatchStageMenuOpen] = useState(false)
  const [draftStarted, setDraftStarted] = useState(false)
  const [stageDragID, setStageDragID] = useState('')
  const [stageDropTargetID, setStageDropTargetID] = useState('')
  const [batchStageMove, setBatchStageMove] = useState(null)
  const [canvasSelection, setCanvasSelection] = useState({nodeIDs: [], edgeIDs: []})
  const canvasSelectionRef = useRef(canvasSelection)
  const [nodes, setNodes, onNodesChange] = useNodesState([])
  const [edges, setEdges, onEdgesChange] = useEdgesState([])
  const [stages, setStages] = useState(() => workflowModel.defaultWorkflowStages())
  const flowHistory = useFlowHistory(nodes, edges, setNodes, setEdges, stages, setStages)
  const [canvasRunState, setCanvasRunState] = useState(() => emptyCanvasRunState())
  const [activeRun, setActiveRun] = useState(null)
  const [lastRun, setLastRun] = useState(null)
  const [rerunModal, setRerunModal] = useState(null)
  const [cancellingRunID, setCancellingRunID] = useState('')
  const [uploadNodeFiles, setUploadNodeFiles] = useState({})
  const [uploadProgress, setUploadProgress] = useState(null)
  const [canvasRunDetail, setCanvasRunDetail] = useState(null)
  const [runDockOpen, setRunDockOpen] = useState(false)
  const [runDockNodeID, setRunDockNodeID] = useState('')
  const [runSnapshotNodes, setRunSnapshotNodes] = useState([])
  const [nodeCategoryFocus, setNodeCategoryFocus] = useState('')
  const [pendingRuns, setPendingRuns] = useState([])
  const [recoveryRunID, setRecoveryRunID] = useState('')
  const [recoveryDetail, setRecoveryDetail] = useState(null)
  const [recoveryLoading, setRecoveryLoading] = useState(false)
  const [recoveryAction, setRecoveryAction] = useState('')
  const [recoveryError, setRecoveryError] = useState('')
  const [recoveryNotice, setRecoveryNotice] = useState('')
  const [notificationBusy, setNotificationBusy] = useState('')
  const recoveryRequestRef = useRef(null)
  const recoveryActionRef = useRef(null)
  const notificationRequestRef = useRef(null)
  const [pendingRunsExpanded, setPendingRunsExpanded] = useState(false)
  const [draftWindowTransfer, setDraftWindowTransfer] = useState(null)
  const [draftWindowSaved, setDraftWindowSaved] = useState(null)
  const draftWindowTransferAppliedRef = useRef(false)
  const draftWindowSavedRef = useRef('')
  // 画布删除回调在 handleNodesChange 之后才定义，用 ref 传递避免依赖顺序问题。
  const afterCanvasNodesRemovedRef = useRef(() => {})
  // 已按真实尺寸排过版的节点结构签名，避免每次渲染都重排。
  const measuredLayoutSignatureRef = useRef('')
  const canvasRunVersionRef = useRef(0)

  useEffect(() => () => {
    recoveryRequestRef.current = null
    recoveryActionRef.current = null
    notificationRequestRef.current = null
  }, [])

  // 布局依赖真实卡片尺寸：节点首次渲染后读一次 DOM 高度并重排，避免估算值与渲染高度不一致造成重叠或大空隙。
  useEffect(() => {
    if (!stages.length || !nodes.length || !canvasCardRef.current) return undefined
    const signature = `${stages.map(stage => stage.id).join(',')}|${nodes.map(node => `${node.id}:${node.data?.stageId || ''}`).join(',')}`
    if (measuredLayoutSignatureRef.current === signature) return undefined
    const timer = window.setTimeout(() => {
      const card = canvasCardRef.current
      if (!card) return
      const sizes = new Map()
      card.querySelectorAll('.react-flow__node').forEach(el => {
        // offsetWidth/offsetHeight 不受画布缩放影响，等于卡片在 100% 下的真实尺寸。
        if (el.offsetWidth > 0 && el.offsetHeight > 0) sizes.set(el.getAttribute('data-id'), {width: el.offsetWidth, height: el.offsetHeight})
      })
      if (sizes.size === 0) return
      measuredLayoutSignatureRef.current = signature
      setNodes(current => {
        let changed = false
        const withSizes = current.map(node => {
          const size = sizes.get(node.id)
          if (!size || node.measured?.width === size.width && node.measured?.height === size.height) return node
          changed = true
          return {...node, measured: size}
        })
        if (!changed) return current
        return workflowModel.autoLayoutWorkflowNodes(withSizes, edges, stages)
      })
    }, 60)
    return () => window.clearTimeout(timer)
  }, [nodes, edges, stages, setNodes])

  const clearCanvasRunOverlay = useCallback(() => {
    canvasRunVersionRef.current += 1
    setActiveRun(null)
    setLastRun(null)
    setRerunModal(null)
    setCancellingRunID('')
    setUploadProgress(null)
    setCanvasRunDetail(null)
    setRunDockOpen(false)
    setRunDockNodeID('')
    setRunSnapshotNodes([])
    setCanvasRunState(current => current.status === 'idle' ? current : emptyCanvasRunState())
  }, [])

  // 首次测得的尺寸要写回受控 nodes：React Flow 只认 nodes 属性里的 measured，光回传 dimensions 变更会让节点一直隐藏、连线不渲染。
  const handleNodesChange = useCallback(changes => {
    if (shouldClearCanvasRunOverlayForNodeChanges(changes)) {
      clearCanvasRunOverlay()
    }
    const measured = changes.filter(change => change.type === 'dimensions' && change.resizing !== true && change.dimensions?.width && change.dimensions?.height)
    if (measured.length > 0) {
      const sizes = new Map(measured.map(change => [change.id, change.dimensions]))
      setNodes(current => {
        let changed = false
        const next = current.map(node => {
          const size = sizes.get(node.id)
          if (!size || node.measured?.width === size.width && node.measured?.height === size.height) return node
          changed = true
          return {...node, measured: {width: size.width, height: size.height}}
        })
        return changed ? next : current
      })
    }
    const rest = changes.filter(change => change.type !== 'dimensions')
    if (rest.length > 0) {
      onNodesChange(rest)
      // React Flow 自己按 Backspace 删除节点时不会走 removeNode，这里补上阶段内重排和选中态清理。
      const removed = rest.filter(change => change.type === 'remove').map(change => change.id)
      if (removed.length > 0) afterCanvasNodesRemovedRef.current(removed)
    }
  }, [clearCanvasRunOverlay, onNodesChange, setNodes])

  const handleEdgesChange = useCallback(changes => {
    if (changes.some(change => change.type === 'remove')) {
      clearCanvasRunOverlay()
    }
    onEdgesChange(changes)
  }, [clearCanvasRunOverlay, onEdgesChange])

  // onSelectionChange 会在同一帧里连续触发，先按最新快照过滤成真正的变化再写状态，避免大画布多选时成倍重渲染。
  const handleCanvasSelectionChange = useCallback(({nodes: selectedNodes = [], edges: selectedEdges = []}) => {
    const nodeIDs = selectedNodes.map(node => node.id)
    const edgeIDs = selectedEdges.map(edge => edge.id)
    const current = canvasSelectionRef.current
    if (sameIDList(current.nodeIDs, nodeIDs) && sameIDList(current.edgeIDs, edgeIDs)) return
    const next = {nodeIDs, edgeIDs}
    canvasSelectionRef.current = next
    setCanvasSelection(next)
  }, [])

  const clearCanvasSelection = useCallback(() => {
    setBatchStageMenuOpen(false)
    if (canvasSelectionRef.current.nodeIDs.length === 0 && canvasSelectionRef.current.edgeIDs.length === 0) return
    const next = {nodeIDs: [], edgeIDs: []}
    canvasSelectionRef.current = next
    setCanvasSelection(next)
    // 走 React Flow 自己的 onChange 才能同步内部节点状态，只改外部数组会导致下次点击无法重新选中。
    onNodesChange(nodes.filter(node => node.selected).map(node => ({type: 'select', id: node.id, selected: false})))
    onEdgesChange(edges.filter(edge => edge.selected).map(edge => ({type: 'select', id: edge.id, selected: false})))
  }, [nodes, edges, onNodesChange, onEdgesChange])

  useEffect(() => {
    canvasSelectionRef.current = canvasSelection
  }, [canvasSelection])

  useEffect(() => {
    if (!batchStageMenuOpen) return undefined
    const closeMenu = event => {
      if (!event.target?.closest?.('.workflowStageBatchMenuWrap')) setBatchStageMenuOpen(false)
    }
    document.addEventListener('mousedown', closeMenu)
    return () => document.removeEventListener('mousedown', closeMenu)
  }, [batchStageMenuOpen])

  useEffect(() => {
    if (!runDockOpen || !runDockNodeID) return
    runDockNavRef.current?.querySelector('button.active')?.scrollIntoView?.({block: 'nearest'})
  }, [runDockOpen, runDockNodeID])

  // 多选后删除、撤销或切换工作流都会让节点消失，批量入口只取仍存在的 ID。
  const canvasSelectedNodeIDs = useMemo(() => {
    const known = new Set(nodes.map(node => node.id))
    return canvasSelection.nodeIDs.filter(id => known.has(id))
  }, [canvasSelection, nodes])

  useEffect(() => {
    if (!selectedWorkflowID) return
    if (workflowOptions.some(item => item.id === selectedWorkflowID)) {
      pendingWorkflowSelectRef.current = ''
      return
    }
    if (pendingWorkflowSelectRef.current === selectedWorkflowID) return
    setSelectedWorkflowID('')
    setNodes([])
    setEdges([])
    setStages([])
    flowHistory.resetHistory([], [], [])
  }, [selectedWorkflowID, workflowOptions, setNodes, setEdges, flowHistory.resetHistory])

  const selectedNode = useMemo(() => nodes.find(node => node.id === selectedNodeID), [nodes, selectedNodeID])
  const selectedEdge = useMemo(() => edges.find(edge => edge.id === selectedEdgeID), [edges, selectedEdgeID])
  const configNode = nodeConfigDraft || selectedNode
  const toolMap = useMemo(() => new Map((catalog.tools || []).map(tool => [tool.id, tool])), [catalog.tools])
  const selectedTool = useMemo(() => (catalog.tools || []).find(tool => tool.id === configNode?.data.tool), [catalog.tools, configNode])
  const selectedLoopTool = useMemo(() => (catalog.tools || []).find(tool => tool.id === configNode?.data.loop?.tool), [catalog.tools, configNode])
  const nodePickerToolOptions = useMemo(() => filterEntries(toolOptions, nodePickerSearchText, ''), [toolOptions, nodePickerSearchText])
  const missingToolEntries = useMemo(() => missingToolDependencies(catalog, nodePickerSearchText), [catalog, nodePickerSearchText])
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
  const filteredWorkflowOptions = useMemo(() => {
    const keyword = workflowSearchText.trim().toLowerCase()
    return workflowOptions.filter(item => {
      if (workflowCategoryFilter && !workflowVisibleCategories.get(item.id)?.has(workflowCategoryFilter)) return false
      if (!keyword) return true
      return String(item.name || '').toLowerCase().includes(keyword) || String(item.id || '').toLowerCase().includes(keyword)
    })
  }, [workflowOptions, workflowCategoryFilter, workflowVisibleCategories, workflowSearchText])
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
  const mappingSources = useMemo(() => workflowModel.buildMappingSources(workflowParameters, selectedNodeID, nodes, edges, catalog.tools || [], stages), [workflowParameters, selectedNodeID, nodes, edges, catalog.tools, stages])
  const selectedNodeSources = mappingSources
  const workflowConfirm = effectiveWorkflowConfirm(workflow, selectedWorkflowID, catalog.workflows || [])
  const canDeleteWorkflow = Boolean(selectedWorkflowID) && !workflowOwnerLocked
  const rerunnableRun = activeRun || lastRun
  const activeRunID = rerunnableRun?.id || ''
  const activeRunStatus = normalizeRunStatus(canvasRunState.status === 'running' ? 'running' : activeRun?.status || lastRun?.status || canvasRunState.status)
  const isActiveRunRunning = activeRunStatus === 'running'
  const activeRunHasRecovery = Boolean(canvasRunDetail?.record?.pending) || (canvasRunDetail?.record?.attempts?.length || 0) > 0 || activeRunStatus === 'pending' || activeRunStatus === 'rolled_back'
  const isEditingCanvas = !isActiveRunRunning
  useEffect(() => {
    if (isEditingCanvas) return
    canvasSelectionRef.current = {nodeIDs: [], edgeIDs: []}
    setCanvasSelection(canvasSelectionRef.current)
  }, [isEditingCanvas])
  const runVisible = canvasRunState.status !== 'idle' || Boolean(canvasRunDetail?.record) || Boolean(activeRunID)
  const sortedPendingRuns = useMemo(() => {
    const currentID = selectedWorkflowID || workflow.id
    return [...pendingRuns].sort((left, right) => {
      const leftCurrent = left.target === currentID ? 1 : 0
      const rightCurrent = right.target === currentID ? 1 : 0
      if (leftCurrent !== rightCurrent) return rightCurrent - leftCurrent
      return new Date(right.started_at || 0) - new Date(left.started_at || 0)
    })
  }, [pendingRuns, selectedWorkflowID, workflow.id])
  const currentWorkflowPendingRuns = useMemo(() => {
    const currentID = selectedWorkflowID || workflow.id
    if (!currentID) return []
    return sortedPendingRuns.filter(item => item.target === currentID)
  }, [sortedPendingRuns, selectedWorkflowID, workflow.id])
  const runDockItem = useMemo(() => findRunLogItem(canvasRunDetail?.logs?.items || [], runDockNodeID), [canvasRunDetail, runDockNodeID])
  const runDockNodes = useMemo(() => {
    const source = runSnapshotNodes.length > 0 ? runSnapshotNodes : nodes
    const known = new Map(source.map(node => [node.id, node]))
    return Object.keys(canvasRunState.nodes || {}).map(nodeID => known.get(nodeID) || {id: nodeID, data: {name: nodeID}})
  }, [runSnapshotNodes, nodes, canvasRunState.nodes])
  const runMachineLabel = describeRunMachines(canvasRunDetail?.record, runSnapshotNodes, runHostOptions, runTargetLabel)
  const displayNodes = useMemo(() => buildDisplayNodes(
    nodes,
    canvasRunState,
    catalog.tools || [],
    isEditingCanvas,
    isActiveRunRunning || activeRunHasRecovery ? null : rerunWorkflowNode,
    activeRunID,
    hosts,
    runDockNodeID
  ), [nodes, canvasRunState, catalog.tools, isEditingCanvas, isActiveRunRunning, activeRunHasRecovery, activeRunID, rerunWorkflowNode, hosts, runDockNodeID])
  const displayEdges = useMemo(() => buildDisplayEdges(edges, displayNodes, canvasRunState), [edges, displayNodes, canvasRunState])
  const workflowStageLayouts = useMemo(() => workflowModel.stageLayout(nodes, stages).stages, [nodes, stages])
  const workflowStageAddPosition = useMemo(() => {
    if (!workflowStageLayouts.length) return null
    const last = workflowStageLayouts[workflowStageLayouts.length - 1]
    return {x: last.x + last.width + 24, y: 72}
  }, [workflowStageLayouts])
  const categoryCounts = useMemo(() => countNodesByCategory(nodes), [nodes])
  const currentDraftSignature = useMemo(
    () => workflowDraftSignature(workflow, nodes, edges, '', workflowParamsText, workflowOwnerID, stages),
    [workflow, nodes, edges, workflowParamsText, workflowOwnerID, stages]
  )
  const hasUnsavedChanges = currentDraftSignature !== savedDraftSignature

  useEffect(() => {
    setWorkflowCategoryFilter(activeCategory || '')
  }, [activeCategory])

  useEffect(() => {
    let ignore = false
    fetchPendingRuns(0)
      .then(body => {
        if (ignore) return
        setPendingRuns(normalizePendingRunSummaries(body?.data?.runs))
      })
      .catch(() => {})
    return () => { ignore = true }
  }, [selectedWorkflowID])

  useEffect(() => {
    if (pendingRuns.length === 0) return undefined
    const timer = window.setInterval(() => { refreshPendingRuns() }, 10000)
    return () => window.clearInterval(timer)
  }, [pendingRuns.length])

  useEffect(() => {
    if (!draftWindow || draftWindowTransferAppliedRef.current) return
    draftWindowTransferAppliedRef.current = true
    const transfer = readDraftWindowTransfer()
    if (transfer) applyDraftWindowTransfer(transfer)
    // 草稿窗口不自动加载上次使用的工作流：带入内容或空白草稿都由用户决定。
  }, [])

  // 独立草稿窗口保存后写回标记，主窗口靠 storage 事件感知；提示只做提醒，不自动覆盖当前画布。
  useEffect(() => {
    if (draftWindow) return undefined
    function handleStorage(event) {
      if (event.key && event.key !== DRAFT_WINDOW_SAVED_KEY) return
      const saved = readDraftWindowSaved()
      if (!saved) return
      const marker = draftWindowSavedRefValue(saved)
      if (!marker || draftWindowSavedRef.current === marker) return
      draftWindowSavedRef.current = marker
      setDraftWindowSaved(saved)
    }
    window.addEventListener('storage', handleStorage)
    handleStorage({key: DRAFT_WINDOW_SAVED_KEY})
    return () => window.removeEventListener('storage', handleStorage)
  }, [])

  // 进入工作流管理时保持空白画布，由用户从顶部选择已保存工作流或新建草稿。

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
    if (!flowInstance) return
    if (nodes.length === 0 && stages.length > 0) {
      fitCanvasViewAfterLayout({nodes: [], stages})
    }
  }, [flowInstance])

  // viewport portal 在 DOM 里没有尺寸，阶段背景层要显式铺满画布，否则百分比和弹性宽度会按内容收缩。
  useEffect(() => {
    const element = canvasCardRef.current
    if (!element) return undefined
    const measure = () => {
      const flowElement = element.querySelector('.react-flow')
      const target = flowElement || element
      setStageLayerSize(current => {
        const next = {width: target.clientWidth, height: target.clientHeight}
        return current && current.width === next.width && current.height === next.height ? current : next
      })
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

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
    closeNodePicker()
    // 运行结果可见时，点击节点用于定位日志；其余情况单击只选中节点，避免配置抽屉挡住画布。
    if (runVisible && canvasRunState.nodes?.[nodeID]) {
      openRunDockNode(nodeID)
    }
  }

  function handleCanvasNodeDoubleClick(nodeID) {
    if (!isEditingCanvas || isActiveRunRunning) return
    openNodeConfigModal(nodeID)
  }

  function openRunDockNode(nodeID) {
    setRunDockOpen(true)
    setRunDockNodeID(nodeID)
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
    if (activeRunHasRecovery) {
      setResult({message: '该运行已有恢复记录，请使用“恢复与通知”入口；需要重新执行变更时请启动新的运行。'})
      return
    }
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
    if (activeRunHasRecovery) {
      setRerunModal(null)
      setResult({message: '该运行已有恢复记录，请使用“恢复与通知”入口；需要重新执行变更时请启动新的运行。'})
      return
    }
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
    const rerunState = buildRerunCanvasRunState(nodes, edges, canvasRunState, nodeID, stages, workflow.recovery, catalog.tools || [], canvasRunDetail?.record?.snapshot?.workflow?.nodes)
    const rerunTarget = rerunnableRun?.target || workflow.id
    setRerunModal(null)
    setRunSnapshotNodes(nodes)
    setRunDockOpen(true)
    setRunDockNodeID(nodeID)
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
      const conflict = workflowModel.validateStageDraft(nodes, [...edges, params], stages)[0]
      if (conflict) {
        setResult({message: conflict})
        return
      }
      clearCanvasRunOverlay()
      if (quickConnectRef.current) quickConnectRef.current.completed = true
      setEdges(current => appendFlowEdge(current, params, nodes))
    },
    [clearCanvasRunOverlay, isEditingCanvas, nodes, edges, setEdges, stages]
  )

  async function refreshPendingRuns() {
    try {
      const body = await fetchPendingRuns(0)
      setPendingRuns(normalizePendingRunSummaries(body?.data?.runs))
    } catch {
      return []
    }
  }

  async function openRecoveryDrawer(runID) {
    if (!runID) return
    const request = {runID}
    recoveryRequestRef.current = request
    recoveryActionRef.current = null
    notificationRequestRef.current = null
    setWorkflowNameMenuOpen(false)
    setWorkflowMoreOpen(false)
    setRunHistoryOpen(false)
    setRecoveryRunID(runID)
    setRecoveryDetail(null)
    setRecoveryAction('')
    setNotificationBusy('')
    setRecoveryLoading(true)
    setRecoveryError('')
    setRecoveryNotice('')
    try {
      const detail = await fetchRunDetail(runID)
      if (recoveryRequestRef.current !== request) return
      const data = detail?.data || detail
      if (data?.record?.id !== runID) throw new Error('运行记录与当前恢复入口不一致，请重新打开。')
      setRecoveryDetail(data)
    } catch (err) {
      if (recoveryRequestRef.current !== request) return
      setRecoveryError(readableAPIError(err, '读取待处理运行失败。'))
    } finally {
      if (recoveryRequestRef.current === request) setRecoveryLoading(false)
    }
  }

  function closeRecoveryDrawer() {
    if (recoveryActionRef.current) return
    recoveryRequestRef.current = null
    notificationRequestRef.current = null
    setRecoveryRunID('')
    setRecoveryDetail(null)
    setRecoveryError('')
    setRecoveryNotice('')
    setRecoveryLoading(false)
    setNotificationBusy('')
  }

  function syncRecoveredRunOverlay(detail) {
    const record = detail?.record
    if (!record?.id || record.id !== activeRunID) return
    const status = normalizeRunStatus(record.status)
    setCanvasRunDetail(detail)
    setCanvasRunState(buildCanvasRunStateFromDetail(detail, runSnapshotNodes.length > 0 ? runSnapshotNodes : nodes))
    setActiveRun(current => current && current.id === record.id ? {...current, status} : current)
    setLastRun(current => current && current.id === record.id ? {...current, status} : current)
  }

  async function executeRecoveryAction(action, note = '') {
    const runID = recoveryRunID
    const request = recoveryRequestRef.current
    if (!runID || request?.runID !== runID || recoveryActionRef.current || notificationRequestRef.current) return
    if (recoveryDetail?.record?.id !== runID) {
      setRecoveryError('运行记录与当前恢复入口不一致，请重新打开后操作。')
      return
    }
    if (action === 'rollback') {
      const backups = recoveryDetail?.record?.backups || []
      const lines = backups.map(backup => `${backup.node_id || backup.tool || '变更节点'}：${backup.target_path || '未知路径'}${backup.backup_path ? `（备份 ${backup.backup_path}）` : '（无备份）'}`)
      const message = `确认按本次运行的备份回滚以下配置？\n\n${lines.join('\n') || '（无备份记录）'}\n\n回滚前会校验备份和目标文件，并重启关联服务后再次健康检查。`
      if (!window.confirm(message)) return
    }
    if (action === 'resolve' && !window.confirm('结束处理会保留当前已变更的配置，仅把本次运行标记为失败并关闭待处理入口，是否继续？')) return
    const operation = {request}
    recoveryActionRef.current = operation
    setRecoveryAction(action)
    setRecoveryError('')
    setRecoveryNotice('')
    try {
      const body = action === 'check'
        ? await retryPendingRunCheck(runID)
        : action === 'rollback'
          ? await rollbackPendingRun(runID)
          : await resolvePendingRun(runID, note)
      if (recoveryRequestRef.current !== request || recoveryActionRef.current !== operation) return
      const detail = body?.data || body
      if (detail?.record && detail.record.id !== runID) throw new Error('恢复响应的运行记录不一致，请重新读取运行结果。')
      if (detail?.record) setRecoveryDetail(detail)
      const status = normalizeRunStatus(detail?.record?.status)
      if (status === 'succeeded') {
        setRecoveryNotice('重试检查通过，运行已标记为成功；原始失败步骤和全部 attempt 证据仍然保留。')
      } else if (status === 'rolled_back') {
        setRecoveryNotice('配置已回滚，服务已恢复；原始变更失败的结论仍保留在运行记录中。')
      } else if (status === 'failed') {
        setRecoveryNotice('已结束处理，运行标记为失败，请继续人工跟进未恢复的变更。')
      } else {
        setRecoveryNotice('恢复动作已执行，运行仍为待处理，请查看最新 attempt 证据后继续处理。')
      }
      if (detail) syncRecoveredRunOverlay(detail)
      await refreshPendingRuns()
    } catch (err) {
      if (recoveryRequestRef.current !== request || recoveryActionRef.current !== operation) return
      const detail = err.body?.data || err.body?.detail
      if (detail?.record?.id === runID) {
        setRecoveryDetail(detail)
        syncRecoveredRunOverlay(detail)
      }
      setRecoveryError(readableAPIError(err, '恢复动作执行失败。'))
      await refreshPendingRuns()
    } finally {
      if (recoveryActionRef.current === operation) {
        recoveryActionRef.current = null
        if (recoveryRequestRef.current === request) setRecoveryAction('')
      }
    }
  }

  async function retryRecoveryNotification(index) {
    const runID = recoveryRunID
    const request = recoveryRequestRef.current
    if (!runID || request?.runID !== runID || notificationRequestRef.current || recoveryActionRef.current) return
    if (recoveryDetail?.record?.id !== runID) {
      setRecoveryError('运行记录与当前恢复入口不一致，请重新打开后操作。')
      return
    }
    const operation = {request}
    notificationRequestRef.current = operation
    setNotificationBusy(String(index))
    setRecoveryNotice('')
    try {
      const body = await retryRunNotification(runID, index)
      if (recoveryRequestRef.current !== request || notificationRequestRef.current !== operation) return
      const detail = body?.data || body
      if (detail?.record && detail.record.id !== runID) throw new Error('通知响应的运行记录不一致，请重新读取运行结果。')
      if (detail?.record) setRecoveryDetail(detail)
      setRecoveryError('')
      setRecoveryNotice('通知已重新发送，未重新执行任何配置变更。')
    } catch (err) {
      if (recoveryRequestRef.current !== request || notificationRequestRef.current !== operation) return
      const detail = err.body?.data || err.body?.detail
      if (detail?.record?.id === runID) setRecoveryDetail(detail)
      setRecoveryError(readableAPIError(err, '通知重试失败。'))
    } finally {
      if (notificationRequestRef.current === operation) {
        notificationRequestRef.current = null
        if (recoveryRequestRef.current === request) setNotificationBusy('')
      }
    }
  }

  // 服务端加载和独立草稿窗口带入的草稿走同一段装填逻辑，差别只在是否记入最近使用。
  function applyWorkflowConfig(config, source, options = {}) {
    const ownerID = source.plugin_id || 'user.workflows'
    const ownerLocked = Boolean(source.type) && source.type !== 'plugin'
    setSaveModalOpen(false)
    setSaveModalErrors([])
    setWorkflowOwnerID(ownerID)
    setWorkflowOwnerLocked(ownerLocked)
      setWorkflowParamsText(JSON.stringify(config.parameters || [], null, 2))
    setRunParamsText(JSON.stringify(workflowModel.defaultParams(config.parameters || []), null, 2))
    const workflowNodes = config.nodes || []
    const workflowStages = workflowModel.normalizeWorkflowStages(config.stages || [])
    const legacyTargetMap = legacyLoopTargetMap(workflowNodes)
    const legacyTargets = new Set(legacyTargetMap.keys())
    const flowNodes = workflowNodes
      .filter(node => !legacyTargets.has(node.id))
      .map((node, index) => workflowNodeToFlowNode(node, index, removeNode, workflowNodes))
    // 阶段工作流里缺 stage_id 的节点不能被悄悄塞进第一个阶段：直接标出待归属节点，由用户决定放在哪里。
    const unassignedNodes = workflowStages.length > 0
      ? flowNodes.filter(node => !workflowStages.some(stage => stage.id === workflowModel.nodeStageID(node)))
      : []
    const stagedNodes = workflowStages.length > 0
      ? workflowModel.normalizeStageNodeOrders(flowNodes, workflowStages)
      : flowNodes
    const flowEdges = remapLegacyLoopEdges(config.edges || [], legacyTargetMap).map((edge, index) => flowEdgeFromWorkflowEdge(edge, index, stagedNodes))
    const layoutNodes = workflowStages.length > 0
      ? workflowModel.autoLayoutWorkflowNodes(stagedNodes, flowEdges, workflowStages)
      : workflowModel.autoLayoutNodes(stagedNodes, flowEdges)
    setWorkflow({...config, stages: workflowStages.length > 0 ? workflowStages : undefined})
    setStages(workflowStages)
    setNodes(layoutNodes)
    setEdges(flowEdges)
    flowHistory.resetHistory(layoutNodes, flowEdges, workflowStages)
    setSavedDraftSignature(workflowDraftSignature(config, layoutNodes, flowEdges, '', JSON.stringify(config.parameters || []), ownerID, workflowStages))
    setSelectedNodeID('')
    setSelectedEdgeID('')
    setNodeConfigModalOpen(false)
    setNodeConfigDraft(null)
    setEdgeConfigModalOpen(false)
    setUploadNodeFiles({})
    closeNodePicker()
    setWorkflowNameMenuOpen(false)
    setWorkflowSearchText('')
    fitCanvasViewAfterLayout({nodes: layoutNodes, stages: workflowStages})
    setMixedStageNotice(unassignedNodes.length > 0 ? {
      workflowID: options.id || config.id || '',
      nodeIDs: unassignedNodes.map(node => node.id),
      names: unassignedNodes.map(node => node.data?.name || node.id)
    } : null)
    return {ownerID, ownerLocked, unassignedNodes}
  }

  async function loadWorkflow(id, options = {}) {
    if (!id) return
    if (!options.silent && id !== selectedWorkflowID && !options.force) {
      if (!confirmDiscardDraft()) return
    }
    clearCanvasRunOverlay()
    setWorkflowAction('loading')
    if (!options.silent) setResult({message: '加载工作流...'})
    try {
      const body = await fetchJSON(`/api/workflows/${encodeURIComponent(id)}`)
      const config = body.data.Config || body.data.config
      const source = body.data.Source || body.data.source || {}
      applyWorkflowConfig(config, source, {id})
      setSelectedWorkflowID(id)
      setDraftStarted(true)
      // 独立草稿窗口只借用画布，不写主窗口的“最近使用”。
      if (!draftWindow) {
        setRecentWorkflowIDs(pushRecentWorkflowID(id))
      }
      if (!options.silent) setResult({message: `已加载工作流 ${id}`})
    } catch (err) {
      setMixedStageNotice(null)
      if (!options.silent) setResult({message: readableAPIError(err, '加载工作流失败。'), response: err.body})
    } finally {
      setWorkflowAction('')
    }
  }

  // 独立草稿窗口把主窗口的当前草稿装进画布；带入内容在服务端没有对应版本，按“未保存修改”处理。
  function applyDraftWindowTransfer(transfer) {
    clearCanvasRunOverlay()
    const applied = applyWorkflowConfig(transfer.draft, {plugin_id: transfer.ownerID, type: transfer.ownerLocked ? 'builtin' : 'plugin'}, {id: transfer.id})
    setSelectedWorkflowID(transfer.id)
    setDraftStarted(true)
    setSavedDraftSignature(DRAFT_WINDOW_TRANSFER_SIGNATURE)
    setEditorValidation(null)
    setDraftWindowTransfer({id: transfer.id, name: transfer.name, ts: transfer.ts, dirty: transfer.dirty, ownerID: applied.ownerID})
    setResult({message: `已从主窗口带入草稿${transfer.name ? `：${transfer.name}` : ''}，修改后可直接保存。`})
  }

  function bringInDraftWindowTransfer() {
    const transfer = readDraftWindowTransfer()
    if (!transfer) {
      setDraftWindowTransfer(null)
      setEditorNotice('主窗口没有可带入的草稿，已清除带入标记。')
      return
    }
    if (hasUnsavedChanges && !window.confirm('重新带入会覆盖草稿窗口当前内容，是否继续？')) return
    applyDraftWindowTransfer(transfer)
  }

  function dismissDraftWindowTransfer() {
    clearDraftWindowTransfer()
    setDraftWindowTransfer(null)
    setEditorNotice('已停止提示主窗口带入的草稿，画布内容保留。')
  }

  // 运行期间继续编辑：草稿经同源 localStorage 交给新窗口，运行仍使用主窗口启动时的快照。
  function openDraftWindow() {
    const check = preflightWorkflowDraft('save')
    const written = writeDraftWindowTransfer({
      id: check.draft.id || workflow.id || '',
      name: check.draft.name || workflow.name || '',
      ownerID: workflowOwnerID,
      ownerLocked: workflowOwnerLocked,
      dirty: hasUnsavedChanges,
      draft: check.draft
    })
    if (!written) {
      setResult({message: '浏览器存储不可用，无法把当前草稿交给独立窗口。'})
      return
    }
    // 固定窗口名，重复点击时复用同一个窗口，避免开出一堆标签。
    const target = window.open(draftWindowURL(window.location.href), 'opsWorkflowDraftWindow')
    if (!target) {
      setResult({message: '浏览器拦截了独立草稿窗口，请允许本站弹出窗口后重试。'})
      return
    }
    setResult({message: `已在新窗口打开草稿${check.draft.name ? `：${check.draft.name}` : ''}，本次运行不受影响。`})
  }

  function reloadSavedDraftWindowWorkflow(saved) {
    if (!saved?.id) return
    if (!confirmDiscardDraft()) return
    clearDraftWindowSaved()
    setDraftWindowSaved(null)
    loadWorkflow(saved.id)
  }

  // 标记只用于这次保存后的提醒，处理完就清掉，避免每次打开主窗口都再弹一遍。
  function dismissSavedDraftWindowWorkflow() {
    clearDraftWindowSaved()
    setDraftWindowSaved(null)
  }

  function updateWorkflowRecovery(patch) {
    clearCanvasRunOverlay()
    setEditorValidation(null)
    setWorkflow(current => {
      const next = pruneWorkflowRecovery({...(current.recovery || {}), ...patch})
      if (next) return {...current, recovery: next}
      const {recovery, ...rest} = current
      return rest
    })
  }

  function updateWorkflowNotification(patch) {
    clearCanvasRunOverlay()
    setEditorValidation(null)
    setWorkflow(current => {
      const recovery = current.recovery || {}
      const notification = {...(recovery.notification || {}), ...patch}
      return {...current, recovery: pruneWorkflowRecovery({...recovery, notification})}
    })
  }

  function toggleWorkflowNotificationEvent(event) {
    clearCanvasRunOverlay()
    setEditorValidation(null)
    setWorkflow(current => {
      const recovery = current.recovery || {}
      const configured = recovery.notification?.events || []
      const baseline = configured.length === 0 ? NOTIFICATION_EVENTS : configured
      const nextEvents = baseline.includes(event)
        ? baseline.filter(item => item !== event)
        : [...baseline, event]
      const notification = {...(recovery.notification || {}), events: nextEvents.length === NOTIFICATION_EVENTS.length ? [] : nextEvents}
      return {...current, recovery: pruneWorkflowRecovery({...recovery, notification})}
    })
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
    if (isActiveRunRunning) return
    if (!confirmDiscardDraft()) return
    clearCanvasRunOverlay()
    setSaveModalOpen(false)
    setSaveModalErrors([])
    const next = emptyWorkflow(workflowCategoryFilter || activeCategory || 'global')
    setWorkflowNameMenuOpen(false)
    setWorkflowMoreOpen(false)
    setWorkflow(next)
    setWorkflowParamsText('[]')
    setRunParamsText('{}')
    setNodes([])
    setEdges([])
    setStages(next.stages)
    flowHistory.resetHistory([], [], next.stages)
    setSelectedWorkflowID('')
    setWorkflowOwnerID('user.workflows')
    setWorkflowOwnerLocked(false)
    setSelectedNodeID('')
    setSelectedEdgeID('')
    setNodeConfigModalOpen(false)
    setNodeConfigDraft(null)
    setEdgeConfigModalOpen(false)
    setEditorValidation(null)
    setSavedDraftSignature(workflowDraftSignature(next, [], [], '', '[]', 'user.workflows', next.stages))
    setDraftStarted(true)
    closeNodePicker()
    fitCanvasViewAfterLayout({nodes: [], stages: next.stages})
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

  // 删除后把仍在画布上的节点按阶段重新编号，阶梯阶段里被删节点留下的空位不会让后续节点乱序。
  const afterCanvasNodesRemoved = useCallback(removed => {
    if (!stages.length) return
    setNodes(current => workflowModel.autoLayoutWorkflowNodes(workflowModel.normalizeStageNodeOrders(current, stages), [], stages))
    setSelectedNodeID(current => removed.includes(current) ? '' : current)
  }, [setNodes, stages])
  afterCanvasNodesRemovedRef.current = afterCanvasNodesRemoved

  const removeNode = useCallback(id => {
    if (!isEditingCanvas) return
    clearCanvasRunOverlay()
    setNodes(current => {
      const next = current.filter(node => node.id !== id)
      if (!stages.length) return next
      return workflowModel.autoLayoutWorkflowNodes(workflowModel.normalizeStageNodeOrders(next, stages), [], stages)
    })
    setEdges(current => current.filter(edge => edge.source !== id && edge.target !== id))
    setResult({message: `已移除节点 ${id}`})
    setSelectedNodeID(current => current === id ? '' : current)
    setNodeConfigModalOpen(false)
    setNodeConfigDraft(null)
    setEdgeConfigModalOpen(false)
    setSelectedEdgeID('')
  }, [isEditingCanvas, clearCanvasRunOverlay, setEdges, setNodes, setResult, stages])

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

  function newNodeStagePlacement(options = {}) {
    if (stages.length === 0) return {}
    const stageIDs = new Set(stages.map(stage => stage.id))
    const insertEdge = options.insertEdge || null
    const targetNode = insertEdge ? nodes.find(node => node.id === insertEdge.target) : null
    if (targetNode && stageIDs.has(targetNode.data.stageId)) {
      const targetOrder = Number(targetNode.data.order) || workflowModel.nextStageOrder(nodes, targetNode.data.stageId)
      return {stageId: targetNode.data.stageId, order: targetOrder, insertBefore: true}
    }
    if (options.stageId && stageIDs.has(options.stageId)) {
      return {stageId: options.stageId, order: workflowModel.nextStageOrder(nodes, options.stageId)}
    }
    const sourceNode = options.connection?.source
      ? nodes.find(node => node.id === options.connection.source)
      : selectedNode
    if (sourceNode && stageIDs.has(sourceNode.data.stageId)) {
      return {stageId: sourceNode.data.stageId, order: workflowModel.nextStageOrder(nodes, sourceNode.data.stageId)}
    }
    const fallbackStageId = stages[0].id
    return {stageId: fallbackStageId, order: workflowModel.nextStageOrder(nodes, fallbackStageId)}
  }

  function addNodeAndMaybeConnect(nextNode, options = {}) {
    setDraftStarted(true)
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
    const placement = newNodeStagePlacement(options)
    if (placement.stageId) {
      nextNode.data = {...nextNode.data, stageId: placement.stageId, order: placement.order}
    }
    // 条件分支跨阶段时新节点可能落到条件之前，先尝试改放到更靠后的阶段，避免一创建就触发阶段顺序冲突。
    if (placement.stageId) {
      const probeNode = {...nextNode, data: {...nextNode.data, stageId: placement.stageId, order: placement.order}}
      // 插入连线时新节点以数组末尾的身份参与建边，换阶段只是改写它的数据。
      const validateWith = candidateItems => {
        // 候选节点始终是数组末尾；从节点拉出的新节点还没有入边，校验前要先补上起始节点这条依赖才算顺序。
        const nextNodes = workflowModel.normalizeStageNodeOrders(candidateItems, stages)
        const nextEdges = insertEdge ? insertNodeIntoEdge(edges, insertEdge, nextNodes[nextNodes.length - 1], nextNodes) : edges
        const dependencyEdges = pendingConnection?.source
          ? [...nextEdges, {id: '__probe__', source: pendingConnection.source, target: probeNode.id, sourceHandle: pendingConnection.sourceHandle || undefined}]
          : nextEdges
        return workflowModel.validateStageDraft(nextNodes, dependencyEdges, stages)
      }
      const rejected = validateWith([...nodes, probeNode])[0] || ''
      if (rejected) {
        const relocated = workflowModel.relocateNodeStage(nodes, stages, probeNode, {
          afterNodeID: pendingConnection?.source || '',
          limitID: placement.stageId,
          validate: validateWith
        })
        if (!relocated) {
          setResult({message: rejected})
          closeNodePicker()
          return
        }
        placement.stageId = relocated.stageId
        placement.order = relocated.order
        placement.insertBefore = false
      }
    }
    setNodes(current => {
      let next = current
      if (placement.insertBefore) {
        next = next.map(node => {
          if (node.data?.stageId !== placement.stageId) return node
          const order = Number(node.data.order) || 0
          if (order < placement.order) return node
          return {...node, data: {...node.data, order: order + 10}}
        })
      }
      next = [...next, nextNode]
      return stages.length > 0 ? workflowModel.autoLayoutWorkflowNodes(next, edges, stages) : next
    })
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

  function stageIDAtFlowPosition(position) {
    if (!stages.length || !position) return ''
    const {stages: layouts} = workflowModel.stageLayout(nodes, stages)
    const found = layouts.find(stage => (
      position.x >= stage.x &&
      position.x <= stage.x + stage.width &&
      position.y >= stage.y &&
      position.y <= stage.y + stage.height
    ))
    return found?.id || ''
  }

  function openNodePickerForStage(stageId, event) {
    event?.stopPropagation()
    if (!isEditingCanvas) return
    const {stages: layouts} = workflowModel.stageLayout(nodes, stages)
    const stage = layouts.find(item => item.id === stageId)
    const position = stage
      ? {x: stage.x + stage.width / 2, y: stage.y + 150}
      : defaultCanvasInsertPosition()
    openNodePicker(position, {mode: 'add', stageId})
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
      insertEdge,
      stageId: options.stageId || ''
    })
    setNodePickerSearchText('')
  }

  function openNodePickerFromEvent(event) {
    event.stopPropagation()
    const point = eventPoint(event)
    const position = point && flowInstance
      ? flowInstance.screenToFlowPosition(point)
      : defaultCanvasInsertPosition()
    openNodePicker(position, {panelPosition: panelPositionFromPoint(point, canvasCardRef.current), stageId: stageIDAtFlowPosition(position)})
  }

  function closeNodePicker() {
    setNodePicker({open: false, mode: 'add', position: null, panelPosition: null, connection: null, insertEdge: null, stageId: ''})
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
    // 条件分支连线拖到空白处时，新节点默认落在当前阶段，但跨阶段分支要落在更靠后的阶段才不冲突。
    const sourceNode = nodes.find(node => node.id === pending.source)
    openNodePicker(position, {
      panelPosition: panelPositionFromPoint(point, canvasCardRef.current),
      connection: {source: pending.source, sourceHandle: pending.sourceHandle || ''},
      stageId: sourceNode?.data?.stageId || ''
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
      panelPosition: panelPositionFromPoint(point, canvasCardRef.current),
      stageId: stageIDAtFlowPosition(position)
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
      const nextEdges = normalizeFlowEdges(reconnectEdge(oldEdge, newConnection, edges), nodes)
      const conflict = workflowModel.validateStageDraft(nodes, nextEdges, stages)[0]
      if (conflict) {
        setResult({message: conflict})
        return
      }
      clearCanvasRunOverlay()
      setEdges(nextEdges)
      setEditorValidation(null)
    },
    [clearCanvasRunOverlay, isEditingCanvas, nodes, edges, stages, setEdges]
  )

  function zoomCanvas(direction) {
    if (!flowInstance) return
    if (direction === 'in') {
      flowInstance.zoomIn()
      return
    }
    flowInstance.zoomOut()
  }

  function stageViewportFor(layoutNodes, layoutStages) {
    const layouts = workflowModel.stageLayout(layoutNodes, layoutStages).stages
    if (layouts.length === 0) return null
    const rect = canvasCardRef.current?.getBoundingClientRect()
    if (!rect || rect.width <= 0 || rect.height <= 0) return null
    const first = layouts[0]
    const last = layouts[layouts.length - 1]
    const bounds = {
      x: first.x - 28,
      y: first.y - 28,
      width: last.x + last.width - first.x + 28 + 24 + 96 + 28,
      height: Math.max(...layouts.map(stage => stage.y + stage.height)) - first.y + 56,
    }
    const zoom = Math.min(rect.width / bounds.width, rect.height / bounds.height, 1)
    return {
      x: (rect.width - bounds.width * zoom) / 2 - bounds.x * zoom,
      y: (rect.height - bounds.height * zoom) / 2 - bounds.y * zoom,
      zoom,
    }
  }

  function fitCanvasView() {
    if (!flowInstance) return
    const viewport = stageViewportFor(nodes, stages)
    if (viewport) {
      flowInstance.setViewport(viewport)
      return
    }
    flowInstance.fitView({padding: 0.2})
  }

  function fitCanvasViewAfterLayout(options = {}) {
    if (!flowInstance) return
    const apply = () => {
      const viewport = stageViewportFor(options.nodes ?? nodes, options.stages ?? stages)
      if (!viewport) return false
      flowInstance.setViewport(viewport)
      return true
    }
    if (apply()) return
    window.setTimeout(() => {
      if (!apply()) flowInstance.fitView({padding: 0.24})
    }, 0)
  }

  function optimizeCanvasLayout() {
    if (!isEditingCanvas) return
    setNodes(current => stages.length > 0
      ? workflowModel.autoLayoutWorkflowNodes(workflowModel.normalizeStageNodeOrders(current, stages), edges, stages)
      : workflowModel.autoLayoutNodes(current, edges))
    setResult({message: nodes.length === 0 ? '当前画布没有节点，无需优化排版。' : '已按阶段顺序重新整理画布。'})
    fitCanvasViewAfterLayout()
  }

  function relayoutStagedNodes(nextNodes) {
    if (!stages.length) return nextNodes
    return workflowModel.autoLayoutWorkflowNodes(workflowModel.normalizeStageNodeOrders(nextNodes, stages), [], stages)
  }

  function renameWorkflowStage(stageId, name) {
    if (!isEditingCanvas || isActiveRunRunning) return
    setStages(current => current.map(stage => stage.id === stageId ? {...stage, name} : stage))
    setEditorValidation(null)
  }

  function addWorkflowStage() {
    if (!isEditingCanvas || isActiveRunRunning) return
    const id = workflowModel.uniqueStageID(stages, `stage-${stages.length + 1}`)
    const next = [...stages, {id, name: `新阶段 ${stages.length + 1}`, execution_mode: 'serial'}]
    setStages(next)
    setResult({message: `已添加阶段「${id}」，可以把步骤拖入该阶段或点击添加步骤。`})
    setEditorValidation(null)
    window.requestAnimationFrame?.(() => fitCanvasViewAfterLayout({stages: next}))
  }

  function stageEdgeConflicts(nextStages, nextNodes = nodes, nextEdges = edges) {
    return workflowModel.validateStageDraft(nextNodes, nextEdges, nextStages)[0] || ''
  }

  // 串并行切换只改阶段模式：edges 始终保留；切回串行时按显式依赖重新编号，避免顺序冲突。
  // 每次切换都给新对象，保证受控 select 在驳回冲突时也会回弹到当前模式。
  function updateWorkflowStageMode(stageId, nextMode) {
    if (!isEditingCanvas || isActiveRunRunning) return
    const mode = nextMode === 'parallel' ? 'parallel' : 'serial'
    const current = stages.find(stage => stage.id === stageId)
    if (!current) return
    // 两种模式共用 max_concurrency：只在执行时按模式解释，切换往返不会丢失并发上限。
    if (mode === workflowModel.stageExecutionMode(current)) {
      setStages(stages.map(stage => stage.id === stageId ? {...stage} : stage))
      return
    }
    const next = stages.map(stage => stage.id === stageId ? {...stage, execution_mode: mode} : stage)
    const nextNodes = mode === 'serial' ? workflowModel.normalizeStageNodeOrders(workflowModel.renumberStageForSerial(nodes, edges, stageId), next) : nodes
    const conflict = stageEdgeConflicts(next, nextNodes)
    if (conflict) {
      setResult({message: `无法切换阶段「${current.name || stageId}」为${mode === 'parallel' ? '并行' : '串行'}：${conflict}`})
      return
    }
    setStages(next)
    if (nextNodes !== nodes) setNodes(workflowModel.autoLayoutWorkflowNodes(nextNodes, edges, next))
    setEditorValidation(null)
    const limit = workflowModel.normalizeStageConcurrency(current.max_concurrency)
    setResult({message: mode === 'parallel'
      ? `阶段「${current.name || stageId}」已改为并行${limit > 0 ? `，最多 ${limit} 个节点同时执行` : '，使用默认并发数'}；依赖不会被自动建立或删除。`
      : `阶段「${current.name || stageId}」已改为串行，节点按当前顺序依次执行。`})
  }

  // 并发数是可随时编辑的输入框，不能用 updateWorkflowStageMode：那样空值会被当成“未变化”再回填旧值。
  function updateWorkflowStageConcurrency(stageId, value) {
    if (!isEditingCanvas || isActiveRunRunning) return
    const limit = workflowModel.normalizeStageConcurrency(value)
    setStages(current => current.map(stage => {
      if (stage.id !== stageId) return stage
      if (workflowModel.stageExecutionMode(stage) !== 'parallel' || limit === workflowModel.normalizeStageConcurrency(stage.max_concurrency)) return stage
      const {max_concurrency: _drop, ...rest} = stage
      return limit > 0 ? {...rest, max_concurrency: limit} : rest
    }))
    setEditorValidation(null)
  }

  function applyStageOrder(next) {
    const unchanged = next.every((stage, index) => stage.id === stages[index]?.id)
    if (unchanged) return
    const conflict = stageEdgeConflicts(next)
    if (conflict) {
      setResult({message: `无法调整阶段顺序：${conflict}`})
      return
    }
    setStages(next)
    setNodes(current => workflowModel.autoLayoutWorkflowNodes(current, edges, next))
    setEditorValidation(null)
  }

  function moveStageTo(stageId, targetIndex) {
    if (!isEditingCanvas || isActiveRunRunning) return
    applyStageOrder(workflowModel.moveWorkflowStageTo(stages, stageId, targetIndex))
  }

  function handleStageDragStart(event, stageId) {
    // 输入框、下拉和按钮保留原生交互，不触发阶段拖拽。
    if (!isEditingCanvas || isActiveRunRunning || event.target.closest('input, select, textarea, button')) {
      event.preventDefault()
      return
    }
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', stageId)
    setStageDragID(stageId)
    setStageDropTargetID('')
  }

  // 拖动时指针下方可能是节点或其它画布元素，按阶段列的实际矩形判定目标，避免放过节点就失效。
  function stageAtPoint(x, y) {
    if (!canvasCardRef.current || !Number.isFinite(x) || !Number.isFinite(y)) return ''
    const columns = canvasCardRef.current.querySelectorAll('.workflowStageColumn')
    for (const column of columns) {
      const rect = column.getBoundingClientRect()
      if (x >= rect.left && x <= rect.right && y >= rect.top && y <= rect.bottom) return column.dataset.stageId || ''
    }
    return ''
  }

  function handleCanvasStageDragOver(event) {
    if (!stageDragID) return
    const stageId = stageAtPoint(event.clientX, event.clientY)
    if (!stageId || stageId === stageDragID) {
      if (stageDropTargetID) setStageDropTargetID('')
      return
    }
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
    if (stageDropTargetID !== stageId) setStageDropTargetID(stageId)
  }

  function handleCanvasStageDrop(event) {
    if (!stageDragID) return
    event.preventDefault()
    const stageId = stageAtPoint(event.clientX, event.clientY)
    const dragged = stageDragID
    setStageDragID('')
    setStageDropTargetID('')
    if (!stageId || stageId === dragged) return
    const targetIndex = stages.findIndex(stage => stage.id === stageId)
    if (targetIndex >= 0) moveStageTo(dragged, targetIndex)
  }

  function handleStageDragEnd() {
    setStageDragID('')
    setStageDropTargetID('')
  }

  function deleteWorkflowStage(stageId) {
    if (!isEditingCanvas || isActiveRunRunning) return
    if (stages.length <= 1) {
      setResult({message: '至少保留一个阶段。'})
      return
    }
    const stage = stages.find(item => item.id === stageId)
    const stageNodeCount = nodes.filter(node => node.data?.stageId === stageId).length
    if (stageNodeCount > 0) {
      setResult({message: `阶段「${stage?.name || stageId}」还有 ${stageNodeCount} 个步骤，请先把步骤移到其他阶段或删除。`})
      return
    }
    if (!window.confirm(`确认删除空阶段「${stage?.name || stageId}」？`)) return
    const next = stages.filter(item => item.id !== stageId)
    setStages(next)
    setNodes(current => workflowModel.autoLayoutWorkflowNodes(current, edges, next))
    setEditorValidation(null)
  }

  function stageOfNodeID(nodeId, nextStages = stages) {
    const node = nodes.find(item => item.id === nodeId)
    return node?.data?.stageId || (nextStages.length ? nextStages[0].id : '')
  }

  function applyStageNodeMove(nextNodes) {
    const next = workflowModel.normalizeStageNodeOrders(nextNodes, stages)
    const conflict = workflowModel.newStageDraftErrors(nodes, next, edges, stages)[0]
    if (conflict) {
      setResult({message: conflict})
      setNodes(current => workflowModel.autoLayoutWorkflowNodes(current, edges, stages))
      return false
    }
    clearCanvasRunOverlay()
    setNodes(workflowModel.autoLayoutWorkflowNodes(next, edges, stages))
    setEditorValidation(null)
    return true
  }

  function moveSelectedNodeToStage(targetStageId) {
    if (!isEditingCanvas || isActiveRunRunning || !selectedNodeID || !stages.length) return
    if (stageOfNodeID(selectedNodeID) === targetStageId) return
    if (!applyStageNodeMove(workflowModel.moveNodeToStage(nodes, selectedNodeID, targetStageId))) return
    setNodeConfigDraft(current => current ? {...current, data: {...current.data, stageId: targetStageId}} : current)
    setEditorValidation(null)
  }

  // 目标阶段里已在移动节点前面的节点会挡住前移（串行顺序 + 串行依赖），直接从选择里排除并说明原因。
  function batchMovePlan(targetStageId, selection = canvasSelectedNodeIDs) {
    const targetOrder = workflowModel.orderedStageNodes(nodes, targetStageId)
    const firstIndex = targetOrder.findIndex(node => selection.includes(node.id))
    const blocked = new Set()
    if (firstIndex > 0) targetOrder.slice(0, firstIndex).forEach(node => blocked.add(node.id))
    const selected = selection.filter(id => !blocked.has(id))
    return {selected, shadowed: selection.filter(id => blocked.has(id))}
  }

  function explainSelectionFilter(shadowedIDs) {
    if (!shadowedIDs.length) return
    const names = shadowedIDs.map(id => nodes.find(node => node.id === id)?.data?.name || id)
    setResult({message: `${names.join('、')} 在目标阶段里已排在更靠前的位置，移入会改变串行顺序，已从本次移动中排除。`})
  }

  function batchMoveConflict(selection, targetStageId) {
    const next = workflowModel.normalizeStageNodeOrders(
      workflowModel.moveNodesToStage(nodes, selection, targetStageId),
      stages,
    )
    return workflowModel.newStageDraftErrors(nodes, next, edges, stages)[0] || ''
  }

  // 冲突只挡住当前目标阶段，换到其他阶段仍可一次完成，所以弹窗保留选择并允许切换目标。
  function applyBatchStageMove(targetStageId = batchStageMove?.targetStageId) {
    if (!targetStageId) return
    const plan = batchMovePlan(targetStageId)
    setBatchStageMenuOpen(false)
    if (!plan.selected.length) {
      setBatchStageMove(null)
      explainSelectionFilter(plan.shadowed)
      return
    }
    // 先落盘再提示冲突，避免在校验回调里调用 setState。
    const moved = applyStageNodeMove(workflowModel.moveNodesToStage(nodes, plan.selected, targetStageId))
    setBatchStageMove(moved ? null : {targetStageId, selected: plan.selected, shadowed: plan.shadowed})
    if (moved) explainSelectionFilter(plan.shadowed)
  }

  function moveSelectedNodeInStage(direction) {
    if (!isEditingCanvas || isActiveRunRunning || !selectedNodeID || !stages.length) return
    const moved = workflowModel.moveNodeWithinStage(nodes, selectedNodeID, direction)
    if (moved !== nodes) applyStageNodeMove(moved)
  }

  // 拖动多选中的任一节点时整组移动；组内显式依赖决定先后，排序后避免整组因阶段内顺序冲突被挡回。
  function draggedStageNodeIDs(draggedNode) {
    let ids = []
    if (draggedNode?.dragging?.length > 1) ids = draggedNode.dragging.map(node => node.id)
    else if (canvasSelectedNodeIDs.length > 1 && canvasSelectedNodeIDs.includes(draggedNode?.id)) ids = canvasSelectedNodeIDs
    else return draggedNode ? [draggedNode.id] : []
    return dependencyFirstIDs(ids, edges)
  }

  // 组内显式依赖要求来源节点排在目标节点之前；无依赖时沿用传入顺序（画布顺序）。
  function dependencyFirstIDs(ids, list) {
    const selected = new Set(ids)
    const indegree = new Map(ids.map(id => [id, 0]))
    const children = new Map(ids.map(id => [id, []]))
    list.forEach(edge => {
      if (!selected.has(edge.source) || !selected.has(edge.target) || edge.source === edge.target) return
      children.get(edge.source).push(edge.target)
      indegree.set(edge.target, (indegree.get(edge.target) || 0) + 1)
    })
    const queue = ids.filter(id => indegree.get(id) === 0)
    const ordered = []
    while (queue.length) {
      const id = queue.shift()
      ordered.push(id)
      children.get(id).forEach(child => {
        const next = indegree.get(child) - 1
        indegree.set(child, next)
        if (next === 0) queue.push(child)
      })
    }
    return ordered
  }

  function handleStageSelectionDragStop(event, draggedNodes) {
    const list = (draggedNodes || []).filter(Boolean)
    if (list.length < 2) return
    handleStageNodeDragStop(event, list[0])
  }

  function handleStageNodeDragStop(event, draggedNode) {
    if (!isEditingCanvas || isActiveRunRunning || !stages.length || !draggedNode) return
    const layouts = workflowModel.stageLayout(nodes, stages)
    if (!layouts.stages.length) return
    const node = nodes.find(item => item.id === draggedNode.id) || draggedNode
    const movingIDs = draggedStageNodeIDs(draggedNode)
    const moving = new Set(movingIDs)
    const width = node.measured?.width || node.width || workflowModel.WORKFLOW_NODE_WIDTH
    const height = node.measured?.height || node.height || workflowModel.WORKFLOW_NODE_HEIGHT
    const centerX = draggedNode.position.x + width / 2
    const centerY = draggedNode.position.y + height / 2
    let target = layouts.stages.find(stage => centerX >= stage.x && centerX <= stage.x + stage.width)
    if (!target) {
      target = layouts.stages.reduce((nearest, stage) => {
        if (!nearest) return stage
        const distance = Math.abs(stage.x + stage.width / 2 - centerX)
        const nearestDistance = Math.abs(nearest.x + nearest.width / 2 - centerX)
        return distance < nearestDistance ? stage : nearest
      }, null)
    }
    if (!target) return
    const stageNodes = workflowModel.orderedStageNodes(nodes, target.id).filter(item => !moving.has(item.id))
    let insertAt = stageNodes.findIndex(item => {
      const position = layouts.positions[item.id] || item.position || {x: 0, y: 0}
      const itemHeight = item.measured?.height || item.height || workflowModel.WORKFLOW_NODE_HEIGHT
      return centerY < position.y + itemHeight / 2
    })
    if (insertAt < 0) insertAt = stageNodes.length
    const orderedIDs = stageNodes.map(item => item.id)
    orderedIDs.splice(insertAt, 0, ...movingIDs)
    const orderMap = new Map(orderedIDs.map((id, index) => [id, (index + 1) * 10]))
    const dragID = draggedStageNodeIDs(draggedNode)[0] || node.id
    const shifted = movingIDs.length > 1 && orderedIDs.indexOf(dragID) !== orderedIDs.indexOf(movingIDs[0])
    if (shifted) {
      setResult({message: '组内节点的显式依赖决定了先后顺序，批量移动已按依赖顺序插入目标阶段。'})
    }
    applyStageNodeMove(nodes.map(item => {
      if (!orderMap.has(item.id)) return item
      const stageId = moving.has(item.id) ? target.id : item.data?.stageId
      return {...item, data: {...item.data, stageId, order: orderMap.get(item.id)}}
    }))
  }

  function stageRunSummary(stageId) {
    const statuses = nodes
      .filter(node => node.data?.stageId === stageId)
      .map(node => normalizeRunStatus(canvasRunState.nodes?.[node.id]?.status))
      .filter(status => status && status !== 'idle')
    if (statuses.length === 0) return null
    if (statuses.includes('running')) return {kind: 'running', label: '运行中'}
    if (statuses.includes('failed')) return {kind: 'failed', label: '失败'}
    if (statuses.includes('waiting')) return {kind: 'running', label: '等待'}
    if (statuses.every(status => status === 'succeeded' || status === 'skipped')) {
      return {kind: 'succeeded', label: '已完成'}
    }
    if (statuses.includes('cancelled')) return {kind: 'failed', label: '已取消'}
    return null
  }

  function stageMutationWarnings(stageId) {
    const stage = stages.find(item => item.id === stageId)
    if (workflowModel.stageExecutionMode(stage) !== 'parallel') return []
    const stageNodes = workflowModel.orderedStageNodes(nodes, stageId)
    const byHost = new Map()
    stageNodes.forEach(node => {
      const host = executionHostForNode(node, runTarget, runTargetOverride)
      if (!byHost.has(host)) byHost.set(host, [])
      byHost.get(host).push(node)
    })
    const warnings = []
    byHost.forEach((list, host) => {
      if (list.length < 2) return
      const target = host ? runHostOptions.find(item => item.id === host)?.name || host : '本机'
      const names = list.map(node => node.data?.name || node.id).join('、')
      // 只提示，不做安全推断：Shell / 工具节点的副作用是否可并行由使用者判断。
      warnings.push(`${target} 上有 ${list.length} 个节点可能同时执行（${names}），请确认命令之间没有写同一文件、重启同一服务等冲突。`)
    })
    return warnings
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

  function updateSelectedNodeScript(nextScript) {
    updateDraftNodeData({script: workflowModel.normalizeScriptConfig(nextScript)})
  }

  function updateSelectedNodeChecksum(nextChecksum) {
    updateDraftNodeData({checksum: workflowModel.normalizeChecksumConfig(nextChecksum)})
  }

  function updateSelectedNodeArchive(nextArchive) {
    updateDraftNodeData({archive: workflowModel.normalizeArchiveConfig(nextArchive)})
  }

  function updateSelectedNodeWait(nextWait) {
    updateDraftNodeData({wait: workflowModel.normalizeWaitConfig(nextWait)})
  }

  function updateSelectedNodeNotify(nextNotify) {
    updateDraftNodeData({notify: workflowModel.normalizeNotifyConfig(nextNotify)})
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

  function clearSelection() {
    setNodeConfigModalOpen(false)
    setNodeConfigDraft(null)
    setEdgeConfigModalOpen(false)
    setSelectedNodeID('')
    setSelectedEdgeID('')
  }

  const undoFlowEdit = useCallback(() => {
    if (!isEditingCanvas) return
    if (!flowHistory.undo()) {
      setEditorNotice('已经回到最早的画布状态，没有更多可撤销的编辑。')
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
      setEditorNotice('已经是最新的画布状态，没有更多可重做的编辑。')
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
    setNodes(current => {
      const next = current.map(node => {
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
          if (data.controlType === 'shell_script') {
            nextData.host = data.host || ''
            nextData.script = workflowModel.normalizeScriptConfig(data.script)
          }
          if (data.controlType === 'file_checksum') {
            nextData.host = data.host || ''
            nextData.checksum = workflowModel.normalizeChecksumConfig(data.checksum)
          }
          if (data.controlType === 'archive') nextData.archive = workflowModel.normalizeArchiveConfig(data.archive)
          if (data.controlType === 'wait') nextData.wait = workflowModel.normalizeWaitConfig(data.wait)
          if (data.controlType === 'notify') nextData.notify = workflowModel.normalizeNotifyConfig(data.notify)
        }
        return {...node, data: nextData}
      })
      return stages.length > 0 ? relayoutStagedNodes(next) : next
    })
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
    const draft = workflowModel.buildWorkflowDraft(workflow, nodes, edges, '', workflowParameters, stages)
    if (!workflowOwnerLocked && workflowOwnerID !== 'user.workflows') {
      const ownerCategories = pluginCategories(catalog, workflowOwnerID)
      draft.category = ownerCategories.find(item => item.id === draft.category)?.id || ownerCategories[0]?.id || 'global'
    }
    const sharedWith = (draft.shared_with || []).filter(id => id && id !== workflowOwnerID && id !== 'user.workflows')
    draft.shared_with = sharedWith.length > 0 ? sharedWith : undefined
    workflowModel.validateStageDraft(nodes, edges, stages).forEach(error => errors.push(error))
    workflowModel.validateConditionDraft(nodes, edges).forEach(error => errors.push(error))
    workflowModel.validateControlDraft(nodes, edges, catalog.tools || []).forEach(error => errors.push(error))
    if (stages.length === 0 && nodes.length > 1 && edges.length === 0 && draft.edges.length === 0) {
      errors.push('多个节点没有连线，无法确定执行顺序。请为条件、循环等控制节点显式连接依赖关系。')
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
    const check = preflightWorkflowDraft('run')
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

  function openSaveModal(options = {}) {
    if (workflowAction) return
    setSaveModalErrorTitle(options.settings ? '设置保存前检查未通过' : '保存前检查未通过')
    setSaveModalErrors([])
    setSaveModalSettings(Boolean(options.settings))
    setSaveModalOpen(true)
  }

  async function saveDraft() {
    const check = preflightWorkflowDraft('save')
    if (check.errors.length > 0) {
      setSaveModalErrorTitle(saveModalSettings ? '设置保存前检查未通过' : '保存前检查未通过')
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
      pendingWorkflowSelectRef.current = check.draft.id
      setSelectedWorkflowID(check.draft.id)
      setDraftStarted(true)
      setSavedDraftSignature(workflowDraftSignature(check.draft, nodes, edges, '', workflowParamsText, source?.plugin_id || workflowOwnerID, stages))
      setRecentWorkflowIDs(pushRecentWorkflowID(check.draft.id))
      setSaveModalOpen(false)
      setSaveModalSettings(false)
      if (draftWindow) {
        // 保存成功后不再需要带入标记，并让主窗口知道该重新加载。
        clearDraftWindowTransfer()
        setDraftWindowTransfer(null)
        writeDraftWindowSaved({id: check.draft.id, name: check.draft.name || check.draft.id, ts: Date.now()})
        setResult({message: summarizeAPIResponse(body, `草稿窗口已保存工作流 ${check.draft.id}，主窗口会提示重新加载。`), response: body})
      } else {
        setResult({message: summarizeAPIResponse(body, '工作流保存成功。'), response: body})
      }
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
    if (isActiveRunRunning) return
    const id = selectedWorkflowID
    if (!id) return
    const label = workflow.name || id
    if (!window.confirm(`确认删除工作流「${label}」？\n\n该操作会删除归属插件目录中的工作流配置，且不可撤销。`)) return
    try {
      setWorkflowAction('deleting')
      const body = await deleteJSON(`/api/workflows/${encodeURIComponent(id)}`)
      clearCanvasRunOverlay()
      const next = emptyWorkflow()
      setWorkflowMoreOpen(false)
      setWorkflow(next)
      setWorkflowParamsText('[]')
      setRunParamsText('{}')
      setNodes([])
      setEdges([])
      setStages(next.stages)
      flowHistory.resetHistory([], [], next.stages)
      setSelectedWorkflowID('')
      setDraftStarted(false)
      setWorkflowOwnerID('user.workflows')
      setWorkflowOwnerLocked(false)
      setRecentWorkflowIDs(forgetRecentWorkflowID(id))
      setSelectedNodeID('')
      setSelectedEdgeID('')
      setNodeConfigModalOpen(false)
      setNodeConfigDraft(null)
      setEdgeConfigModalOpen(false)
      setEditorValidation(null)
      fitCanvasViewAfterLayout({nodes: [], stages: next.stages})
      setSavedDraftSignature(workflowDraftSignature(next, [], [], '', '[]', 'user.workflows', next.stages))
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
    const check = preflightRunParamsStep()
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
    setActiveRun(null)
    setLastRun(null)
    setCanvasRunDetail(null)
    setUploadProgress(null)
    const runNodesSnapshot = nodes.map(node => ({...node, data: {
      ...node.data,
      host: executionHostForNode(node, runTarget, runTargetOverride)
    }}))
    const initialRunState = buildRunningCanvasRunState(runNodesSnapshot, check.draft.edges, stages)
    setRunSnapshotNodes(runNodesSnapshot)
    setRunSetupOpen(false)
    setRunDockOpen(true)
    setRunDockNodeID(firstRunDockNodeID(runNodesSnapshot, initialRunState))
    setCanvasRunState(initialRunState)
    const applyRunOverlay = nextState => {
      if (canvasRunVersionRef.current !== runVersion) return
      setCanvasRunState(nextState)
      setRunDockOpen(true)
      setRunDockNodeID(current => current && nextState.nodes?.[current] ? current : firstRunDockNodeID(runNodesSnapshot, nextState))
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
      const body = await postJSON(`/api/workflows/${check.draft.id}/run?async=true`, {
        params: runParams,
        workflow: check.draft,
        confirm: confirmRequired,
        target: runTarget === RUN_TARGET_FOLLOW ? undefined : runTarget,
        override_nodes: runTarget === RUN_TARGET_FOLLOW ? false : runTargetOverride
      })
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

  function openRunSetup() {
    if (isActiveRunRunning) {
      setResult({message: `当前工作流正在运行：${activeRunID || '等待运行记录 ID'}，请先等待完成或取消运行。`, run: activeRun || undefined})
      return
    }
    setWorkflowNameMenuOpen(false)
    setWorkflowMoreOpen(false)
    setEditorValidation(null)
    setRunSetupOpen(true)
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
          if (status !== 'running') {
            if (status === 'pending') refreshPendingRuns()
            return detail
          }
        }
      } catch (err) {
        missingAttempts += 1
        if (missingAttempts >= maxMissingAttempts) throw err
      }
      await sleep(1000)
    }
    return null
  }

  const draftCheck = preflightWorkflowDraft('save')
  const draftStatus = draftCheck.errors.length > 0
    ? {kind: 'warning', title: `${draftCheck.errors.length} 项待配置`, detail: draftCheck.errors[0]}
    : hasUnsavedChanges
      ? {kind: 'changed', title: '有未保存修改', detail: '结构检查已通过，保存后可供其他用户加载。'}
      : {kind: 'ready', title: selectedWorkflowID ? '已保存' : '草稿就绪', detail: '结构检查已通过，可以保存或运行。'}
  const recentWorkflowOptions = recentWorkflowIDs
    .map(id => ({id, item: workflowOptions.find(option => option.id === id)}))
    .filter(entry => entry.item && (!workflowCategoryFilter || workflowVisibleCategories.get(entry.id)?.has(workflowCategoryFilter)))
  const recentWorkflowIDsInResults = new Set(recentWorkflowOptions.map(({id}) => id))
  const workflowLoader = (
    <section className="workflowLoader" aria-labelledby="workflow-selection-title">
      <div className="workflowLoaderToolbar">
        <label className="workflowLoaderSearch">
          <span className="visuallyHidden">搜索工作流</span>
          <input
            type="search"
            value={workflowSearchText}
            disabled={Boolean(workflowAction) || isActiveRunRunning}
            placeholder="搜索名称或 ID"
            aria-label="搜索工作流"
            onChange={event => setWorkflowSearchText(event.target.value)}
          />
        </label>
        <label className="workflowLoaderCategory">
          <span className="visuallyHidden">按分类筛选工作流</span>
          <select
            aria-label="按分类筛选工作流"
            value={workflowCategoryFilter}
            disabled={Boolean(workflowAction) || isActiveRunRunning || workflowCategoryGroups.length === 0}
            onChange={event => setWorkflowCategoryFilter(event.target.value)}
          >
            <option value="">全部分类</option>
            {workflowCategoryGroups.map(group => (
              <option key={group.id} value={group.id}>{group.label}（{group.count}）</option>
            ))}
          </select>
        </label>
      </div>
      <div className="workflowLoaderSummary">
        <strong id="workflow-selection-title">选择工作流</strong>
        <span>
          {workflowOptions.length === 0
            ? '暂无已保存工作流'
            : workflowCategoryFilter
              ? `${workflowCategoryLabel(workflowCategoryFilter, workflowCategoryLabels)} · ${filteredWorkflowOptions.length} 个`
              : `全部 · ${filteredWorkflowOptions.length} 个`}
        </span>
      </div>
      <div className="workflowLoaderList" role="listbox" aria-label="工作流列表">
        {workflowAction === 'loading' && <div className="workflowLoaderEmpty">正在加载工作流...</div>}
        {selectedWorkflowOutsideFilter && (
          <div className="workflowLoaderGroup">
            <span className="workflowLoaderGroupLabel">当前已加载</span>
            {renderWorkflowListItem(workflowOptions.find(item => item.id === selectedWorkflowID), {outsideFilter: true})}
          </div>
        )}
        {workflowOptionGroups.map(group => (
          <div key={group.id} className="workflowLoaderGroup">
            <span className="workflowLoaderGroupLabel">{group.label}<em>{group.items.length}</em></span>
            {group.items.map(item => renderWorkflowListItem(item, {recent: recentWorkflowIDsInResults.has(item.id)}))}
          </div>
        ))}
        {workflowAction !== 'loading' && workflowOptionGroups.length === 0 && !selectedWorkflowOutsideFilter && (
          <div className="workflowLoaderEmpty">
            {workflowOptions.length === 0
              ? '还没有已保存工作流。可从下方新建一个。'
              : '没有匹配的工作流，请调整搜索词或分类。'}
          </div>
        )}
      </div>
    </section>
  )

  function renderWorkflowListItem(item, {recent = false, outsideFilter = false} = {}) {
    if (!item) return null
    const selected = item.id === selectedWorkflowID
    const owner = item.source?.plugin_id
    const ownerLabel = owner && owner !== 'user.workflows' ? item.source.plugin_name || owner : ''
    const shared = workflowCategoryFilter && workflowCategoryKey(item) !== workflowCategoryFilter
    const disabled = Boolean(workflowAction) || isActiveRunRunning
    return (
      <button
        key={item.id}
        type="button"
        role="option"
        aria-selected={selected}
        className={`workflowLoaderItem${selected ? ' active' : ''}`}
        data-workflow-id={item.id}
        disabled={disabled}
        title={item.id}
        onClick={event => {
          event.stopPropagation()
          loadWorkflow(item.id, {force: selected})
        }}
      >
        <span className="workflowLoaderItemMain">
          <strong>{item.name || item.id}</strong>
          <small>{item.id}</small>
        </span>
        <span className="workflowLoaderItemMeta">
          {recent && !selected && <span className="workflowLoaderTag">最近</span>}
          {outsideFilter && <span className="workflowLoaderTag muted">当前</span>}
          {shared && <span className="workflowLoaderTag muted">共享</span>}
          {ownerLabel && <span className="workflowLoaderTag muted">{ownerLabel}</span>}
          {selected && <span className="workflowLoaderTag active">已加载</span>}
        </span>
      </button>
    )
  }
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
  const workflowTopBar = (
    <header className={`workflowTopBar${workflowNameMenuOpen ? ' menuOpen' : ''}`}>
      <div className="workflowTopBarMain">
        <span className="workflowTopBarLabel">工作流</span>
        <div className="workflowNameMenu">
          <button
            type="button"
            className="workflowNameTrigger"
            aria-haspopup="dialog"
            aria-expanded={workflowNameMenuOpen}
            disabled={isActiveRunRunning}
            title={isActiveRunRunning ? '运行期间不能切换工作流' : undefined}
            onClick={() => {
              setWorkflowMoreOpen(false)
              setWorkflowNameMenuOpen(open => !open)
            }}
          >
            <strong>{workflow.name || workflow.id || '未命名工作流'}</strong>
            <span aria-hidden="true">▾</span>
          </button>
          {workflowNameMenuOpen && (
            <>
              <div className="workflowMenuScrim" onClick={() => setWorkflowNameMenuOpen(false)} />
              <div className="workflowNamePopover" role="dialog" aria-label="切换工作流">
                <div className="workflowNamePopoverHeader">
                  <div>
                    <strong>{selectedWorkflowID ? '切换工作流' : '打开工作流'}</strong>
                    <span>{workflowOwnerLabel} · {workflowCategoryText}{selectedWorkflowID ? ' · 已加载到当前画布' : ''}</span>
                  </div>
                  <button type="button" className="modalClose" onClick={() => setWorkflowNameMenuOpen(false)}>×</button>
                </div>
                {workflowLoader}
                <div className="workflowLoaderFooter">
                  <span>最近打开的工作流会标记为“最近”</span>
                  <button className="secondary" type="button" onClick={createWorkflow} disabled={Boolean(workflowAction) || isActiveRunRunning}>新建工作流</button>
                </div>
              </div>
            </>
          )}
        </div>
        <span className={`workflowSaveState ${draftStatus.kind}`} role="status" aria-live="polite" title={draftStatus.detail}>{draftStatus.title}</span>
      </div>
      <div className="workflowTopBarActions">
        <button className="secondary workflowTopBarIcon" type="button" onClick={undoFlowEdit} disabled={!flowHistory.canUndo || isActiveRunRunning} title="撤销（Ctrl+Z）">撤销</button>
        <button className="secondary workflowTopBarIcon" type="button" onClick={redoFlowEdit} disabled={!flowHistory.canRedo || isActiveRunRunning} title="重做（Ctrl+Y / Ctrl+Shift+Z）">重做</button>
        <button className="secondary" type="button" onClick={openSaveModal} disabled={Boolean(workflowAction) || isActiveRunRunning} title="保存（Ctrl+S）">保存</button>
        {!draftWindow && (
          <button
            className="secondary workflowTopBarIcon"
            type="button"
            onClick={openDraftWindow}
            disabled={Boolean(workflowAction) || nodes.length === 0}
            aria-label="打开独立草稿窗口"
            title={isActiveRunRunning
              ? '本次运行继续执行；在独立窗口编辑草稿，不会影响正在运行的快照'
              : '在新窗口打开当前草稿，与画布内容互不覆盖'}
          >草稿窗口</button>
        )}
        {draftWindow ? (
          <span className="workflowTopBarHint">独立草稿窗口仅编辑和保存，运行请在主窗口进行</span>
        ) : isActiveRunRunning ? (
          <button className="primary danger" type="button" onClick={cancelActiveRun} disabled={!activeRunID || cancellingRunID === activeRunID} aria-label="取消当前工作流运行">
            {!activeRunID ? '启动中...' : cancellingRunID === activeRunID ? '取消中...' : '取消运行'}
          </button>
        ) : (
          <>
            {currentWorkflowPendingRuns.length > 0 && (
              <button
                className="secondary workflowPendingTopBarButton"
                type="button"
                onClick={() => openRecoveryDrawer(currentWorkflowPendingRuns[0].id)}
                title={`当前工作流有 ${currentWorkflowPendingRuns.length} 个待处理运行，请先重试检查或回滚`}
              >
                待处理 {currentWorkflowPendingRuns.length}
              </button>
            )}
            <button
              className="primary"
              type="button"
              onClick={openRunSetup}
              disabled={nodes.length === 0 || Boolean(workflowAction)}
              aria-label="运行工作流"
              title={hasUnsavedChanges ? '运行当前画布内容（包含未保存修改）' : '运行工作流'}
            >
              运行
            </button>
          </>
        )}
        <div className="workflowMoreMenu">
          <button
            className="secondary"
            type="button"
            aria-haspopup="menu"
            aria-expanded={workflowMoreOpen}
            onClick={() => {
              setWorkflowNameMenuOpen(false)
              setWorkflowMoreOpen(open => !open)
            }}
          >
            更多
          </button>
          {workflowMoreOpen && (
            <>
              <div className="workflowMenuScrim" onClick={() => setWorkflowMoreOpen(false)} />
              <div className="workflowMorePopover" role="menu">
                <button type="button" role="menuitem" onClick={() => { setWorkflowMoreOpen(false); createWorkflow() }} disabled={Boolean(workflowAction) || isActiveRunRunning}>新建工作流</button>
                <button
                  type="button"
                  role="menuitem"
                  onClick={() => { setWorkflowMoreOpen(false); openSaveModal({settings: true}) }}
                  disabled={!selectedWorkflowID || Boolean(workflowAction) || isActiveRunRunning}
                  title={selectedWorkflowID ? '修改名称、描述、归属、共享与参数' : '当前画布还没有保存为工作流'}
                >
                  工作流设置
                </button>
                <button type="button" role="menuitem" onClick={() => { setWorkflowMoreOpen(false); setWorkflowTransferOpen(true) }} disabled={isActiveRunRunning}>导入工作流…</button>
                <button type="button" role="menuitem" onClick={() => { setWorkflowMoreOpen(false); validateDraft() }} disabled={Boolean(workflowAction)}>{workflowAction === 'validating' ? '校验中...' : '手动校验'}</button>
                <button type="button" role="menuitem" onClick={() => { setWorkflowMoreOpen(false); optimizeCanvasLayout() }} disabled={isActiveRunRunning}>重新整理画布</button>
                {resultPanel && <button type="button" role="menuitem" onClick={() => { setWorkflowMoreOpen(false); setRunHistoryOpen(true) }}>运行记录</button>}
                <button className="danger" type="button" role="menuitem" onClick={() => { setWorkflowMoreOpen(false); deleteWorkflow() }} disabled={!canDeleteWorkflow || Boolean(workflowAction) || isActiveRunRunning} title={workflowOwnerLocked ? '内置工作流不支持在此删除' : undefined}>删除工作流</button>
              </div>
            </>
          )}
        </div>
      </div>
    </header>
  )
  const workflowStatusNotices = (editorValidation?.errors?.length > 0 || editorNotice || draftWindowSaved) ? (
    <div className="workflowStatusNotices">
      {editorValidation?.errors?.length > 0 && <ValidationSummary status={editorValidation} />}
      {editorNotice && <div className="workflowEditorNotice" role="status" aria-live="polite">{editorNotice}</div>}
      {draftWindowSaved && (
        <div className="workflowEditorNotice workflowDraftWindowSaved" role="status" aria-live="polite">
          <span>
            独立草稿窗口已保存工作流：{draftWindowSaved.name || draftWindowSaved.id}
            {formatDraftWindowTime(draftWindowSaved.ts) ? `（${formatDraftWindowTime(draftWindowSaved.ts)}）` : ''}。当前画布仍是原内容。
          </span>
          <span className="workflowDraftWindowSavedActions">
            <button type="button" className="secondary small" onClick={() => reloadSavedDraftWindowWorkflow(draftWindowSaved)}>重新加载</button>
            <button type="button" className="secondary small" onClick={dismissSavedDraftWindowWorkflow}>稍后处理</button>
          </span>
        </div>
      )}
    </div>
  ) : null
  const draftWindowTransferBanner = draftWindow && draftWindowTransfer ? (
    <div className="workflowDraftWindowBanner" role="status">
      <div>
        <strong>已带入主窗口草稿{draftWindowTransfer.name ? `：${draftWindowTransfer.name}` : ''}</strong>
        <span>
          {formatDraftWindowTime(draftWindowTransfer.ts) ? `${formatDraftWindowTime(draftWindowTransfer.ts)} 带入 · ` : ''}
          这个窗口只编辑和保存草稿，运行仍在主窗口进行，并使用启动时的工作流快照。
        </span>
      </div>
      <div className="workflowDraftWindowBannerActions">
        <button type="button" className="secondary small" onClick={bringInDraftWindowTransfer} disabled={Boolean(workflowAction)}>重新带入主窗口内容</button>
        <button type="button" className="secondary small" onClick={dismissDraftWindowTransfer}>不再提示</button>
      </div>
    </div>
  ) : null
  const workflowPendingBanner = !draftWindow && sortedPendingRuns.length > 0 ? (
    <div className="workflowPendingBanner" role="alert" aria-label="待处理运行">
      <div className="workflowPendingBannerText">
        <strong>有 {sortedPendingRuns.length} 个运行等待处理</strong>
        <span>配置已变更但后续检查失败，需要重试检查或回滚；刷新页面或重启服务后仍会保留。</span>
      </div>
      <div className="workflowPendingBannerActions">
        {(pendingRunsExpanded ? sortedPendingRuns : sortedPendingRuns.slice(0, 3)).map(item => (
          <button
            key={item.id}
            type="button"
            className="secondary"
            onClick={() => openRecoveryDrawer(item.id)}
            title={`${item.id}${item.error ? `\n${item.error}` : ''}`}
          >
            {item.target || item.id} · {formatRunTimestamp(item.started_at) || item.id}
          </button>
        ))}
        {sortedPendingRuns.length > 3 && (
          <button type="button" className="secondary" onClick={() => setPendingRunsExpanded(expanded => !expanded)} aria-expanded={pendingRunsExpanded}>
            {pendingRunsExpanded ? '收起待处理列表' : `更多（${sortedPendingRuns.length - 3}）`}
          </button>
        )}
      </div>
    </div>
  ) : null
  const workflowRunParamsContent = (
    <div className="form compact workflowRunSetup">
      <div className="workflowRunInputGroup" aria-label="运行入参">
        <div className="workflowRunInputGroupHeader">
          <strong>运行入参</strong>
          <span className="workflowRunInputGroupTag">填写 / 选择</span>
        </div>
        <section className="workflowRunParamSection">
          <div className="workflowRunParamGroupHeader">
            <strong>工作流参数</strong>
            <span>节点参数请在画布中打开对应节点配置，这里只填写工作流级参数。</span>
          </div>
        {workflowParameters.length === 0 && <div className="empty small">当前工作流无需工作流级参数。</div>}
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
        </section>
        {uploadFlowNodes(nodes).length > 0 && (
          <section className="workflowRunParamSection">
            <div className="workflowRunParamGroupHeader">
              <strong>上传文件</strong>
              <span>上传节点在运行时会等待这里选择的文件，然后按节点配置的目标目录上传。</span>
            </div>
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
          </section>
        )}
      </div>
      <div className="workflowRunSettingGroup" aria-label="执行设置">
        <div className="workflowRunInputGroupHeader">
          <strong>执行设置</strong>
          <span className="workflowRunInputGroupTag neutral">可调整</span>
        </div>
      <section className="workflowRunParamSection">
        <strong>执行机器</strong>
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
        {runTarget !== RUN_TARGET_FOLLOW && (
          <>
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
          </>
        )}
      </section>
      </div>
      <div className="workflowRunInfoGroup" aria-label="运行摘要">
        <div className="workflowRunInputGroupHeader">
          <strong>运行摘要</strong>
          <span className="workflowRunInputGroupTag muted">只读预览</span>
        </div>
      <section className="workflowRunParamSection">
        <div className="workflowRunSummary">
          <span>工作流</span>
          <strong>{workflow.name || workflow.id || '未保存草稿'}</strong>
          <span>结构</span>
          <strong>{nodes.length} 个节点 · {edges.length} 条依赖{stages.length > 0 ? ` · ${stages.length} 个阶段` : ''}</strong>
          <span>执行机器</span>
          <strong>{runTargetLabel}</strong>
          <span>参数</span>
          <strong>{workflowParameters.length} 项工作流参数</strong>
          {nodesWithOwnTarget > 0 && (
            <>
              <span>单独指定机器</span>
              <strong>{nodes.filter(node => node.data?.host).map(node => node.data?.name || node.id).join('、')}</strong>
            </>
          )}
          {uploadFlowNodes(nodes).length > 0 && (
            <>
              <span>上传文件</span>
              <strong>{uploadFlowNodes(nodes).map(node => `${node.data?.name || node.id}：${uploadFileSummary(uploadNodeFiles, node.id)}`).join('；')}</strong>
            </>
          )}
          <span>失败恢复</span>
          <strong>{workflowRecoverySummary(workflow.recovery)}</strong>
          <span>结束动作</span>
          <strong>{workflowNotificationSummary(workflow.recovery?.notification)}</strong>
        </div>
        <div className="workflowSidebarHint">
          <span>
            {runTarget === RUN_TARGET_FOLLOW
              ? `沿用节点配置，${Math.max(nodes.length - nodesWithOwnTarget, 0)} 个节点在本机执行。`
              : runTargetOverride
                ? `全部 ${nodes.length} 个节点都会改到该目标执行。`
                : `${runTargetAppliesTo} 个未指定执行目标的节点会改到该目标执行。`}
          </span>
        </div>
        {hasUnsavedChanges && (
          <div className="workflowRunDraftNotice">当前有未保存修改，运行会使用画布上的最新内容。</div>
        )}
      </section>
      </div>
    </div>
  )
  const batchStageMoveModal = batchStageMove && (
    <div className="modalBackdrop" onClick={() => setBatchStageMove(null)}>
      <section className="modal workflowStageConvertModal" role="dialog" aria-modal="true" aria-label="批量移入阶段" onClick={event => event.stopPropagation()}>
        <div className="modalHeader">
          <div>
            <h3>批量移入阶段</h3>
            <p>{batchStageMove.selected.length} 个步骤移动到「{stages.find(stage => stage.id === batchStageMove.targetStageId)?.name || batchStageMove.targetStageId}」时与阶段顺序冲突</p>
          </div>
          <button type="button" className="modalClose" onClick={() => setBatchStageMove(null)}>×</button>
        </div>
        <ul className="workflowStageConvertReasons">
          {workflowModel.newStageDraftErrors(
            nodes,
            workflowModel.normalizeStageNodeOrders(workflowModel.moveNodesToStage(nodes, batchStageMove.selected, batchStageMove.targetStageId), stages),
            edges,
            stages,
          ).map(reason => <li key={reason}>{reason}</li>)}
        </ul>
        {batchStageMove.shadowed.length > 0 && (
          <p className="workflowSidebarHint">
            已排除 {batchStageMove.shadowed.map(id => nodes.find(node => node.id === id)?.data?.name || id).join('、')}：它们在目标阶段里已排在更靠前的位置。
          </p>
        )}
        <div className="workflowStageBatchTargets">
          {stages.filter(stage => stage.id !== batchStageMove.targetStageId).map(stage => (
            <button key={stage.id} type="button" className="secondary small" onClick={() => applyBatchStageMove(stage.id)}>
              改移到「{stage.name || stage.id}」
            </button>
          ))}
        </div>
        <div className="modalFooter">
          <button type="button" className="secondary" onClick={() => setBatchStageMove(null)}>取消</button>
        </div>
      </section>
    </div>
  )
  const canvasSelectionBar = isEditingCanvas && canvasSelectedNodeIDs.length > 0 ? (
    <div className="canvasSelectionBar nodrag nopan" role="status" onMouseDown={event => event.stopPropagation()}>
      <span className="canvasSelectionBarCount">已选 {canvasSelectedNodeIDs.length} 个步骤</span>
      <span className="workflowStageBatchMenuWrap">
        <button
          type="button"
          className="secondary small workflowStageBatchTarget"
          aria-haspopup="menu"
          aria-expanded={batchStageMenuOpen}
          disabled={isActiveRunRunning}
          onClick={() => setBatchStageMenuOpen(open => !open)}
        >
          移入阶段 ▾
        </button>
        {batchStageMenuOpen && (
          <div className="workflowStageBatchMenu" role="menu" aria-label="把选中的步骤移入阶段">
            {stages.length < 2 && <span className="workflowStageBatchMenuEmpty">请先添加其他阶段</span>}
            {stages.filter(stage => !canvasSelectedNodeIDs.every(id => nodes.find(node => node.id === id)?.data?.stageId === stage.id)).map(stage => (
              <button
                key={stage.id}
                type="button"
                role="menuitem"
                onClick={() => applyBatchStageMove(stage.id)}
              >{stage.name || stage.id}</button>
            ))}
          </div>
        )}
      </span>
      <button type="button" className="secondary small" onClick={clearCanvasSelection}>取消选择</button>
    </div>
  ) : null
  const workflowCanvas = (
    <section
      className={`card canvasCard${nodeCategoryFocus ? ` cat-focus-${nodeCategoryFocus}` : ''}`}
      ref={canvasCardRef}
      onDoubleClick={handlePaneDoubleClick}
      onDragOver={handleCanvasStageDragOver}
      onDrop={handleCanvasStageDrop}
    >
      {mixedStageNotice && (
        <div className="workflowLegacyNotice workflowMixedStageNotice" role="status">
          <div>
            <strong>节点未归属阶段</strong>
            <span>{mixedStageNotice.names.join('、')} 缺少阶段归属，保存前需要放到某个阶段。</span>
          </div>
        </div>
      )}
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
        onNodeDoubleClick={(_, node) => handleCanvasNodeDoubleClick(node.id)}
        onNodeDragStop={handleStageNodeDragStop}
        onSelectionChange={handleCanvasSelectionChange}
        onSelectionDragStop={handleStageSelectionDragStop}
        onEdgeClick={(_, edge) => openEdgeConfigModal(edge.id)}
        onEdgeDoubleClick={handleEdgeDoubleClick}
        onPaneClick={() => { clearSelection(); closeNodePicker() }}
        onInit={setFlowInstance}
        nodesDraggable={isEditingCanvas}
        nodesConnectable={isEditingCanvas}
        edgesReconnectable={isEditingCanvas}
        elementsSelectable={isEditingCanvas}
        deleteKeyCode={isEditingCanvas ? ['Backspace', 'Delete'] : null}
        zoomOnDoubleClick={false}
      >
        <MiniMap />
        <Background />
        {workflowStageLayouts.length > 0 && (selectedWorkflowID || draftStarted || nodes.length > 0) && (
          <ViewportPortal>
            <div className="workflowStageLayer" style={stageLayerSize ? {width: stageLayerSize.width, height: stageLayerSize.height} : undefined}>
              {workflowStageLayouts.map(stage => {
                const run = stageRunSummary(stage.id)
                const dragging = stageDragID === stage.id
                return (
                  <section
                    key={stage.id}
                    data-stage-id={stage.id}
                    className={`workflowStageColumn${run ? ` run-${run.kind}` : ''}${dragging ? ' stageDragging' : ''}${stageDropTargetID === stage.id ? ' stageDropTarget' : ''}`}
                    style={{left: stage.x, top: stage.y, width: stage.width, height: stage.height}}
                    aria-label={`阶段 ${stage.index + 1}：${stage.name}`}
                  >
                    <header
                      className={`workflowStageHeader nodrag nopan${isEditingCanvas && !isActiveRunRunning ? ' draggable' : ''}`}
                      draggable={isEditingCanvas && !isActiveRunRunning}
                      title={isEditingCanvas && !isActiveRunRunning ? '拖动阶段顶部调整顺序' : undefined}
                      onDragStart={event => handleStageDragStart(event, stage.id)}
                      onDragEnd={handleStageDragEnd}
                    >
                      <div className="workflowStageHeading">
                        <span
                          className="workflowStageIndex"
                          aria-hidden="true"
                        >{stage.index + 1}</span>
                        {isEditingCanvas ? (
                          <input
                            className="workflowStageNameInput nodrag nopan"
                            value={stage.name}
                            title={stage.name}
                            aria-label={`阶段 ${stage.index + 1} 名称`}
                            disabled={isActiveRunRunning}
                            onChange={event => renameWorkflowStage(stage.id, event.target.value)}
                            onBlur={event => { if (!event.target.value.trim()) renameWorkflowStage(stage.id, stage.id) }}
                          />
                        ) : (
                          <strong className="workflowStageName" title={stage.name}>{stage.name}</strong>
                        )}
                        {isEditingCanvas ? (
                          <span className="workflowStageModeControls">
                            <select
                              className="workflowStageModeSelect nodrag nopan"
                              value={workflowModel.stageExecutionMode(stage)}
                              aria-label={`阶段 ${stage.name} 执行模式`}
                              title={workflowModel.stageExecutionMode(stage) === 'parallel'
                                ? '并行：同阶段节点按显式依赖和并发上限同时执行'
                                : '串行：同阶段节点按画布顺序依次执行'}
                              disabled={isActiveRunRunning}
                              onChange={event => updateWorkflowStageMode(stage.id, event.target.value)}
                            >
                              {workflowModel.stageExecutionModes.map(mode => (
                                <option key={mode.value} value={mode.value}>{mode.label}</option>
                              ))}
                            </select>
                            {workflowModel.stageExecutionMode(stage) === 'parallel' && (
                              <input
                                className="workflowStageConcurrency nodrag nopan"
                                type="number"
                                min="1"
                                max={workflowModel.maxStageConcurrency}
                                value={stage.max_concurrency || ''}
                                aria-label={`阶段 ${stage.name} 最大并发数`}
                                title={`最大并发数，留空表示默认 4，范围 1-${workflowModel.maxStageConcurrency}`}
                                placeholder="默认 4"
                                disabled={isActiveRunRunning}
                                onChange={event => updateWorkflowStageConcurrency(stage.id, event.target.value)}
                              />
                            )}
                          </span>
                        ) : (
                          <span className={`workflowStageMeta${workflowModel.stageExecutionMode(stage) === 'parallel' ? ' parallel' : ''}`}>
                            {stage.nodeCount} 步 · {workflowModel.stageModeLabel(stage)}
                          </span>
                        )}
                        {run && <span className={`workflowStageRunStatus ${run.kind}`}>{run.label}</span>}
                      </div>
                      {isEditingCanvas && (
                        <div className="workflowStageActions">
                          <button
                            type="button"
                            className="workflowStageIconButton"
                            title={`把选中的 ${canvasSelectedNodeIDs.length} 个步骤移入本阶段`}
                            aria-label={`把选中的 ${canvasSelectedNodeIDs.length} 个步骤移入阶段 ${stage.name}`}
                            disabled={canvasSelectedNodeIDs.length === 0 || isActiveRunRunning}
                            onClick={() => applyBatchStageMove(stage.id)}
                          >⇥</button>
                          <button
                            type="button"
                            className="workflowStageIconButton accent"
                            title="在阶段末尾添加步骤"
                            aria-label={`在阶段 ${stage.name} 添加步骤`}
                            disabled={isActiveRunRunning}
                            onClick={event => openNodePickerForStage(stage.id, event)}
                          >＋</button>
                          <button
                            type="button"
                            className="workflowStageIconButton danger"
                            title="删除空阶段"
                            aria-label={`删除阶段 ${stage.name}`}
                            disabled={isActiveRunRunning}
                            onClick={() => deleteWorkflowStage(stage.id)}
                          >×</button>
                        </div>
                      )}
                    </header>
                    {stage.nodeCount === 0 && (
                      <div className="workflowStageEmptyHint">
                        {isEditingCanvas ? '把步骤拖到这里，或点击阶段右上角 ＋ 添加' : '该阶段没有步骤'}
                      </div>
                    )}
                    {stageMutationWarnings(stage.id).map(warning => (
                      <div key={warning} className="workflowStageWarning nodrag nopan" role="status">{warning}</div>
                    ))}
                  </section>
                )
              })}
              {isEditingCanvas && workflowStageAddPosition && (
                <button
                  type="button"
                  className="workflowStageAddColumn nodrag nopan"
                  style={{left: workflowStageAddPosition.x, top: workflowStageAddPosition.y}}
                  disabled={isActiveRunRunning}
                  onClick={addWorkflowStage}
                >
                  <span aria-hidden="true">＋</span>
                  <span>添加阶段</span>
                </button>
              )}
            </div>
          </ViewportPortal>
        )}
        {isEditingCanvas && !draftWindow && !selectedWorkflowID && !draftStarted && nodes.length === 0 && !nodePicker.open && (
          <div className="canvasEmptyCallout nodrag nopan" onMouseDown={event => event.stopPropagation()}>
            <strong>先选择一个工作流</strong>
            <span>从顶部名称下拉里打开已有工作流，或新建一个空白草稿再添加步骤。</span>
            <div className="canvasEmptyCalloutActions">
              <button type="button" className="primary" onClick={() => { setWorkflowMoreOpen(false); setWorkflowNameMenuOpen(true) }}>选择工作流</button>
              <button type="button" className="secondary" onClick={createWorkflow} disabled={isActiveRunRunning}>新建工作流</button>
            </div>
          </div>
        )}
        {isEditingCanvas && (selectedWorkflowID || draftStarted) && nodes.length === 0 && !nodePicker.open && (
          <div className="canvasEmptyCallout nodrag nopan" onMouseDown={event => event.stopPropagation()}>
            <strong>从添加节点开始编排</strong>
            <span>{stages.length > 0
              ? '点击阶段右上角的 ＋ 添加步骤，或双击画布空白处、拖出连线后选择下游节点。'
              : '点击右上角「添加节点」或双击画布空白处开始。'}</span>
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
            onAddTool={tool => addToolNode(tool, nodePicker.position, {connection: nodePicker.connection, insertEdge: nodePicker.insertEdge || pendingInsertEdge, stageId: nodePicker.stageId})}
            onAddControl={controlType => addControlNode(controlType, nodePicker.position, {connection: nodePicker.connection, insertEdge: nodePicker.insertEdge || pendingInsertEdge, stageId: nodePicker.stageId})}
            missingTools={missingToolEntries}
            onOpenPlugins={openPluginManager ? () => { closeNodePicker(); openPluginManager() } : undefined}
            onClose={closeNodePicker}
          />
        )}
        {canvasSelectionBar}
        {canvasCategoryLegend}
        <CanvasDock
          onAddNode={isEditingCanvas ? () => openNodePicker() : undefined}
          onZoomIn={() => zoomCanvas('in')}
          onZoomOut={() => zoomCanvas('out')}
          onFitView={fitCanvasView}
        />
      </ReactFlow>
    </section>
  )
  const runDock = runVisible ? (
    <section className={`workflowRunDock${runDockOpen ? ' open' : ''}`} aria-label="运行日志">
      <header className="workflowRunDockHeader">
        <div className="workflowRunDockSummary">
          <strong>运行日志</strong>
          <span className={`workflowRunStatusChip ${activeRunStatus}`}>{runStatusLabel(activeRunStatus)}</span>
          {activeRunID && <code title="运行 ID">{activeRunID}</code>}
          <span title="本次运行实际使用的执行目标">执行机器：{runMachineLabel}</span>
          {uploadProgress && <span>上传 {uploadProgress.percent}%</span>}
        </div>
        <div className="workflowRunDockActions">
          {activeRunID && !isActiveRunRunning && <button type="button" className="secondary" onClick={() => openRecoveryDrawer(activeRunID)}>恢复与通知</button>}
          {resultPanel && <button type="button" className="secondary" onClick={() => setRunHistoryOpen(true)}>完整日志</button>}
          <button type="button" className="secondary" onClick={() => setRunDockOpen(open => !open)}>{runDockOpen ? '收起' : '展开'}</button>
          {!isActiveRunRunning && (
            <button type="button" className="secondary" onClick={() => clearCanvasRunOverlay()}>关闭运行结果</button>
          )}
        </div>
      </header>
      {runDockOpen && (
        <div className="workflowRunDockBody">
          <nav className="workflowRunDockNodes" aria-label="运行节点列表" ref={runDockNavRef}>
            {runDockNodes.map(node => {
              const run = canvasRunState.nodes?.[node.id]
              return (
                <button
                  key={node.id}
                  type="button"
                  className={runDockNodeID === node.id ? 'active' : ''}
                  onClick={() => setRunDockNodeID(node.id)}
                >
                  <span className={`workflowRunStatusDot ${run?.status || 'waiting'}`} aria-hidden="true" />
                  <span className="workflowRunNodeName">{node.data?.name || node.id}</span>
                  <small>{run?.label || runStatusLabel(run?.status || 'waiting')}</small>
                </button>
              )
            })}
            {runDockNodes.length === 0 && <div className="empty small">本次运行没有可展示的节点。</div>}
          </nav>
          <div className="workflowRunDockDetail">
            {runDockNodeID ? (
              <CanvasNodeLogPanel
                embedded
                nodeID={runDockNodeID}
                item={runDockItem}
                run={canvasRunState.nodes?.[runDockNodeID]}
              />
            ) : (
              <div className="empty small">点击画布节点或左侧列表，查看对应的逐节点日志。</div>
            )}
          </div>
        </div>
      )}
    </section>
  ) : null
  return (
    <div className="workflowWorkspace">
      {workflowTopBar}
      {draftWindowTransferBanner}
      {workflowPendingBanner}
      {workflowStatusNotices}
      <div className="workflowCanvasArea">{workflowCanvas}</div>
      {runDock}
      {batchStageMoveModal}

      {saveModalOpen && (
        <div className="modalBackdrop" onClick={() => { if (workflowAction !== 'saving') setSaveModalOpen(false) }}>
          <section className={`modal workflowSaveModal${saveModalSettings ? ' workflowSettingsModal' : ''}`} role="dialog" aria-modal="true" aria-label={saveModalSettings ? '工作流设置' : '保存工作流'} onClick={event => event.stopPropagation()}>
            <div className="modalHeader">
              <div>
                <h3>{saveModalSettings ? '工作流设置' : '保存工作流'}</h3>
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
                <div className="workflowRecoveryFields">
                  <strong>失败恢复策略</strong>
                  <small className="workflowSelectionHint">已发生配置变更后失败时暂停为待处理；节点 on_failure=pending 时同样生效。未填写节点 ID 时按工具参数自动识别。</small>
                  <label className="checkboxLabel">
                    <input
                      type="checkbox"
                      checked={Boolean(workflow.recovery?.pending_on_failure)}
                      disabled={Boolean(workflowAction)}
                      onChange={event => updateWorkflowRecovery({pending_on_failure: event.target.checked})}
                    />
                    <span>变更后失败转为待处理</span>
                  </label>
                  <label className="checkboxLabel">
                    <input
                      type="checkbox"
                      checked={Boolean(workflow.recovery?.skip_change_on_no_diff)}
                      disabled={Boolean(workflowAction)}
                      onChange={event => updateWorkflowRecovery({skip_change_on_no_diff: event.target.checked})}
                    />
                    <span>差异检查未发现变更时跳过配置下发与服务重启（健康检查仍执行）</span>
                  </label>
                  <label>
                    <span>差异检查节点</span>
                    <input
                      value={workflow.recovery?.diff_node_id || ''}
                      disabled={Boolean(workflowAction)}
                      placeholder="例如 check"
                      onChange={event => updateWorkflowRecovery({diff_node_id: event.target.value})}
                    />
                  </label>
                  <label>
                    <span>变更节点（逗号分隔）</span>
                    <input
                      value={(workflow.recovery?.change_node_ids || []).join(', ')}
                      disabled={Boolean(workflowAction)}
                      placeholder="例如 deploy"
                      onChange={event => updateWorkflowRecovery({change_node_ids: splitNodeIDList(event.target.value)})}
                    />
                  </label>
                  <label>
                    <span>服务节点（逗号分隔）</span>
                    <input
                      value={(workflow.recovery?.service_node_ids || []).join(', ')}
                      disabled={Boolean(workflowAction)}
                      placeholder="例如 restart"
                      onChange={event => updateWorkflowRecovery({service_node_ids: splitNodeIDList(event.target.value)})}
                    />
                  </label>
                  <label>
                    <span>健康检查节点（逗号分隔）</span>
                    <input
                      value={(workflow.recovery?.health_node_ids || []).join(', ')}
                      disabled={Boolean(workflowAction)}
                      placeholder="留空时使用失败步骤本身"
                      onChange={event => updateWorkflowRecovery({health_node_ids: splitNodeIDList(event.target.value)})}
                    />
                  </label>
                </div>
                <div className="workflowRecoveryFields">
                  <strong>结束动作（Webhook）</strong>
                  <small className="workflowSelectionHint">运行结束后按状态发送通知；通知失败可单独重试，不会重新执行配置变更。</small>
                  <label>
                    <span>Webhook 地址</span>
                    <input
                      value={workflow.recovery?.notification?.url || ''}
                      disabled={Boolean(workflowAction)}
                      placeholder="https://example.com/hooks/workflow"
                      onChange={event => updateWorkflowNotification({url: event.target.value})}
                    />
                  </label>
                  <div className="workflowNotificationEvents" role="group" aria-label="通知触发状态">
                    {NOTIFICATION_EVENTS.map(event => {
                      const events = workflow.recovery?.notification?.events || []
                      const checked = events.length === 0 || events.includes(event)
                      return (
                        <button
                          key={event}
                          type="button"
                          className={checked ? 'workflowNotificationEvent active' : 'workflowNotificationEvent'}
                          aria-pressed={checked}
                          disabled={Boolean(workflowAction)}
                          onClick={() => toggleWorkflowNotificationEvent(event)}
                        >
                          {notificationEventLabel(event)}
                        </button>
                      )
                    })}
                  </div>
                  <label>
                    <span>超时</span>
                    <input
                      value={workflow.recovery?.notification?.timeout || ''}
                      disabled={Boolean(workflowAction)}
                      placeholder="10s"
                      onChange={event => updateWorkflowNotification({timeout: event.target.value})}
                    />
                  </label>
                </div>
              </details>
            </div>
            <div className="modalFooter">
              <button className="secondary" type="button" onClick={() => setSaveModalOpen(false)} disabled={workflowAction === 'saving'}>取消</button>
              <button className="primary" type="button" onClick={saveDraft} disabled={Boolean(workflowAction)}>{workflowAction === 'saving' ? '保存中...' : saveModalSettings ? '保存设置' : '保存'}</button>
            </div>
          </section>
        </div>
      )}

      {runSetupOpen && (
        <div className="modalBackdrop drawerBackdrop" onClick={() => { if (!isActiveRunRunning) setRunSetupOpen(false) }}>
          <aside className="nodeConfigDrawer workflowRunSetupDrawer" role="dialog" aria-modal="true" aria-label="运行准备" onClick={event => event.stopPropagation()}>
            <div className="modalHeader nodeConfigDrawerHeader">
              <div>
                <h3>运行工作流</h3>
                <p>{workflow.name || workflow.id || '未保存草稿'} · {nodes.length} 个节点 · 执行机器：{runTargetLabel}</p>
              </div>
              <button type="button" className="modalClose" onClick={() => setRunSetupOpen(false)} disabled={isActiveRunRunning}>×</button>
            </div>
            <div className="nodeConfigDrawerBody">
              {editorValidation?.errors?.length > 0 && <ValidationSummary status={editorValidation} />}
              {workflowRunParamsContent}
            </div>
            <div className="modalFooter nodeConfigDrawerFooter workflowRunSetupFooter">
              <button className="secondary" type="button" onClick={() => setRunSetupOpen(false)} disabled={isActiveRunRunning}>取消</button>
              <button className="secondary" type="button" onClick={validateDraft} disabled={Boolean(workflowAction)}>{workflowAction === 'validating' ? '校验中...' : '校验'}</button>
              <button className="primary" type="button" onClick={runDraft} disabled={isActiveRunRunning} aria-label="开始运行工作流">开始运行</button>
            </div>
          </aside>
        </div>
      )}

      {runHistoryOpen && resultPanel && (
        <div className="modalBackdrop drawerBackdrop" onClick={() => setRunHistoryOpen(false)}>
          <aside className="nodeConfigDrawer workflowRunHistoryDrawer" role="dialog" aria-modal="true" aria-label="运行日志" onClick={event => event.stopPropagation()}>
            <div className="modalHeader nodeConfigDrawerHeader">
              <div>
                <h3>运行日志</h3>
                <p>{activeRunID ? `运行 ID：${activeRunID} · ${runStatusLabel(activeRunStatus)}` : '当前工作流最近一次执行结果'}</p>
              </div>
              <button type="button" className="modalClose" onClick={() => setRunHistoryOpen(false)}>×</button>
            </div>
            <div className="nodeConfigDrawerBody workflowRunHistoryBody">
              <div className="workflowRunDraftNotice">运行结果为只读快照，不会修改当前画布草稿。</div>
              {resultPanel}
            </div>
          </aside>
        </div>
      )}

      {recoveryRunID && (
        <div className="modalBackdrop drawerBackdrop" onClick={closeRecoveryDrawer}>
          <aside className="nodeConfigDrawer workflowRecoveryDrawer" role="dialog" aria-modal="true" aria-label="待处理恢复" onClick={event => event.stopPropagation()}>
            <div className="modalHeader nodeConfigDrawerHeader">
              <div>
                <h3>待处理恢复</h3>
                <p>{recoveryRunID}</p>
              </div>
              <button type="button" className="modalClose" onClick={closeRecoveryDrawer} disabled={Boolean(recoveryAction)}>×</button>
            </div>
            <div className="nodeConfigDrawerBody workflowRecoveryBody">
              {recoveryLoading && <div className="empty small">正在读取待处理运行...</div>}
              {recoveryError && <div className="workflowRecoveryMessage error" role="alert">{recoveryError}</div>}
              {recoveryNotice && <div className="workflowRecoveryMessage ok" role="status">{recoveryNotice}</div>}
              <RecoveryPanel
                detail={recoveryDetail}
                busyAction={recoveryAction}
                notificationBusy={notificationBusy}
                onCheck={() => executeRecoveryAction('check')}
                onRollback={() => executeRecoveryAction('rollback')}
                onResolve={note => executeRecoveryAction('resolve', note)}
                onRetryNotification={retryRecoveryNotification}
              />
            </div>
          </aside>
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
            onScriptChange={updateSelectedNodeScript}
            onChecksumChange={updateSelectedNodeChecksum}
            onArchiveChange={updateSelectedNodeArchive}
            onWaitChange={updateSelectedNodeWait}
            onNotifyChange={updateSelectedNodeNotify}
            onHostChange={updateSelectedNodeHost}
            loopTool={selectedLoopTool}
            nodes={nodes}
            tools={toolOptions}
            stages={stages}
            stageDisabled={isActiveRunRunning}
            onStageChange={moveSelectedNodeToStage}
            onStageMove={moveSelectedNodeInStage}
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

      {workflowTransferOpen && (
        <WorkflowTransferModal
          onClose={() => setWorkflowTransferOpen(false)}
          onImported={async () => {
            await refreshCatalog({keepCategory: true})
          }}
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

function sameIDList(left, right) {
  if (left.length !== right.length) return false
  const leftSet = new Set(left)
  return right.every(id => leftSet.has(id))
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

function readRecentWorkflowIDs() {
  try {
    const parsed = JSON.parse(window.localStorage?.getItem(RECENT_WORKFLOW_KEY) || '[]')
    return Array.isArray(parsed) ? parsed.filter(id => typeof id === 'string' && id) : []
  } catch {
    return []
  }
}

function pushRecentWorkflowID(id) {
  if (!id) return readRecentWorkflowIDs()
  const next = [id, ...readRecentWorkflowIDs().filter(item => item !== id)].slice(0, RECENT_WORKFLOW_LIMIT)
  try {
    window.localStorage?.setItem(RECENT_WORKFLOW_KEY, JSON.stringify(next))
  } catch {
    // Ignore storage failures; recent workflows only speed up switching.
  }
  return next
}

function forgetRecentWorkflowID(id) {
  const next = readRecentWorkflowIDs().filter(item => item !== id)
  try {
    window.localStorage?.setItem(RECENT_WORKFLOW_KEY, JSON.stringify(next))
  } catch {
    // Ignore storage failures.
  }
  return next
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

export function CanvasNodeLogPanel({nodeID, item, run, minimized, onMinimize, onRestore, onClose, embedded = false}) {
  const stdout = item?.stdout || ''
  const stderr = item?.stderr || run?.error || run?.skippedReason || ''
  const title = item?.title || nodeID
  const status = item?.status || run?.status || 'waiting'
  if (embedded) {
    return (
      <section className="canvasNodeLogPanel embedded">
        <div className="canvasNodeLogHeader">
          <div>
            <strong>{title}</strong>
            <span>{runStatusLabel(status)}</span>
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

const NOTIFICATION_EVENTS = ['succeeded', 'failed', 'pending', 'rolled_back']

const attemptKindLabel = {check: '重试检查', rollback: '回滚配置', resolve: '结束处理'}
const attemptStatusLabel = {succeeded: '成功', failed: '失败', running: '执行中'}

// RecoveryPanel 只渲染运行快照驱动的只读证据和恢复入口，不读取当前画布。
export function RecoveryPanel({detail, busyAction = '', notificationBusy = '', onCheck, onRollback, onResolve, onRetryNotification}) {
  const [resolveNote, setResolveNote] = useState('')
  const record = detail?.record || null
  const pending = record?.pending || null
  const backups = record?.backups || []
  const attempts = record?.attempts || []
  const notifications = record?.notifications || []
  const attemptLogs = detail?.logs?.attempts || {}
  const snapshotWorkflow = record?.snapshot?.workflow || null
  const failedStepID = pending?.step_id || ''
  const failedStep = (record?.steps || []).find(step => step.id === failedStepID) || null
  const failedNode = (snapshotWorkflow?.nodes || []).find(node => node.id === failedStepID) || null
  const failedStage = (snapshotWorkflow?.stages || []).find(stage => stage.id === failedNode?.stage_id) || null
  const failedLogItem = findRunLogItem(detail?.logs?.items || [], failedStepID)
  const lastAttempt = attempts.length > 0 ? attempts[attempts.length - 1] : null
  const busy = Boolean(busyAction || notificationBusy)
  const pendingActive = normalizeRunStatus(record?.status) === 'pending' && pending?.status === 'pending'
  const rollbackBackups = backups.filter(backup => backup.status !== 'skipped')
  const blockedBackup = rollbackBackups.find(backup => backup.status === 'created' || !backup.backup_path || !backup.sha256)
  const rollbackDisabledReason = rollbackBackups.length === 0
    ? '本次运行没有记录可回滚的配置备份。'
    : blockedBackup?.status === 'created'
      ? '本次变更为新建文件，默认不自动删除，请按路径人工处理。'
      : blockedBackup
        ? '本次备份缺少路径或下发后的 SHA256，无法安全确认目标状态，请人工处理。'
        : ''
  if (!record) {
    return (
      <div className="empty small">没有读取到运行快照，恢复操作不可用。请稍后刷新或从运行记录中重新打开。</div>
    )
  }
  const lastOperationLabel = lastAttempt
    ? `${attemptKindLabel[lastAttempt.kind] || lastAttempt.kind} · ${attemptStatusLabel[lastAttempt.status] || lastAttempt.status} · ${formatRunTimestamp(lastAttempt.ended_at || lastAttempt.started_at)}`
    : '尚未执行恢复操作'
  return (
    <div className="workflowRecoveryPanel">
      <section className="workflowRecoverySection">
        <div className="workflowRecoveryStatusRow">
          <span className={`workflowRunStatusChip ${normalizeRunStatus(record.status)}`}>{runStatusLabel(record.status)}</span>
          {pending && <span className={`workflowRecoveryPendingChip ${pending.status}`}>待处理状态：{pendingStateLabel(pending.status)}</span>}
          <code>{record.id}</code>
          <span>开始：{formatRunTimestamp(record.started_at) || '未知'}</span>
        </div>
        {pending?.reason && <div className="workflowRecoveryAlert">{pending.reason}</div>}
        {!pendingActive && (
          <div className="workflowRecoveryAlert closed">
            {normalizeRunStatus(record.status) === 'rolled_back'
              ? `已回滚，服务已恢复。${pending?.resolution || ''}原变更失败的结论仍保留在下方原始步骤中。`
              : pending?.resolution || '该运行已不在待处理状态，只能查看历史证据。'}
          </div>
        )}
        <dl className="workflowRecoveryFacts">
          <div>
            <dt>失败阶段</dt>
            <dd>{failedStage ? `${failedStage.name || failedStage.id}（${failedStage.id}）` : failedNode?.stage_id || '未记录'}</dd>
          </div>
          <div>
            <dt>失败步骤</dt>
            <dd>{failedStepID ? `${failedNode?.name || failedStepID}（${failedStepID}）` : '未记录'}</dd>
          </div>
          <div>
            <dt>实际机器</dt>
            <dd>{describeRunMachines(record, [], [], '') || '本机'}</dd>
          </div>
          <div>
            <dt>错误摘要</dt>
            <dd className="wrap">{record.error || failedStep?.error || pending?.reason || '无错误摘要'}</dd>
          </div>
          <div>
            <dt>当前检查结果</dt>
            <dd className="wrap">{failedStep ? `${runStatusLabel(failedStep.status)}${failedStep.error ? `：${failedStep.error}` : ''}` : '未记录失败步骤结果'}</dd>
          </div>
          <div>
            <dt>目标配置</dt>
            <dd className="wrap">{backups.length > 0 ? backups.map(backup => backup.target_path || '未知路径').join('、') : '本次运行未记录配置目标'}</dd>
          </div>
          <div>
            <dt>最后一次操作</dt>
            <dd className="wrap">{lastOperationLabel}</dd>
          </div>
        </dl>
        {pendingActive && (
          <div className="workflowRecoveryActions">
            <button type="button" className="primary" disabled={busy} onClick={onCheck} title="只重跑只读检查，不重复配置下发或服务重启">
              {busyAction === 'check' ? '检查中...' : '重试检查'}
            </button>
            <button type="button" className="secondary danger" disabled={busy || Boolean(rollbackDisabledReason)} onClick={onRollback} title={rollbackDisabledReason || '按本次运行的备份恢复配置并重启关联服务'}>
              {busyAction === 'rollback' ? '回滚中...' : '回滚配置'}
            </button>
            <div className="workflowRecoveryResolve">
              <input value={resolveNote} disabled={busy} placeholder="结束处理说明（可选）" onChange={event => setResolveNote(event.target.value)} />
              <button type="button" className="secondary" disabled={busy} onClick={() => onResolve(resolveNote.trim())}>
                {busyAction === 'resolve' ? '处理中...' : '结束处理'}
              </button>
            </div>
            {rollbackDisabledReason && <div className="workflowRecoveryBlocked">{rollbackDisabledReason}</div>}
          </div>
        )}
      </section>

      <section className="workflowRecoverySection">
        <div className="workflowRecoverySectionHeader">
          <strong>本次备份</strong>
          <span>{backups.length} 个目标</span>
        </div>
        {backups.length === 0 && <div className="empty small">没有备份记录，无法一键回滚；请根据下方日志人工核对目标文件。</div>}
        {backups.length > 0 && (
          <table className="workflowRecoveryTable">
            <thead>
              <tr><th>节点</th><th>目标路径</th><th>备份</th><th>状态</th><th>SHA256</th></tr>
            </thead>
            <tbody>
              {backups.map((backup, index) => (
                <tr key={`${backup.node_id}-${index}`}>
                  <td title={backup.host_name || backup.host_id || '本机'}>
                    <strong>{backup.node_id || backup.tool || '未知节点'}</strong>
                    <small>{backup.host_name || backup.host_id || '本机'}</small>
                  </td>
                  <td className="wrap"><code>{backup.target_path || '未知'}</code></td>
                  <td className="wrap"><code>{backup.backup_path || '无'}</code></td>
                  <td>
                    {backupStatusLabel(backup.status)}
                    {backup.recorded_at && <small>记录于 {formatRunTimestamp(backup.recorded_at)}</small>}
                    {backup.restored_sha256 && <small>{pendingActive ? '文件已还原，待完成服务与健康检查' : '文件已还原'}</small>}
                  </td>
                  <td className="wrap"><code>{backup.restored_sha256 || backup.sha256 || '无'}</code></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      {failedLogItem && (
        <section className="workflowRecoverySection">
          <div className="workflowRecoverySectionHeader">
            <strong>原始失败日志</strong>
            <span>{failedLogItem.title || failedStepID}</span>
          </div>
          <CanvasNodeLogPanel embedded nodeID={failedStepID} item={failedLogItem} run={null} />
        </section>
      )}

      <section className="workflowRecoverySection">
        <div className="workflowRecoverySectionHeader">
          <strong>恢复记录</strong>
          <span>每次操作独立保存，不覆盖原始日志</span>
        </div>
        {attempts.length === 0 && <div className="empty small">尚未执行恢复操作。</div>}
        {attempts.map(attempt => {
          const logs = attemptLogs[attempt.id] || {}
          return (
            <details key={attempt.id} className={`workflowRecoveryAttempt ${attempt.status}`} open={attempt === lastAttempt}>
              <summary>
                <span className={`workflowRecoveryAttemptKind ${attempt.kind}`}>{attemptKindLabel[attempt.kind] || attempt.kind}</span>
                <strong>{attemptStatusLabel[attempt.status] || attempt.status}</strong>
                <span>{formatRunTimestamp(attempt.started_at)}</span>
                {attempt.error && <em title={attempt.error}>{attempt.error}</em>}
              </summary>
              {attempt.detail && <div className="workflowRecoveryAttemptDetail">{attempt.detail}</div>}
              {logs.summary && <InlineLogBlock title="执行摘要" value={logs.summary} />}
              {logs.stdout && Object.entries(logs.stdout).map(([key, value]) => (
                <InlineLogBlock key={`out-${key}`} title={`标准输出 · ${key}`} value={value} />
              ))}
              {logs.stderr && Object.entries(logs.stderr).map(([key, value]) => (
                <InlineLogBlock key={`err-${key}`} title={`错误输出 · ${key}`} value={value} />
              ))}
            </details>
          )
        })}
      </section>

      <section className="workflowRecoverySection">
        <div className="workflowRecoverySectionHeader">
          <strong>结束动作通知</strong>
          <span>{notifications.length} 条记录</span>
        </div>
        {notifications.length === 0 && <div className="empty small">该运行没有触发通知。可在工作流高级设置中配置 Webhook 与触发事件。</div>}
        {notifications.map((notification, index) => (
          <div key={`${notification.event}-${index}`} className={`workflowRecoveryNotification ${notification.status}`}>
            <div>
              <strong>{notificationEventLabel(notification.event)}</strong>
              <small>{notification.url}</small>
            </div>
            <span>{notification.status === 'sent' ? '已发送' : notification.status === 'failed' ? '发送失败' : notification.status}</span>
            <span>{notification.attempts} 次</span>
            <span>{formatRunTimestamp(notification.updated_at || notification.sent_at)}</span>
            {notification.error && <em title={notification.error}>{notification.error}</em>}
            {notification.status !== 'sent' && (
              <button
                type="button"
                className="secondary"
                disabled={busy || Boolean(notificationBusy)}
                onClick={() => onRetryNotification(index)}
              >
                {notificationBusy === String(index) ? '重试中...' : '重试通知'}
              </button>
            )}
          </div>
        ))}
      </section>
    </div>
  )
}

function pendingStateLabel(status) {
  if (status === 'pending') return '等待处理'
  if (status === 'resolved') return '检查通过'
  if (status === 'rolled_back') return '已回滚'
  if (status === 'closed') return '已结束处理'
  return status || '未知'
}

function backupStatusLabel(status) {
  if (status === 'created') return '新建文件（无备份）'
  if (status === 'skipped') return '无差异跳过'
  if (status === 'updated') return '已更新（有备份）'
  return status || '已记录'
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

// 会在某台机器上执行命令的内置控制节点；HTTP/等待/解压/通知在本机或平台进程内完成。
const hostBoundControlTypes = new Set(['loop', 'shell_script', 'file_checksum'])
const localControlNodeTypes = new Set(['http_request', 'archive', 'wait', 'notify', 'condition', 'parallel', 'join', 'upload', 'extract_config'])

export function executionHostForNode(node, runTarget = RUN_TARGET_FOLLOW, overrideNodes = false) {
  if (node?.type !== 'toolNode' && !hostBoundControlTypes.has(node?.data?.controlType)) return ''
  const ownTarget = node?.data?.host || ''
  if (runTarget === RUN_TARGET_FOLLOW || (ownTarget && !overrideNodes)) return ownTarget
  return runTarget === RUN_TARGET_LOCAL || runTarget === '__local__' ? '' : runTarget
}

export function describeRunMachines(record, snapshotNodes, hosts, fallback) {  const targets = new Map()
  ;(snapshotNodes || []).forEach(node => {
    const controlType = node.data?.controlType || ''
    if (node.type === 'toolNode') targets.set(node.data?.host || '', '')
    else if (hostBoundControlTypes.has(controlType)) targets.set(node.data?.host || '', '')
  })
  ;(record?.steps || []).forEach(step => {
    if (step.status === 'skipped' || (!hostBoundControlTypes.has(step.type) && !localControlNodeTypes.has(step.type) && !['tool', 'loop'].includes(step.type))) return
    targets.set(step.host_id || '', step.host_name || '')
  })
  return Array.from(targets, ([id, name]) => {
    if (!id || id === RUN_TARGET_LOCAL || id === '__local__') return '本机'
    return name || (hosts || []).find(host => host.id === id)?.name || id
  }).join('、') || fallback
}

function firstRunDockNodeID(nodes, runState) {
  const entries = Object.entries(runState?.nodes || {})
  const preferred = entries.find(([, item]) => item?.status === 'running') || entries.find(([, item]) => item?.status === 'failed')
  if (preferred) return preferred[0]
  return entries[0]?.[0] || nodes?.[0]?.id || ''
}

export function buildRunningCanvasRunState(nodes, edges = [], stages = []) {
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
  const entries = stages.length > 0
    ? workflowModel.orderedWorkflowNodes(nodes, stages).slice(0, 1)
    : findEntryNodes(nodes, edges)
  entries.forEach(node => {
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

export function buildRerunCanvasRunState(nodes, edges, currentState, nodeID, stages = [], recovery = {}, tools = [], snapshotNodes = []) {
  const overlay = {
    ...emptyCanvasRunState(),
    status: 'running',
    message: `正在从节点 ${nodeID} 开始重跑。`,
    nodes: {...(currentState?.nodes || {})},
    conditionMatches: {...(currentState?.conditionMatches || {})}
  }
  const selected = nodes.find(node => node.id === nodeID)
  const stageIndex = stages.findIndex(stage => stage.id === workflowModel.nodeStageID(selected))
  let executionEdges = edges
  if (stageIndex >= 0 && workflowModel.stageExecutionMode(stages[stageIndex]) === 'parallel') {
    const toolMap = new Map(tools.map(tool => [tool.id, tool]))
    const changes = new Set(recovery?.change_node_ids?.length ? recovery.change_node_ids : nodes.filter(node => {
      const tool = toolMap.get(node.data?.tool)
      return tool?.parameters?.some(parameter => parameter.name === 'backup') && tool?.outputs?.some(output => output.name === 'backup_path')
    }).map(node => node.id))
    const services = new Set(recovery?.service_node_ids?.length ? recovery.service_node_ids : nodes.filter(node => {
      const parameters = toolMap.get(node.data?.tool)?.parameters || []
      return parameters.some(parameter => parameter.name === 'service') && parameters.some(parameter => parameter.name === 'action')
    }).map(node => node.id))
    const hostByID = new Map((snapshotNodes || []).map(node => [node.id, node.host || '']))
    const ordered = workflowModel.orderedStageNodes(workflowModel.renumberStageForSerial(nodes, edges, stages[stageIndex].id), stages[stageIndex].id)
    const lastByHost = new Map()
    executionEdges = [...edges]
    ordered.forEach(node => {
      if (!changes.has(node.id) && !services.has(node.id)) return
      const target = String(hostByID.has(node.id) ? hostByID.get(node.id) : node.data?.host || '').trim()
      const host = target === '__local__' ? '' : target
      const previous = lastByHost.get(host)
      if (previous) executionEdges.push({source: previous, target: node.id})
      lastByHost.set(host, node.id)
    })
  }
  const affected = reachableNodeIDs(nodeID, executionEdges)
  if (stageIndex >= 0) {
    if (workflowModel.stageExecutionMode(stages[stageIndex]) !== 'parallel') {
      workflowModel.orderedStageNodes(nodes, stages[stageIndex].id).forEach(node => {
        if (workflowModel.nodeStageOrder(node) >= workflowModel.nodeStageOrder(selected)) affected.add(node.id)
      })
    }
    stages.slice(stageIndex + 1).forEach(stage => workflowModel.orderedStageNodes(nodes, stage.id).forEach(node => affected.add(node.id)))
  }
  affected.forEach(id => {
    const status = id === nodeID ? 'running' : 'waiting'
    overlay.nodes[id] = {
      status,
      label: runStatusLabel(status),
      title: id === nodeID ? '正在重跑该节点' : '等待上游重跑完成'
    }
    delete overlay.conditionMatches[id]
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
  const rank = {failed: 5, pending: 6, rolled_back: 6, running: 4, skipped: 3, succeeded: 2, idle: 1}
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
      data: {...(edge.data || {}), case: edgeCase || undefined}
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

// 工作流引用了未注册的工具时，catalog 只给出 missing_tools 列表；挑选节点时把这些工具连同原因一起展示，
// 避免用户以为插件工具列表是完整的。
function missingToolDependencies(catalog, searchText) {
  const keyword = String(searchText || '').trim().toLowerCase()
  const entries = []
  const seen = new Set()
  for (const workflow of catalog?.workflows || []) {
    for (const toolID of workflow.missing_tools || []) {
      if (!toolID || seen.has(toolID)) continue
      seen.add(toolID)
      entries.push({
        id: toolID,
        name: toolID,
        reason: `插件未安装或未注册，引用它的工作流「${workflow.name || workflow.id}」无法运行；安装或启用对应插件后即可添加`
      })
    }
    for (const dependency of workflow.dependencies || []) {
      if (!dependency?.id || seen.has(dependency.id) || !dependency.disabled) continue
      seen.add(dependency.id)
      entries.push({
        id: dependency.id,
        name: dependency.name || dependency.id,
        reason: `插件「${dependency.name || dependency.id}」已禁用，工作流「${workflow.name || workflow.id}」中的工具暂不可用；启用插件后即可添加`
      })
    }
  }
  if (!keyword) return entries
  return entries.filter(item => `${item.id} ${item.name} ${item.reason}`.toLowerCase().includes(keyword))
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

export function buildDisplayNodes(nodes, runState, tools = [], canRemove = true, onRerun = null, activeRunID = '', hosts = [], logNodeID = '') {
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
      className: [node.className, 'cat-node', `cat-${categoryID}`, logNodeID === node.id ? 'logFocused' : ''].filter(Boolean).join(' '),
      data: {
        ...node.data,
        categoryID,
        run: nodeRun,
        hostLabel,
        onRemove: canRemove ? node.data?.onRemove : null,
        onRerun: onRerun && activeRunID ? onRerun : null,
        rerunDisabled: runState?.status === 'running',
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

function normalizePendingRunSummaries(runs) {
  return (runs || [])
    .filter(item => item && item.kind === 'workflow' && normalizeRunStatus(item.status) === 'pending')
    .sort((left, right) => new Date(right.started_at || 0) - new Date(left.started_at || 0))
}

function formatRunTimestamp(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString()
}

export function workflowRecoverySummary(recovery) {
  const parts = []
  if (recovery?.pending_on_failure) parts.push('已变更后失败转为待处理')
  if (recovery?.skip_change_on_no_diff) parts.push('无差异跳过配置下发')
  if (recovery?.diff_node_id) parts.push(`差异检查：${recovery.diff_node_id}`)
  if ((recovery?.change_node_ids || []).length > 0) parts.push(`变更节点：${recovery.change_node_ids.join('、')}`)
  if ((recovery?.service_node_ids || []).length > 0) parts.push(`服务节点：${recovery.service_node_ids.join('、')}`)
  if ((recovery?.health_node_ids || []).length > 0) parts.push(`检查节点：${recovery.health_node_ids.join('、')}`)
  return parts.length > 0 ? parts.join('；') : '按节点 on_failure 与变更备份自动判断'
}

export function workflowNotificationSummary(notification) {
  if (!notification?.url) return '未配置 Webhook 通知'
  const events = (notification.events || []).length > 0 ? notification.events.map(notificationEventLabel).join('、') : '成功、失败、待处理、已回滚'
  return `Webhook ${notification.url}（${events}）`
}

export function notificationEventLabel(event) {
  if (event === 'succeeded') return '成功'
  if (event === 'failed') return '失败'
  if (event === 'pending') return '待处理'
  if (event === 'rolled_back') return '已回滚'
  return event
}

export function splitNodeIDList(value) {
  const seen = new Set()
  const out = []
  String(value || '').split(/[,，\s]+/).forEach(item => {
    const id = item.trim()
    if (!id || seen.has(id)) return
    seen.add(id)
    out.push(id)
  })
  return out
}

export function pruneWorkflowRecovery(recovery) {
  if (!recovery) return null
  const next = {}
  if (recovery.pending_on_failure) next.pending_on_failure = true
  if (recovery.skip_change_on_no_diff) next.skip_change_on_no_diff = true
  if (recovery.diff_node_id) next.diff_node_id = String(recovery.diff_node_id).trim()
  ;['change_node_ids', 'service_node_ids', 'health_node_ids'].forEach(key => {
    const values = (recovery[key] || []).map(item => String(item).trim()).filter(Boolean)
    if (values.length > 0) next[key] = values
  })
  const notification = recovery.notification || {}
  const url = String(notification.url || '').trim()
  const timeout = String(notification.timeout || '').trim()
  const events = (notification.events || []).filter(event => NOTIFICATION_EVENTS.includes(event))
  const headers = notification.headers && Object.keys(notification.headers).length > 0 ? notification.headers : null
  if (url || timeout || events.length > 0 || headers) {
    next.notification = {}
    if (url) next.notification.url = url
    if (events.length > 0) next.notification.events = events
    if (headers) next.notification.headers = headers
    if (timeout) next.notification.timeout = timeout
  }
  return Object.keys(next).length > 0 ? next : null
}

function emptyWorkflow(category = 'global') {  return {
    id: '',
    name: '',
    description: '',
    version: '1.0.0',
    category,
    tags: [],
    shared_with: [],
    parameters: [],
    stages: workflowModel.defaultWorkflowStages(),
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
  if (control.type === 'shell_script') data.script = workflowModel.defaultScriptConfig()
  if (control.type === 'file_checksum') data.checksum = workflowModel.defaultChecksumConfig()
  if (control.type === 'archive') data.archive = workflowModel.defaultArchiveConfig()
  if (control.type === 'wait') data.wait = workflowModel.defaultWaitConfig()
  if (control.type === 'notify') data.notify = workflowModel.defaultNotifyConfig()
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
  const nodeType = workflowModel.controlNodeTypeFromWorkflow(node.type || (node.tool ? 'tool' : node.loop ? 'loop' : 'condition'))
  const stageId = node.stage_id || ''
  const order = Number(node.order) || 0
  if (nodeType === 'condition') {
    return {
      id: node.id,
      type: 'conditionNode',
      data: {
        name: node.name || node.id,
        sourceConfig: node,
        condition: node.condition || defaultCondition(),
        stageId,
        order,
        onRemove
      },
      position: {x: 80 + index * 220, y: 120 + (index % 3) * 90}
    }
  }
  if (nodeType === 'parallel' || nodeType === 'join' || nodeType === 'loop' || nodeType === 'upload' || nodeType === 'extract_config' || nodeType === 'http_request' || nodeType === 'shell_script' || nodeType === 'file_checksum' || nodeType === 'archive' || nodeType === 'wait' || nodeType === 'notify') {
    const control = controlNodeCatalog.find(item => item.type === nodeType) || {type: nodeType, title: controlNodeTitle(nodeType)}
    const flowNode = newControlFlowNode(control, node.id, {x: 80 + index * 220, y: 120 + (index % 3) * 90}, onRemove)
    flowNode.data.name = node.name || node.id
    flowNode.data.sourceConfig = node
    flowNode.data.stageId = stageId
    flowNode.data.order = order
    if (hostBoundControlTypes.has(nodeType)) flowNode.data.host = node.host || ''
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
    if (nodeType === 'upload') flowNode.data.upload = normalizeUploadConfig(node.upload || defaultUpload())
    if (nodeType === 'extract_config') flowNode.data.extract_config = normalizeExtractConfig(node.extract_config || defaultExtractConfig())
    if (nodeType === 'http_request') flowNode.data.http = workflowModel.normalizeHttpRequestConfig(node.http || workflowModel.defaultHttpRequest())
    if (nodeType === 'shell_script') flowNode.data.script = workflowModel.normalizeScriptConfig(node.script || workflowModel.defaultScriptConfig())
    if (nodeType === 'file_checksum') flowNode.data.checksum = workflowModel.normalizeChecksumConfig(node.checksum || workflowModel.defaultChecksumConfig())
    if (nodeType === 'archive') flowNode.data.archive = workflowModel.normalizeArchiveConfig(node.archive || workflowModel.defaultArchiveConfig())
    if (nodeType === 'wait') flowNode.data.wait = node.wait ? workflowModel.normalizeWaitConfig(node.wait) : workflowModel.defaultWaitConfig()
    if (nodeType === 'notify') flowNode.data.notify = node.notify ? workflowModel.normalizeNotifyConfig(node.notify) : workflowModel.defaultNotifyConfig()
    return flowNode
  }
  return {
    id: node.id,
    type: 'toolNode',
    data: {
      tool: node.tool,
      sourceConfig: node,
      name: node.name || node.id,
      host: node.host || '',
      params: node.params || {},
      on_failure: node.on_failure || 'stop',
      stageId,
      order,
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
    data: {sourceConfig: edge, ...(edgeCase ? {case: edgeCase} : {})}
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
  const layerGapX = 120
  const layerGapY = 28
  const layerHeights = orderedLayers.map(layerNodes => layerNodes.length * workflowModel.WORKFLOW_NODE_HEIGHT + Math.max(0, layerNodes.length - 1) * layerGapY)
  const maxLayerHeight = Math.max(...layerHeights, 0)
  const positions = new Map()
  let x = 80

  orderedLayers.forEach((layerNodes, layerIndex) => {
    let y = 80 + Math.max(0, (maxLayerHeight - layerHeights[layerIndex]) / 2)
    layerNodes.forEach(node => {
      positions.set(node.id, {x, y})
      y += workflowModel.WORKFLOW_NODE_HEIGHT + layerGapY
    })
    x += workflowModel.WORKFLOW_NODE_WIDTH + layerGapX
  })

  return nodes.map(node => ({
    ...node,
    position: positions.get(node.id) || node.position || {x: 80, y: 80}
  }))
}

function autoLayoutNodeSize(node) {
  const measured = node?.measured || {}
  if (measured.width > 0 && measured.height > 0) return {width: measured.width, height: measured.height}
  return {width: workflowModel.WORKFLOW_NODE_WIDTH, height: workflowModel.WORKFLOW_NODE_HEIGHT}
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
      http: node.data.http ? clonePlainValue(node.data.http) : node.data.http,
      script: node.data.script ? clonePlainValue(node.data.script) : node.data.script,
      checksum: node.data.checksum ? clonePlainValue(node.data.checksum) : node.data.checksum,
      archive: node.data.archive ? clonePlainValue(node.data.archive) : node.data.archive,
      wait: node.data.wait ? clonePlainValue(node.data.wait) : node.data.wait,
      notify: node.data.notify ? clonePlainValue(node.data.notify) : node.data.notify
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

// 超出上限时保留最近的快照，撤销才能按时间倒序一步步退回。
function prunePast(state) {
  if (state.past.length > FLOW_HISTORY_LIMIT) state.past = state.past.slice(-FLOW_HISTORY_LIMIT)
}

// 只比较会写回工作流的内容：拖拽位置、选中环、尺寸标记等纯视图字段不算编辑。
function flowHistoryContent(snapshot) {
  const nodes = (snapshot?.nodes || []).map(node => ({
    id: node.id,
    type: node.type,
    data: JSON.stringify(node.data ?? null)
  }))
  const edges = (snapshot?.edges || []).map(edge => ({
    id: edge.id,
    source: edge.source,
    target: edge.target,
    targetHandle: edge.targetHandle || '',
    sourceHandle: edge.sourceHandle || '',
    data: JSON.stringify(edge.data ?? null)
  }))
  return JSON.stringify({nodes, edges, stages: snapshot?.stages || []})
}

function isEmptyHistoryStep(before, after) {
  return flowHistoryContent(before) === flowHistoryContent(after)
}

function useFlowHistory(nodes, edges, setNodes, setEdges, stages = [], setStages = () => {}) {
  const [availability, setAvailability] = useState({canUndo: false, canRedo: false})
  const stateRef = useRef({past: [], future: [], last: null, pending: null, timer: null, skip: false, skipTimer: null})
  const nodesRef = useRef(nodes)
  const edgesRef = useRef(edges)
  const stagesRef = useRef(stages)
  nodesRef.current = nodes
  edgesRef.current = edges
  stagesRef.current = stages

  const syncAvailability = useCallback(() => {
    const state = stateRef.current
    const canUndo = state.past.length > 0
    const canRedo = state.future.length > 0
    setAvailability(current => current.canUndo === canUndo && current.canRedo === canRedo ? current : {canUndo, canRedo})
  }, [])

  const commitPending = useCallback(() => {
    const state = stateRef.current
    if (!state.pending) return
    // React Flow 回传的尺寸/位置补丁会让画布反复变化但不改任何内容；
    // 只有内容真的变了才落成一步，撤销按钮才不会在刚加载完就说“还能撤销”。
    const current = {nodes: nodesRef.current, edges: edgesRef.current, stages: stagesRef.current}
    if (!isEmptyHistoryStep(state.pending, current)) {
      state.past.push(state.pending)
      prunePast(state)
    }
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
    state.last = {nodes: nodesRef.current, edges: edgesRef.current, stages: stagesRef.current}
    syncAvailability()
  }, [commitPending, syncAvailability])

  useEffect(() => {
    const state = stateRef.current
    const current = {nodes, edges, stages}
    if (!state.last) {
      state.last = current
      return
    }
    if (state.skip) {
      state.skip = false
      state.last = current
      return
    }
    if (state.last.nodes === nodes && state.last.edges === edges && state.last.stages === stages) return
    if (!state.pending) state.pending = state.last
    if (state.timer) window.clearTimeout(state.timer)
    state.timer = window.setTimeout(() => {
      const latest = stateRef.current
      commitPending()
      latest.last = {nodes: nodesRef.current, edges: edgesRef.current, stages: stagesRef.current}
      latest.timer = null
      syncAvailability()
    }, FLOW_HISTORY_SETTLE_MS)
  }, [nodes, edges, stages, commitPending, syncAvailability])

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
    setStages(snapshot.stages || [])
  }, [setNodes, setEdges, setStages])

  const undo = useCallback(() => {
    flushPending()
    const state = stateRef.current
    if (state.past.length === 0) return false
    state.future.push({nodes: nodesRef.current, edges: edgesRef.current, stages: stagesRef.current})
    if (state.future.length > FLOW_HISTORY_LIMIT) state.future.shift()
    restore(state.past.pop())
    syncAvailability()
    return true
  }, [flushPending, restore, syncAvailability])

  const redo = useCallback(() => {
    flushPending()
    const state = stateRef.current
    if (state.future.length === 0) return false
    const current = {nodes: nodesRef.current, edges: edgesRef.current, stages: stagesRef.current}
    // 与当前画布内容相同的快照没有可重做的内容，跳过它们，连续重做才能一步步前进。
    let snapshot = state.future.pop()
    while (snapshot && isEmptyHistoryStep(current, snapshot)) {
      snapshot = state.future.length > 0 ? state.future.pop() : null
    }
    if (!snapshot) {
      syncAvailability()
      return false
    }
    state.past.push(current)
    prunePast(state)
    restore(snapshot)
    syncAvailability()
    return true
  }, [flushPending, restore, syncAvailability])

  const resetHistory = useCallback((nextNodes, nextEdges, nextStages) => {
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
      edges: nextEdges === undefined ? edgesRef.current : nextEdges,
      stages: nextStages === undefined ? stagesRef.current : nextStages
    }
    syncAvailability()
  }, [syncAvailability])

  return {canUndo: availability.canUndo, canRedo: availability.canRedo, undo, redo, resetHistory}
}

function workflowDraftSignature(workflow, nodes, edges, fallbackCategory, workflowParamsText, ownerID = '', stages = workflow.stages) {
  let parameters = []
  try {
    const parsed = JSON.parse(workflowParamsText || '[]')
    if (Array.isArray(parsed)) parameters = parsed
  } catch {
    return JSON.stringify({ownerID, workflow, stages, nodes: (nodes || []).map(node => ({id: node.id, type: node.type, data: node.data})), edges, workflowParamsText})
  }
  return JSON.stringify({ownerID, draft: workflowModel.buildWorkflowDraft(workflow, nodes, edges, fallbackCategory, parameters, stages)})
}
