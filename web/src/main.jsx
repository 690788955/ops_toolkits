import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { createRoot } from 'react-dom/client'
import '@xyflow/react/dist/style.css'
import './styles.css'
import * as yaml from 'js-yaml'
import {approveRunNode, bindWorkflowConfigPath, bootstrapAuthTokenFromURL, deleteConfigVersion, deleteJSON, downloadUserWorkflowExport, fetchConfigVersion, fetchConfigVersions, fetchJSON, fetchMarket, fetchMarketSources, fetchNexusShareTargets, fetchRunDetail, fetchRunSummary, postJSON, postMarketInstall, postPluginZip, putJSON, rejectRunNode, rescanPlugins, saveConfigVersion, setDefaultConfigVersion, uploadWorkflowConfigFile} from './api.js'
import {buildTextDiff, countTextMatches, replaceTextDraft} from './configDiff.js'
import {buildConfigItemCollections} from './configItems.js'
import {filterEntries, readableAPIError, summarizeAPIResponse, tagsForEntries} from './utils.js'
import FileUploadInput from './FileUploadInput.jsx'
import ResizeHandle, {useResizableWidth} from './ResizeHandle.jsx'
import {normalizeLogFontSize} from './logUtils.js'
import SshHostsModal, {sshHostStatusLabel, summarizeSshHosts} from './SshHostsModal.jsx'
import PluginShareModal from './PluginShareModal.jsx'
import {MarketSourcesModal} from './MarketSourcesPanel.jsx'
import {useModalEscape} from './useModalEscape.js'
import {isDraftWindowLocation} from './workflow/draftWindow.js'
// 审批等待判定放在独立模块里：这里不能直接 import 编辑器，否则懒加载的画布会被打进首屏。
import {formatApprovalWait, isRunAwaiting, runAwaitingApproval} from './runApproval.js'

const WorkflowEditor = React.lazy(() => import('./workflow/WorkflowEditor.jsx'))
const LiveRunTerminal = React.lazy(() => import('./LiveRunTerminal.jsx'))
const RunRecordCanvas = React.lazy(() => import('./workflow/RunRecordCanvas.jsx'))
const RunLogView = React.lazy(() => import('./workflow/run/RunLogView.jsx'))
const DraftWindowShell = React.lazy(() => import('./workflow/DraftWindowShell.jsx'))

const APP_TABS = new Set(['tools', 'workflows', 'config', 'runs'])

function activeTabFromHash() {
  const tab = String(window.location.hash || '').replace(/^#/, '').split('?')[0]
  return APP_TABS.has(tab) ? tab : 'tools'
}

// 工作流深链的查询参数只认 run：其余 hash 片段（例如 #runs）不带语义，直接忽略。
export function runIDFromHash(hash) {
  const value = String(hash || '').replace(/^#/, '')
  const queryIndex = value.indexOf('?')
  if (queryIndex < 0) return ''
  const params = new URLSearchParams(value.slice(queryIndex + 1))
  return params.get('run') || ''
}

// 待审批横幅只在「当前页面处理不了这条审批」时提示：
// 运行记录页自己就有审批卡片；工作流页正在看的就是那条待审批运行时，画布上方也有审批条。
export function shouldShowAwaitingBanner({activeTab = '', total = 0, items = [], linkedRunID = ''}) {
  if (Number(total) <= 0) return false
  if (activeTab === 'runs') return false
  if (activeTab === 'workflows' && linkedRunID) {
    return !(items || []).some(item => item?.id === linkedRunID)
  }
  return true
}

// 「待审批」横幅：任何 tab 下都要能一眼看到，所以抽成纯组件放在 App 外面，方便单测。
// 文案只展示第一条的摘要：用户此刻真正要的是一个入口，不是把审批列表铺满屏幕。
export function AwaitingApprovalBanner({items = [], total = 0, onGo}) {
  const count = Number(total) || 0
  if (count <= 0) return null
  const first = Array.isArray(items) ? items[0] : null
  const approval = runAwaitingApproval(first)
  const message = truncateBannerText(approval?.message || '', 60)
  const title = first ? `${first.target || first.id}${approval?.message ? ` · ${approval.message}` : ''}` : ''
  return (
    <section className="runAwaitingBanner" role="status" aria-live="polite" aria-label="待审批运行">
      <div className="runAwaitingBannerText">
        <strong>{count} 个运行等待人工确认</strong>
        {first && (
          <span className="runAwaitingBannerItem" title={title}>
            {runKindText(first.kind)} · {first.target || first.id}
            {message ? ` · ${message}` : ''}
          </span>
        )}
        {count > 1 && <span className="runAwaitingBannerMore">还有 {count - 1} 个等待确认的运行，点「去确认」逐条处理。</span>}
      </div>
      <button type="button" className="primary runAwaitingBannerButton" onClick={() => onGo?.(first)}>去确认</button>
    </section>
  )
}

// 消息可能很长，直接铺开会撑破横幅；完整内容留在 title 上。
function truncateBannerText(value, limit) {
  const text = String(value || '').replace(/\s+/g, ' ').trim()
  return text.length <= limit ? text : `${text.slice(0, limit)}…`
}

// 运行类型统一叫法，横幅和运行记录列表共用。
function runKindText(kind) {
  if (kind === 'workflow') return '工作流'
  if (kind === 'tool') return '工具'
  return '运行'
}

function App() {
  const [catalog, setCatalog] = useState(null)
  const [activeCategory, setActiveCategory] = useState('')
  const [activeTab, setActiveTab] = useState(activeTabFromHash)
  const [workflowEditorMounted, setWorkflowEditorMounted] = useState(false)
  const [selected, setSelected] = useState(null)
  const [params, setParams] = useState({})
  const [searchText, setSearchText] = useState('')
  const [activeTag, setActiveTag] = useState('')
  const [result, setResult] = useState({message: '等待执行...'})
  const [pluginModalOpen, setPluginModalOpen] = useState(false)
  const [pluginUploadState, setPluginUploadState] = useState({message: ''})
  const [configSelectedPlugin, setConfigSelectedPlugin] = useState(null)
  const [configSelectedWorkflow, setConfigSelectedWorkflow] = useState(null)
  const [platformSettingsOpen, setPlatformSettingsOpen] = useState(false)
  const [globalEnvOpen, setGlobalEnvOpen] = useState(false)
  const [sshModalOpen, setSshModalOpen] = useState(false)
  const [sshData, setSshData] = useState(null)
  const [sshError, setSshError] = useState('')
  const [runHosts, setRunHosts] = useState([])
  const [runLocal, setRunLocal] = useState(true)
  const [resultExpanded, setResultExpanded] = useState(false)
  const [cancellingToolRun, setCancellingToolRun] = useState(false)
  // 待审批运行提到 App 级：用户可能停在工作流管理/全局工具/配置页，只有 App 能覆盖全部 tab。
  const [awaitingRuns, setAwaitingRuns] = useState({total: 0, items: []})
  const [focusRun, setFocusRun] = useState(null)
  // 深链 #workflows?run=<runID>：工作流工作台可以直接打开某条运行记录，便于分享和刷新后回到同一条。
  const [linkedRunID, setLinkedRunID] = useState(() => runIDFromHash(window.location.hash))
  const awaitingTotal = Number(awaitingRuns.total) || 0
  const awaitingRequestRef = useRef(0)
  const focusRunTokenRef = useRef(0)
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

  async function refreshSshStatus() {
    try {
      const body = await fetchJSON('/api/ssh/hosts')
      setSshData(body.data || null)
      setSshError('')
      return body.data
    } catch (err) {
      setSshError(readableAPIError(err, '加载操作机状态失败。'))
      throw err
    }
  }

  useEffect(() => {
    let ignore = false
    async function loadStatus() {
      try {
        const body = await fetchJSON('/api/ssh/hosts')
        if (ignore) return
        setSshData(body.data || null)
        setSshError('')
      } catch (err) {
        if (!ignore) setSshError(readableAPIError(err, '加载操作机状态失败。'))
      }
    }
    loadStatus()
    const timer = window.setInterval(loadStatus, 5000)
    return () => {
      ignore = true
      window.clearInterval(timer)
    }
  }, [])

  // 待审批计数：limit=5 只是为了横幅能展示第一条摘要，总数以接口的 total 为准。
  // 失败时保留上一次结果——横幅偶发空白比“计数闪成 0 又跳回来”更糟。
  async function refreshAwaitingRuns() {
    if (typeof document !== 'undefined' && document.hidden) return
    const requestID = ++awaitingRequestRef.current
    try {
      const body = await fetchJSON('/api/runs/?awaiting=1&limit=5')
      if (requestID !== awaitingRequestRef.current) return
      const items = Array.isArray(body.data?.runs) ? body.data.runs : []
      const total = Number(body.data?.total ?? items.length)
      setAwaitingRuns({total: Number.isFinite(total) ? total : items.length, items})
    } catch {
      // 静默失败：横幅只是提示，下一个轮询周期会重试。
    }
  }

  // 10 秒一轮：比运行记录页的 2 秒轮询慢得多，两套轮询不会叠加成请求风暴；
  // 切页、窗口重新聚焦、标签页重新可见时立即补一次，保证“切回来就能看到”。
  useEffect(() => {
    refreshAwaitingRuns()
    function refreshWhenVisible() {
      if (document.hidden) return
      refreshAwaitingRuns()
    }
    window.addEventListener('focus', refreshWhenVisible)
    document.addEventListener('visibilitychange', refreshWhenVisible)
    const timer = window.setInterval(() => {
      // 运行记录页自己每 2 秒刷列表并把计数回调上来，这里跳过这一轮避免重复请求。
      if (activeTab === 'runs') return
      refreshAwaitingRuns()
    }, 10000)
    return () => {
      window.clearInterval(timer)
      window.removeEventListener('focus', refreshWhenVisible)
      document.removeEventListener('visibilitychange', refreshWhenVisible)
    }
  }, [activeTab])

  // 标题前缀是给「页面不在前台」时用的：标签页栏上就能看到还有几条没确认。
  useEffect(() => {
    if (awaitingTotal <= 0) return undefined
    const baseTitle = document.title
    document.title = `(${awaitingTotal}) 待审批 · ${baseTitle}`
    return () => {
      document.title = baseTitle
    }
  }, [awaitingTotal])

  // token 递增是为了让“同一条运行再次点去确认”也能触发运行记录页重新定位。
  function goToAwaitingRun(run) {
    const runID = typeof run === 'string' ? run : run?.id || ''
    focusRunTokenRef.current += 1
    setFocusRun({runID, token: focusRunTokenRef.current})
    setActiveTab('runs')
  }

  // 用户手动切页 = “重新开始”：清掉横幅带过来的一次性定位。
  // 否则运行记录页卸载再挂载时，会照着旧 token 把待审批筛选和选中项又抢回去。
  function changeTab(tab) {
    setFocusRun(null)
    if (tab !== 'workflows') setLinkedRunID('')
    setActiveTab(tab)
    setSelected(null)
    setActiveTag('')
    resetResult()
  }

  useEffect(() => {
    if (activeTab === 'workflows') setWorkflowEditorMounted(true)
  }, [activeTab])

  useEffect(() => {
    const target = activeTab === 'workflows' && linkedRunID
      ? `#workflows?run=${encodeURIComponent(linkedRunID)}`
      : `#${activeTab}`
    if (window.location.hash === target) return
    window.history.replaceState(null, '', `${window.location.pathname}${window.location.search}${target}`)
  }, [activeTab, linkedRunID])

  useEffect(() => {
    function syncTabFromHash() {
      setActiveTab(activeTabFromHash())
      setLinkedRunID(runIDFromHash(window.location.hash))
    }
    window.addEventListener('hashchange', syncTabFromHash)
    return () => window.removeEventListener('hashchange', syncTabFromHash)
  }, [])

  const category = useMemo(() => {
    return catalog?.categories?.find(item => item.id === activeCategory)
  }, [catalog, activeCategory])

  const categoryDisabled = Boolean(category?.disabled)

  const hasConfigTab = useMemo(() => {
    const tools = catalog?.tools || []
    const workflows = catalog?.workflows || []
    return tools.some(tool => (tool.config_file_entries && tool.config_file_entries.length > 0) || (tool.config_files && tool.config_files.length > 0)) ||
      workflows.some(workflow => (workflow.config_files || []).length > 0)
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

  const workflowCount = useMemo(() => (catalog?.workflows || []).length, [catalog])
  const visibleWorkflowCount = useMemo(() => {
    if (!activeCategory) return workflowCount
    return (catalog?.workflows || []).filter(item => item.category === activeCategory).length
  }, [catalog, activeCategory, workflowCount])

  const configCount = useMemo(() => {
    const {toolConfigItems, workflowConfigItems} = buildConfigItemCollections(catalog, activeCategory)
    return toolConfigItems.length + workflowConfigItems.length
  }, [catalog, activeCategory])

  function resetResult() {
    runVersionRef.current += 1
    setResultExpanded(false)
    setResult({message: '等待执行...'})
  }

  function selectEntry(entry) {
    setSelected({...entry, kind: 'tool'})
    setRunHosts([])
    setRunLocal(true)
    const next = {}
    ;(entry.parameters || []).forEach(param => {
      next[param.name] = param.default === undefined || param.default === null ? '' : String(param.default)
    })
    setParams(next)
    resetResult()
  }

  async function runSelected() {
    if (!selected) return
    if (!runLocal && runHosts.length === 0) {
      setResult({message: '请至少选择一个执行目标：本机或操作机。'})
      return
    }
    const missingRequired = (selected.parameters || [])
      .filter(param => param.required && !hasConfiguredParamValue(params?.[param.name]))
      .map(param => param.description || param.name)
    if (missingRequired.length > 0) {
      setResult({message: `请先填写必填参数：${missingRequired.join('、')}`})
      return
    }
    const needsConfirm = selected.confirm?.required
    if (needsConfirm && !window.confirm(selected.confirm.message || '该操作需要确认，是否继续？')) return
    runVersionRef.current += 1
    const runVersion = runVersionRef.current
    setResult({message: '执行中...'})
    try {
      const body = await postJSON(`/api/tools/${selected.id}/run?async=true`, {params, confirm: Boolean(needsConfirm), include_local: runLocal, targets: runHosts.length > 0 ? runHosts : undefined})
      if (runVersionRef.current !== runVersion) return
      if (body.data?.runs?.length) {
        const batch = {batchID: body.data.batch_id, runs: body.data.runs}
        setResult({batch, message: '执行中...'})
        pollBatchRunDetails(batch, runVersion)
        return
      }
      if (body.id) {
        const running = {...body, status: body.status || 'running'}
        setResult({run: running, message: '执行中...'})
        pollToolRunDetail(running, runVersion)
        return
      }
      setResult({message: summarizeAPIResponse(body, '执行请求已提交。'), response: body})
    } catch (err) {
      if (runVersionRef.current !== runVersion) return
      setResult({message: readableAPIError(err, '执行失败。'), response: err.body})
    }
  }

  async function pollToolRunDetail(run, runVersion) {
    while (runVersionRef.current === runVersion) {
      try {
        const detail = await fetchRunDetail(run.id)
        if (runVersionRef.current !== runVersion) return
        const detailData = detail?.data || detail
        const status = detailData?.record?.status || run.status || 'running'
        setResult({run: {...run, status}, detail: {data: detailData}})
        if (status !== 'running') return
      } catch (err) {
        if (runVersionRef.current !== runVersion) return
        setResult({message: readableAPIError(err, '读取运行详情失败。'), run})
        return
      }
      await new Promise(resolve => window.setTimeout(resolve, 1000))
    }
  }

  async function pollBatchRunDetails(batch, runVersion) {
    while (runVersionRef.current === runVersion) {
      try {
        const summaries = await Promise.all(batch.runs.map(run => fetchRunSummary(run.id)))
        if (runVersionRef.current !== runVersion) return
        const runs = batch.runs.map((run, index) => {
          const summary = summaries[index]?.data?.record
          return summary ? {...run, ...summary} : run
        })
        const nextBatch = {...batch, runs}
        const active = runs.some(run => run.status === 'running')
        setResult({batch: nextBatch, message: active ? '执行中...' : batchResultMessage(runs)})
        if (!active) return
      } catch (err) {
        if (runVersionRef.current !== runVersion) return
        setResult({message: readableAPIError(err, '读取批量运行状态失败。'), batch})
        return
      }
      await new Promise(resolve => window.setTimeout(resolve, 800))
    }
  }

  async function cancelCurrentToolRun() {
    const batchRuns = result?.batch?.runs || []
    const runningRuns = batchRuns.filter(run => run.status === 'running')
    const runIDs = runningRuns.length > 0
      ? runningRuns.map(run => run.id)
      : result?.run?.status === 'running' && result.run.id
        ? [result.run.id]
        : []
    if (runIDs.length === 0) return
    if (!window.confirm(`确认取消 ${runIDs.length} 个运行任务？`)) return
    setCancellingToolRun(true)
    setResult(current => ({...current, message: `正在取消 ${runIDs.length} 个运行任务...`}))
    try {
      await Promise.all(runIDs.map(runID => postJSON(`/api/runs/${encodeURIComponent(runID)}/cancel`, {})))
      setResult(current => ({...current, message: `已发送取消请求（${runIDs.length} 个任务），状态会自动刷新。`}))
    } catch (err) {
      setResult(current => ({...current, message: readableAPIError(err, '取消运行任务失败。')}))
    } finally {
      setCancellingToolRun(false)
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

  const sshSummary = summarizeSshHosts(sshData)
  const sshButtonTitle = sshError
    ? `操作机状态加载失败：${sshError}`
    : sshSummary.total > 0
      ? `操作机在线 ${sshSummary.online}/${sshSummary.total}，点击配置`
      : '配置 SSH 操作机'

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
            <span>全局工具</span>
            <small>跨分类选择所有可见工具</small>
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
        <button className="pluginAction" onClick={() => setPluginModalOpen(true)} title="插件管理">
          <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M11 5h2v6h6v2h-6v6h-2v-6H5v-2h6V5Z" /></svg>
          <span>插件管理</span>
        </button>
        <div className="sidebarFooter" role="group" aria-label="平台快捷操作">
          <button className="sidebarAction sshStatusButton" type="button" title={sshButtonTitle} aria-label="操作机" onClick={() => setSshModalOpen(true)}>
            <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M4 4h16a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2h-6v2h3v2H7v-2h3v-2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Zm0 2v9h16V6H4Z" /></svg>
            <span>操作机</span>
            <span className="sshStatus">
              <span className={`sshDot ${sshError ? 'offline' : sshSummary.state}`} aria-hidden="true" />
              <span className="sshStatusCount">{sshSummary.total > 0 ? `${sshSummary.online}/${sshSummary.total}` : '未配置'}</span>
            </span>
          </button>
          <button className="sidebarAction" type="button" title="工具和插件脚本可引用的全局变量" aria-label="全局变量" onClick={() => setGlobalEnvOpen(true)}>
            <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M9.4 16.6 4.8 12l4.6-4.6L8 6l-6 6 6 6 1.4-1.4Zm5.2 0 4.6-4.6-4.6-4.6L16 6l6 6-6 6-1.4-1.4Z" /></svg>
            <span>全局变量</span>
          </button>
          <button className="sidebarAction" type="button" title="平台、界面和服务设置" aria-label="平台设置" onClick={() => setPlatformSettingsOpen(true)}>
            <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M19.4 13a7.6 7.6 0 0 0 .1-1 7.6 7.6 0 0 0-.1-1l2.1-1.6a.5.5 0 0 0 .1-.7l-2-3.4a.5.5 0 0 0-.6-.2l-2.5 1a7.6 7.6 0 0 0-1.7-1l-.4-2.6a.5.5 0 0 0-.5-.4h-4a.5.5 0 0 0-.5.4l-.4 2.6a7.6 7.6 0 0 0-1.7 1l-2.5-1a.5.5 0 0 0-.6.2l-2 3.4a.5.5 0 0 0 .1.7L4.5 11a7.6 7.6 0 0 0-.1 1 7.6 7.6 0 0 0 .1 1l-2.1 1.6a.5.5 0 0 0-.1.7l2 3.4a.5.5 0 0 0 .6.2l2.5-1a7.6 7.6 0 0 0 1.7 1l.4 2.6a.5.5 0 0 0 .5.4h4a.5.5 0 0 0 .5-.4l.4-2.6a7.6 7.6 0 0 0 1.7-1l2.5 1a.5.5 0 0 0 .6-.2l2-3.4a.5.5 0 0 0-.1-.7L19.4 13ZM12 15.5A3.5 3.5 0 1 1 12 8.5a3.5 3.5 0 0 1 0 7Z" /></svg>
            <span>平台设置</span>
          </button>
        </div>
      </aside>

      <main className="content">
        <header className="topbar">
          <div>
            <h2>{activeTab === 'workflows' ? '工作流管理' : category?.name || '全局工具'}</h2>
            <p>{activeTab === 'workflows'
              ? activeCategory
                ? `当前分类 ${category?.name || activeCategory}，${visibleWorkflowCount} 个工作流可加载；画布仍可跨分类组合工具`
                : `统一管理全部 ${workflowCount} 个工作流，可跨分类组合工具`
              : category?.description || '跨分类选择所有可见工具'}</p>
          </div>
        </header>

        {shouldShowAwaitingBanner({activeTab, total: awaitingTotal, items: awaitingRuns.items, linkedRunID}) && (
          <AwaitingApprovalBanner items={awaitingRuns.items} total={awaitingTotal} onGo={goToAwaitingRun} />
        )}

        <div className="tabs">
          <button className={activeTab === 'tools' ? 'tab active' : 'tab'} onClick={() => changeTab('tools')}>工具 <span className="tabCount">{toolEntries.length}</span></button>
          <button className={activeTab === 'workflows' ? 'tab active' : 'tab'} onClick={() => changeTab('workflows')}>工作流管理 <span className="tabCount">{visibleWorkflowCount}</span></button>
          {hasConfigTab && <button className={activeTab === 'config' ? 'tab active' : 'tab'} onClick={() => { setConfigSelectedPlugin(null); setConfigSelectedWorkflow(null); changeTab('config') }}>配置 <span className="tabCount">{configCount}</span></button>}
          <button className={activeTab === 'runs' ? 'tab active' : 'tab'} onClick={() => changeTab('runs')}>运行记录{awaitingTotal > 0 && <span className="tabCount tabCountAwaiting">{awaitingTotal}</span>}</button>
        </div>

        {activeTab === 'tools' && !selected && (
          <GettingStartedCard toolCount={toolEntries.length} />
        )}

        {workflowEditorMounted && (
          <div className={activeTab === 'workflows' ? 'tabPanel active' : 'tabPanel inactive'} aria-hidden={activeTab !== 'workflows'}>
            <React.Suspense fallback={<section className="card canvasCard"><div className="empty">正在加载工作流画布...</div></section>}>
              <WorkflowEditor
                catalog={catalog}
                activeCategory={activeCategory}
                hosts={sshData?.hosts || []}
                setResult={setResult}
                refreshCatalog={refreshCatalog}
                openPluginManager={() => setPluginModalOpen(true)}
                onOpenRunsTab={() => changeTab('runs')}
                openRunID={linkedRunID}
                onOpenRun={runID => setLinkedRunID(runID || '')}
                resultPanel={(
                  <section className="card resultCard workflowResultCard">
                    {resultPanelHeader}
                    <ResultView result={result} />
                  </section>
                )}
              />
            </React.Suspense>
          </div>
        )}

        {activeTab === 'runs' ? (
          <RunHistoryPanel catalog={catalog} focusRun={focusRun} onAwaitingChanged={refreshAwaitingRuns} />
        ) : (activeTab === 'config' && hasConfigTab) ? (
          <ConfigPanel
            catalog={catalog}
            activeCategory={activeCategory}
            configSelectedPlugin={configSelectedPlugin}
            setConfigSelectedPlugin={setConfigSelectedPlugin}
            configSelectedWorkflow={configSelectedWorkflow}
            setConfigSelectedWorkflow={setConfigSelectedWorkflow}
            refreshCatalog={refreshCatalog}
          />
        ) : activeTab !== 'workflows' ? (
          <RunPanel entries={entries} totalEntries={toolEntries.length} selected={selected} params={params} setParams={setParams} selectEntry={selectEntry} runSelected={runSelected} searchText={searchText} setSearchText={setSearchText} activeTag={activeTag} setActiveTag={setActiveTag} availableTags={availableTags} hosts={sshData?.hosts || []} runHosts={runHosts} setRunHosts={setRunHosts} runLocal={runLocal} setRunLocal={setRunLocal} isRunning={result?.run?.status === 'running' || (result?.batch?.runs || []).some(run => run.status === 'running')} onCancelRun={cancelCurrentToolRun} isCancelling={cancellingToolRun} />
        ) : null}

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
          onRefresh={async () => {
            await refreshCatalog({keepCategory: true})
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
      {sshModalOpen && (
        <SshHostsModal
          data={sshData}
          error={sshError}
          onClose={() => setSshModalOpen(false)}
          onRefresh={refreshSshStatus}
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

function GettingStartedCard({toolCount}) {
  return (
    <section className="gettingStarted" aria-labelledby="getting-started-title">
      <div className="gettingStartedCopy">
        <h3 id="getting-started-title">从一个安全的工具执行开始</h3>
        <p>先选择工具并填写参数，执行结果和实时日志会显示在下方。熟悉单个工具后，再用工作流把多个步骤串起来。</p>
      </div>
      <div className="gettingStartedSteps">
        <div className="gettingStartedStep"><strong>1</strong><span><b>选择工具</b><small>{toolCount} 个可用工具</small></span></div>
        <div className="gettingStartedStep"><strong>2</strong><span><b>填写参数</b><small>必填项会在执行前校验</small></span></div>
        <div className="gettingStartedStep"><strong>3</strong><span><b>查看结果</b><small>支持日志、状态和支持包</small></span></div>
      </div>
    </section>
  )
}

export function RunHistoryPanel({catalog, focusRun = null, onAwaitingChanged}) {
  const [runs, setRuns] = useState([])
  const [totalRuns, setTotalRuns] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [searchInput, setSearchInput] = useState('')
  const [searchText, setSearchText] = useState('')
  const [selectedRunID, setSelectedRunID] = useState('')
  const [selectedDetail, setSelectedDetail] = useState(null)
  const [statusFilter, setStatusFilter] = useState('')
  const [detailView, setDetailView] = useState('logs')
  const [selectedWorkflow, setSelectedWorkflow] = useState(null)
  const [workflowMessage, setWorkflowMessage] = useState('')
  const [message, setMessage] = useState('正在加载运行记录...')
  const [loading, setLoading] = useState(false)
  const [cancellingRunID, setCancellingRunID] = useState('')
  const [cleanupOpen, setCleanupOpen] = useState(false)
  const [cleanupKeep, setCleanupKeep] = useState('50')
  const [cleanupDryRun, setCleanupDryRun] = useState(true)
  const [cleanupBusy, setCleanupBusy] = useState(false)
  const [cleanupResult, setCleanupResult] = useState(null)
  // 待审批要单独一档筛选：这类运行的状态仍是 running，用状态筛子根本筛不出来。
  const [awaitingOnly, setAwaitingOnly] = useState(false)
  const [awaitingCount, setAwaitingCount] = useState(0)
  const [approvalNote, setApprovalNote] = useState('')
  const [approvalAction, setApprovalAction] = useState('')
  const [approvalClock, setApprovalClock] = useState(() => Date.now())
  const listRequestRef = useRef(0)
  const detailRequestRef = useRef(0)
  const focusTokenRef = useRef(0)
  // 运行记录列表和右侧日志详情谁宽谁窄，交给用户拖：排查时经常一边要列表、一边要长日志。
  const layoutRef = useRef(null)
  const listWidth = useResizableWidth({storageKey: 'ops.runRecords.listWidth', min: 300, max: 680})
  const totalPages = Math.max(1, Math.ceil(totalRuns / pageSize))

  // 徽标上的数字必须反映全量而不是当前页，所以单独要一次 limit=0 的 total。
  // 计数只是辅助信息，读取失败时保留上一次的值，不影响列表本身。
  async function loadAwaitingCount() {
    try {
      const body = await fetchJSON('/api/runs/?awaiting=1&limit=0')
      const total = Number(body.data?.total ?? (body.data?.runs || []).length)
      setAwaitingCount(Number.isFinite(total) ? total : 0)
    } catch {
      // 静默失败：下一次轮询或手动刷新会重试。
    }
  }

  async function loadRuns({silent = false} = {}) {
    const requestID = ++listRequestRef.current
    if (!silent) {
      setLoading(true)
      setMessage('正在加载运行记录...')
    }
    try {
      const query = new URLSearchParams()
      query.set('limit', String(pageSize))
      query.set('offset', String((page - 1) * pageSize))
      if (statusFilter) query.set('status', statusFilter)
      if (awaitingOnly) query.set('awaiting', '1')
      if (searchText) query.set('q', searchText)
      const body = await fetchJSON(`/api/runs/?${query.toString()}`)
      if (requestID !== listRequestRef.current) return
      const items = body.data?.runs || []
      const total = Number(body.data?.total ?? items.length)
      setRuns(items)
      setTotalRuns(Number.isFinite(total) ? total : items.length)
      if (!silent) {
        setMessage(total > 0 ? `已加载 ${items.length} 条，共 ${total} 条运行记录。` : '暂无匹配的运行记录。')
      }
      await loadAwaitingCount()
    } catch (err) {
      if (requestID !== listRequestRef.current) return
      if (!silent) {
        setMessage(readableAPIError(err, '加载运行记录失败。'))
      }
    } finally {
      if (requestID === listRequestRef.current && !silent) {
        setLoading(false)
      }
    }
  }

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setSearchText(searchInput.trim())
      setPage(1)
    }, 300)
    return () => window.clearTimeout(timer)
  }, [searchInput])

  useEffect(() => {
    loadRuns()
  }, [page, pageSize, statusFilter, searchText, awaitingOnly])

  useEffect(() => {
    if (page > totalPages) setPage(totalPages)
  }, [page, totalPages])

  async function refreshSelectedRun(runID, {silent = false} = {}) {
    if (!runID) return
    const requestID = ++detailRequestRef.current
    if (!silent) {
      setSelectedRunID(runID)
      setSelectedDetail(null)
      setMessage(`正在读取运行记录 ${runID}...`)
    }
    try {
      const body = await fetchRunDetail(runID)
      if (requestID !== detailRequestRef.current) return
      setSelectedDetail(body.data)
      if (!silent) {
        setMessage(`已加载运行记录 ${runID}。`)
      }
    } catch (err) {
      if (requestID !== detailRequestRef.current) return
      if (!silent) {
        setMessage(readableAPIError(err, '读取运行详情失败。'))
      }
    }
  }

  async function selectRun(runID, nextDetailView = '') {
    setSelectedRunID(runID)
    if (nextDetailView) {
      setDetailView(nextDetailView)
    }
    await refreshSelectedRun(runID)
  }

  // 待审批判定统一走 runApproval.js：列表项只有 awaiting_approval/approval 摘要，
  // 详情才有完整 approvals 数组，两种形状在这里归一，JSX 里不散落 .some()。
  const pendingApproval = useMemo(() => runAwaitingApproval(selectedDetail?.record), [selectedDetail])

  // 详情接口只在运行中/待处理时返回 actionable；缺省按“可以处理”看待，
  // 由后端在真正确认时兜底报 409，避免老版本后端把按钮全部禁用。
  const approvalActionable = Boolean(pendingApproval) && selectedDetail?.actionable !== false

  // 节点名优先用工作流配置里的 name，读不到工作流（或节点已删）就退回节点 ID。
  const approvalNodeName = useMemo(() => {
    if (!pendingApproval) return ''
    const nodes = selectedWorkflow?.nodes || []
    const match = nodes.find(node => node.id === pendingApproval.node_id)
    return match?.name || pendingApproval.node_id
  }, [pendingApproval, selectedWorkflow])

  // 已等待时长要随轮询一起走：详情每 2 秒刷新一次，这里补一拍保证数字在涨。
  useEffect(() => {
    if (!pendingApproval) return undefined
    setApprovalClock(Date.now())
    const timer = window.setInterval(() => setApprovalClock(Date.now()), 10000)
    return () => window.clearInterval(timer)
  }, [pendingApproval])

  // 运行记录页自己每 2 秒轮询列表，App 的 10 秒 tick 在这一页会跳过；
// 计数一变就在这里回传，顶栏横幅无需等到切页才发现新的待审批。
  useEffect(() => {
    onAwaitingChanged?.()
  }, [awaitingCount])

  // 从横幅或别的页面点「去确认」过来：开关待审批筛选、回到第一页并选中那条运行，
  // 详情里就会直接出现审批卡片。token 变化才重新定位，避免普通重渲染把筛选抢回去。
  useEffect(() => {
    if (!focusRun || !focusRun.token) return
    if (focusTokenRef.current === focusRun.token) return
    focusTokenRef.current = focusRun.token
    setAwaitingOnly(true)
    setPage(1)
    if (focusRun.runID) selectRun(focusRun.runID)
  }, [focusRun])

  useEffect(() => {
    let ignore = false
    async function loadWorkflowForDetail() {
      const record = selectedDetail?.record
      const target = record?.kind === 'workflow' ? record.target : ''
      setSelectedWorkflow(null)
      setWorkflowMessage('')
      if (!target) return
      // 画布优先用运行记录里的快照：工作流改过之后，用当前配置渲染会显示成新拓扑 + 旧结论。
      const snapshot = record?.snapshot?.workflow
      if (snapshot?.nodes?.length > 0) {
        setSelectedWorkflow({...snapshot, id: snapshot.id || target, name: snapshot.name || target})
        setWorkflowMessage('')
        return
      }
      try {
        setWorkflowMessage('正在加载运行画布...')
        const body = await fetchJSON(`/api/workflows/${encodeURIComponent(target)}`)
        if (ignore) return
        const config = body.data?.Config || body.data?.config
        setSelectedWorkflow(config || null)
        setWorkflowMessage(config ? '这条运行记录没有保存工作流快照，画布按当前工作流展示。' : '未读取到工作流配置，无法展示画布。')
      } catch (err) {
        if (!ignore) setWorkflowMessage(readableAPIError(err, '加载运行画布失败。'))
      }
    }
    loadWorkflowForDetail()
    return () => { ignore = true }
  }, [selectedDetail?.record?.kind, selectedDetail?.record?.target, selectedDetail?.record?.snapshot])

  useEffect(() => {
    // 待审批也算“还在跑”：批准之后必须让列表和详情自己更新，否则用户会以为没生效。
    const hasRunningRun = runs.some(item => item.status === 'running' || isRunAwaiting(item)) ||
      selectedDetail?.record?.status === 'running' ||
      Boolean(pendingApproval) ||
      awaitingCount > 0
    if (!hasRunningRun) return undefined
    const timer = window.setInterval(() => {
      loadRuns({silent: true})
      if (selectedRunID) {
        refreshSelectedRun(selectedRunID, {silent: true})
      }
    }, 2000)
    return () => window.clearInterval(timer)
  }, [runs, selectedDetail?.record?.status, selectedRunID, page, pageSize, statusFilter, searchText, awaitingOnly, awaitingCount, pendingApproval])

  function changeStatusFilter(value) {
    setStatusFilter(value)
    setPage(1)
  }

  // 切筛选一律回到第一页：否则从第 3 页切到只剩 1 条结果的筛选会看到空白。
  function changeAwaitingOnly(value) {
    setAwaitingOnly(Boolean(value))
    setPage(1)
  }

  function changePageSize(value) {
    setPageSize(Number(value))
    setPage(1)
  }

  async function runCleanup({dryRun = cleanupDryRun, confirmed = false} = {}) {
    const keep = Number(cleanupKeep)
    if (!Number.isInteger(keep) || keep < 0) {
      setCleanupResult({kind: 'error', title: '保留数量无效', detail: '请输入 0 或正整数作为保留数量。'})
      return
    }
    if (!dryRun && !confirmed && !window.confirm(`确认删除除最新 ${keep} 条以外的运行记录？该操作不可撤销。`)) return
    setCleanupBusy(true)
    setCleanupResult(null)
    try {
      const body = await postJSON('/api/runs/cleanup', {keep, dry_run: dryRun})
      const result = body.data || {}
      setCleanupResult({
        kind: 'ok',
        title: dryRun ? '预演完成' : '清理完成',
        detail: `总计 ${Number(result.total || 0)} 条，保留 ${Number(result.keep ?? keep)} 条，${dryRun ? '预演将删除' : '已删除'} ${Number(result.deleted || 0)} 条。`,
        keep: Number(result.keep ?? keep),
        dryRun,
        candidates: Array.isArray(result.candidates) ? result.candidates : []
      })
      if (!dryRun) {
        setCleanupDryRun(true)
        setSelectedRunID('')
        setSelectedDetail(null)
        setMessage(`已清理 ${Number(result.deleted || 0)} 条运行记录。`)
        await loadRuns({silent: true})
      }
    } catch (err) {
      setCleanupResult({kind: 'error', title: '清理失败', detail: readableAPIError(err, '清理运行记录失败。')})
    } finally {
      setCleanupBusy(false)
    }
  }

  function changeCleanupKeep(value) {
    setCleanupKeep(value)
    setCleanupResult(null)
  }

  function changeCleanupDryRun(value) {
    setCleanupDryRun(value)
    setCleanupResult(null)
  }

  async function cancelRun(runID) {
    if (!runID) return
    setCancellingRunID(runID)
    setMessage(`正在取消运行任务 ${runID}...`)
    try {
      await postJSON(`/api/runs/${encodeURIComponent(runID)}/cancel`, {})
      setMessage(`已发送取消请求：${runID}`)
      await loadRuns()
      if (selectedRunID === runID) {
        await selectRun(runID)
      }
    } catch (err) {
      setMessage(readableAPIError(err, '取消运行任务失败。'))
    } finally {
      setCancellingRunID('')
    }
  }

  // 审批提交：备注可空（空备注不写进请求体，后端按“没有备注”处理）。
  // 驳回会让本次运行直接失败，所以坚持要一次二次确认。
  async function submitRunApproval(nodeID, action) {
    const runID = selectedRunID
    if (!runID || !nodeID || approvalAction) return
    const nodeLabel = approvalNodeName || nodeID
    if (action === 'reject' && !window.confirm(`确认驳回节点「${nodeLabel}」的审批？驳回后本次运行会直接失败。`)) return
    const note = approvalNote.trim()
    const payload = note ? {note} : {}
    setApprovalAction(action)
    setMessage(action === 'approve' ? `正在确认节点 ${nodeLabel}...` : `正在驳回节点 ${nodeLabel}...`)
    try {
      const body = action === 'approve'
        ? await approveRunNode(runID, nodeID, payload)
        : await rejectRunNode(runID, nodeID, payload)
      setApprovalNote('')
      setMessage(action === 'approve'
        ? `已确认继续：${nodeLabel}（${runID}），后续节点将继续执行。`
        : `已驳回：${nodeLabel}（${runID}），本次运行按失败结束。`)
      // 审批接口返回的就是运行详情，先落地一次让卡片立刻消失，再走一遍刷新与轮询对齐。
      if (body?.data?.record) setSelectedDetail(body.data)
      await refreshSelectedRun(runID, {silent: true})
      await loadRuns({silent: true})
      onAwaitingChanged?.()
    } catch (err) {
      setMessage(readableAPIError(err, action === 'approve' ? '确认审批失败。' : '驳回审批失败。'))
    } finally {
      setApprovalAction('')
    }
  }

  const statusOptions = [
    ['', '全部'],
    ['pending', '待处理'],
    ['failed', '失败'],
    ['rolled_back', '已回滚'],
    ['running', '运行中'],
    ['cancelled', '已取消'],
    ['succeeded', '成功']
  ]

  const runHistoryStyle = listWidth.width ? {'--run-history-list-width': `${listWidth.width}px`} : undefined
  const measureRunHistoryList = () => {
    const layout = layoutRef.current
    if (!layout) return 0
    const card = layout.querySelector('.runHistoryListCard')
    return card ? card.getBoundingClientRect().width : 0
  }

  return (
    <div className="runHistoryLayout" ref={layoutRef} style={runHistoryStyle}>
      <section className="card runHistoryListCard">
        <div className="cardHeader">
          <h3>运行记录{awaitingCount > 0 && <span className="runHistoryAwaitingCount">待审批 {awaitingCount}</span>}</h3>
          <div className="runHistoryHeaderActions">
            <button className="secondary" type="button" onClick={() => { setCleanupOpen(true); setCleanupResult(null); setCleanupDryRun(true) }}>清理</button>
            <button className="secondary" type="button" disabled={loading} onClick={() => loadRuns()}>{loading ? '刷新中...' : '刷新'}</button>
          </div>
        </div>
        <div className="runHistoryFilters">
          <input value={searchInput} placeholder="搜索运行 ID、目标、操作机或参数" onChange={event => setSearchInput(event.target.value)} />
          <div className="tagFilters runStatusFilters">
            {(awaitingCount > 0 || awaitingOnly) && (
              <button className={`tagChip runAwaitingChip${awaitingOnly ? ' active' : ''}`} onClick={() => changeAwaitingOnly(!awaitingOnly)}>待审批 {awaitingCount}</button>
            )}
            {statusOptions.map(([value, label]) => (
              <button key={value || 'all'} className={statusFilter === value ? 'tagChip active' : 'tagChip'} onClick={() => changeStatusFilter(value)}>{label}</button>
            ))}
          </div>
        </div>
        <div className="runHistoryList">
          {runs.map(item => {
            const awaiting = runAwaitingApproval(item)
            const waitLabel = awaiting ? formatApprovalWait(awaiting.since) : ''
            // 由 CLI 等其它进程启动的运行，Web 端提交审批一定失败，直接标注而不是给一个会报错的按钮。
            const actionable = !awaiting || item.approval_actionable !== false
            return (
              <div key={item.id} className={`runHistoryItem${selectedRunID === item.id ? ' active' : ''}${awaiting ? ' awaiting' : ''}`}>
                <button type="button" className="runHistorySelectButton" onClick={() => selectRun(item.id, awaiting ? 'logs' : '')}>
                  <div className="runHistoryItemMain">
                    <div className="runHistoryItemTitle">
                      <span className="runHistoryKind">{runKindText(item.kind)}</span>
                      <strong>{item.target || item.id}</strong>
                    </div>
                    <span className="runHistoryItemID">{item.id}</span>
                    {awaiting && (
                      <span className="runHistoryAwaitingMessage" title={awaiting.message || ''}>
                        {awaiting.message || '审批节点没有填写提示，请确认前置改动已经生效。'}
                      </span>
                    )}
                    {awaiting && waitLabel && <span className="runHistoryAwaitingMeta">已等待 {waitLabel}</span>}
                  </div>
                  <small className={`runStatusText ${item.status || 'unknown'}${awaiting ? ' awaiting' : ''}`}>{awaiting ? '运行中 · 待审批' : runStatusText(item.status)}</small>
                  <em>{formatRunTime(item.started_at)}</em>
                </button>
                <div className="runHistoryItemActions">
                  {awaiting && (actionable ? (
                    <button type="button" className="primary runHistoryAwaitingButton" onClick={() => selectRun(item.id, 'logs')}>去确认</button>
                  ) : (
                    <span className="runHistoryAwaitingLocked" title="该运行由其他进程（例如 CLI）启动，审批要在启动它的终端处理；Web 端无法提交">由其他进程启动</span>
                  ))}
                  {item.status === 'running' && !awaiting && (
                    <button type="button" className="secondary danger runCancelButton" disabled={cancellingRunID === item.id} onClick={() => cancelRun(item.id)}>
                      {cancellingRunID === item.id ? '取消中' : '取消'}
                    </button>
                  )}
                </div>
              </div>
            )
          })}
          {runs.length === 0 && <div className="empty">{awaitingOnly ? '现在没有等待确认的运行。有运行停在审批节点时会出现在这里。' : searchText || statusFilter ? '没有匹配的运行记录。' : '暂无运行记录。'}</div>}
        </div>
        {totalRuns > 0 && (
          <div className="paginationBar runHistoryPagination">
            <label>
              <span>每页</span>
              <select value={pageSize} onChange={event => changePageSize(event.target.value)}>
                <option value="10">10</option>
                <option value="20">20</option>
                <option value="50">50</option>
              </select>
            </label>
            <div className="paginationControls">
              <button className="secondary paginationButton" disabled={page <= 1} onClick={() => setPage(current => Math.max(1, current - 1))}>上一页</button>
              <span>{page} / {totalPages} · 共 {totalRuns} 条</span>
              <button className="secondary paginationButton" disabled={page >= totalPages} onClick={() => setPage(current => Math.min(totalPages, current + 1))}>下一页</button>
            </div>
          </div>
        )}
        {message && <div className="runHistoryMessage" role="status" aria-live="polite">{message}</div>}
        {cleanupOpen && (
          <div className="modalBackdrop" onClick={() => { if (!cleanupBusy) setCleanupOpen(false) }}>
            <section className="modal runCleanupModal" role="dialog" aria-modal="true" aria-label="清理运行记录" onClick={event => event.stopPropagation()}>
              <div className="modalHeader">
                <div>
                  <h3>清理运行记录</h3>
                  <p>保留最新记录，删除更早的运行目录。</p>
                </div>
                <button type="button" className="modalClose" onClick={() => setCleanupOpen(false)} disabled={cleanupBusy}>×</button>
              </div>
              <div className="form compact">
                <label>
                  <span>保留最新记录数</span>
                  <input type="number" min="0" value={cleanupKeep} disabled={cleanupBusy} onChange={event => changeCleanupKeep(event.target.value)} />
                </label>
                <label className="checkboxLabel">
                  <input type="checkbox" checked={cleanupDryRun} disabled={cleanupBusy} onChange={event => changeCleanupDryRun(event.target.checked)} />
                  <span>仅预演，不实际删除</span>
                </label>
                <div className="empty small">清理会删除除最新 N 条之外的所有运行目录；运行中的记录也可能被删除，建议先预演确认。</div>
                {cleanupResult && (
                  <div className={`runCleanupResult ${cleanupResult.kind}`}>
                    <strong>{cleanupResult.title}</strong>
                    <span>{cleanupResult.detail}</span>
                  </div>
                )}
                {cleanupResult?.kind === 'ok' && cleanupResult.dryRun && (
                  cleanupResult.candidates.length > 0 ? (
                    <div className="runCleanupPreview">
                      <div className="runCleanupPreviewHead">
                        <strong>将删除以下 {cleanupResult.candidates.length} 条记录</strong>
                        <span>保留最新 {cleanupResult.keep} 条</span>
                      </div>
                      <ul className="runCleanupCandidateList">
                        {cleanupResult.candidates.map(item => (
                          <li key={item.id}>
                            <div>
                              <strong>{item.target || item.id}</strong>
                              <small>{item.kind === 'workflow' ? '工作流' : item.kind === 'tool' ? '工具' : item.kind || '运行'} · {item.id}</small>
                            </div>
                            <div>
                              <span className={`runStatusText ${item.status || ''}`}>{runStatusText(item.status)}</span>
                              <time>{formatRunTime(item.started_at)}</time>
                            </div>
                          </li>
                        ))}
                      </ul>
                      <button className="secondary danger" type="button" disabled={cleanupBusy} onClick={() => runCleanup({dryRun: false, confirmed: true})}>
                        {cleanupBusy ? '处理中...' : '确认清理以上记录'}
                      </button>
                    </div>
                  ) : (
                    <div className="empty small">没有可清理的记录，无需删除。</div>
                  )
                )}
              </div>
              <div className="modalFooter">
                <button className="secondary" type="button" onClick={() => setCleanupOpen(false)} disabled={cleanupBusy}>关闭</button>
                <button className="primary" type="button" onClick={() => runCleanup()} disabled={cleanupBusy}>{cleanupBusy ? '处理中...' : cleanupDryRun ? '开始预演' : '确认清理'}</button>
              </div>
            </section>
          </div>
        )}
      </section>
      <ResizeHandle
        label="调整运行记录列表宽度"
        min={300}
        max={680}
        value={listWidth.width}
        fallback={400}
        measure={measureRunHistoryList}
        onChange={listWidth.setWidth}
        onReset={listWidth.reset}
      />
      <section className="card runHistoryDetailCard">
        <div className="cardHeader">
          <h3>记录详情</h3>
          <div className="runDetailActions">
            {selectedDetail?.record?.kind === 'workflow' && (
              <div className="segmentedControl" role="group" aria-label="运行详情视图">
                <button type="button" className={detailView === 'logs' ? 'active' : ''} onClick={() => setDetailView('logs')}>日志</button>
                <button type="button" className={detailView === 'canvas' ? 'active' : ''} onClick={() => setDetailView('canvas')}>画布</button>
              </div>
            )}
            {selectedDetail?.record?.status === 'running' && (
              <button type="button" className="secondary danger" disabled={cancellingRunID === selectedRunID} onClick={() => cancelRun(selectedRunID)}>
                {cancellingRunID === selectedRunID ? '取消中' : '取消运行'}
              </button>
            )}
            {selectedRunID && <a className="secondary" href={`/api/runs/${encodeURIComponent(selectedRunID)}/support.zip`}>导出支持包</a>}
          </div>
        </div>
        {pendingApproval && (
          <div className={`runApprovalCard${approvalActionable ? '' : ' locked'}`} role="alert" aria-label="待审批">
            <div className="runApprovalCardText">
              <strong>等待人工审批：{approvalNodeName || pendingApproval.node_id}</strong>
              <span>{pendingApproval.message || '审批节点没有填写提示，请确认前置改动已经生效后再继续。'}</span>
              {!approvalActionable && (
                <span className="runApprovalCardLockedNote">
                  该运行由其他进程（例如 CLI）启动，审批只能在启动它的终端处理；需要 Web 确认请从这里重新发起运行。
                </span>
              )}
              <span className="runApprovalCardMeta">
                节点 {pendingApproval.node_id}
                {pendingApproval.since ? ` · 开始等待 ${formatRunTime(pendingApproval.since)}` : ''}
                {formatApprovalWait(pendingApproval.since, approvalClock) ? ` · 已等待 ${formatApprovalWait(pendingApproval.since, approvalClock)}` : ''}
              </span>
            </div>
            <div className="runApprovalCardActions">
              <input
                aria-label="审批备注"
                value={approvalNote}
                disabled={!approvalActionable || Boolean(approvalAction)}
                placeholder={approvalActionable ? '可选备注，会写入运行记录' : '该运行无法在 Web 端提交审批'}
                onChange={event => setApprovalNote(event.target.value)}
              />
              <button type="button" className="primary" disabled={!approvalActionable || Boolean(approvalAction)} onClick={() => submitRunApproval(pendingApproval.node_id, 'approve')}>
                {approvalAction === 'approve' ? '确认中…' : '确认继续'}
              </button>
              <button type="button" className="secondary danger" disabled={!approvalActionable || Boolean(approvalAction)} onClick={() => submitRunApproval(pendingApproval.node_id, 'reject')}>
                {approvalAction === 'reject' ? '驳回中…' : '驳回'}
              </button>
            </div>
          </div>
        )}
        {selectedDetail ? (
          detailView === 'canvas' && selectedDetail.record?.kind === 'workflow' ? (
            <React.Suspense fallback={<div className="empty small">正在加载运行画布...</div>}>
              {selectedWorkflow ? (
                <RunRecordCanvas catalog={catalog} workflow={selectedWorkflow} detail={selectedDetail} />
              ) : (
                <div className="empty">{workflowMessage || '正在加载运行画布...'}</div>
              )}
            </React.Suspense>
          ) : (
            <RunDetail detail={selectedDetail} run={{id: selectedRunID, status: selectedDetail.record?.status}} />
          )
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
  if (status === 'pending') return '待处理'
  if (status === 'rolled_back') return '已回滚'
  if (status === 'cancelled') return '已取消'
  if (status === 'skipped') return '跳过'
  return status || '未知'
}

function formatRunTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function ConfigPanel({catalog, activeCategory, configSelectedPlugin, setConfigSelectedPlugin, configSelectedWorkflow, setConfigSelectedWorkflow, refreshCatalog}) {
  const [configView, setConfigView] = useState('tools')
  const {toolConfigItems, workflowConfigItems} = useMemo(() => buildConfigItemCollections(catalog, activeCategory), [catalog, activeCategory])

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
  if (configSelectedWorkflow) {
    return (
      <WorkflowConfigPanel
        workflow={configSelectedWorkflow}
        onBack={() => setConfigSelectedWorkflow(null)}
        onOpenPluginConfig={plugin => {
          setConfigSelectedWorkflow(null)
          setConfigSelectedPlugin(plugin)
        }}
        onSaved={async () => {
          await refreshCatalog()
        }}
      />
    )
  }

  const activeItems = configView === 'tools' ? toolConfigItems : workflowConfigItems
  const activeTitle = configView === 'tools' ? '工具配置' : '工作流配置'
  const activeEmptyText = configView === 'tools'
    ? '当前没有工具配置。工具配置只维护插件工具声明的一份配置文件。'
    : '当前没有工作流配置。'

  return (
    <div className="configPanelShell">
      <div className="tabs configSubTabs">
        <button className={configView === 'tools' ? 'tab active' : 'tab'} onClick={() => setConfigView('tools')}>工具配置 <span>{toolConfigItems.length}</span></button>
        <button className={configView === 'workflows' ? 'tab active' : 'tab'} onClick={() => setConfigView('workflows')}>工作流配置 <span>{workflowConfigItems.length}</span></button>
      </div>
      <section className="card listCard">
        <div className="cardHeader">
          <h3>{activeTitle}</h3>
          <span>{activeItems.length} 项</span>
        </div>
        <ConfigItemsList
          items={activeItems}
          onSelect={configView === 'tools' ? setConfigSelectedPlugin : setConfigSelectedWorkflow}
          emptyText={activeEmptyText}
        />
      </section>
    </div>
  )
}

function ConfigItemsList({items, onSelect, emptyText = '当前没有可配置项。'}) {
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
            {item.referencedPlugins?.length > 0 && (
              <small className="configReferencedPlugins">
                引用插件配置：{item.referencedPlugins.map(plugin => plugin.name || plugin.id).join('、')}
              </small>
            )}
            {item.disabled && <small>状态：插件已禁用</small>}
          </div>
        </button>
      ))}
      {items.length === 0 && <div className="empty">{emptyText}</div>}
    </div>
  )
}

const marketStatusLabels = {
  not_installed: '未安装',
  installed: '已安装',
  update_available: '可更新',
  local_newer: '本地较新',
  disabled: '已禁用'
}

function ListViewIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path d="M5.5 4h8M5.5 8h8M5.5 12h8" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
      <circle cx="2.6" cy="4" r="1" fill="currentColor" />
      <circle cx="2.6" cy="8" r="1" fill="currentColor" />
      <circle cx="2.6" cy="12" r="1" fill="currentColor" />
    </svg>
  )
}

function GridViewIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <rect x="2.2" y="2.2" width="5" height="5" rx="1.2" stroke="currentColor" strokeWidth="1.4" fill="none" />
      <rect x="8.8" y="2.2" width="5" height="5" rx="1.2" stroke="currentColor" strokeWidth="1.4" fill="none" />
      <rect x="2.2" y="8.8" width="5" height="5" rx="1.2" stroke="currentColor" strokeWidth="1.4" fill="none" />
      <rect x="8.8" y="8.8" width="5" height="5" rx="1.2" stroke="currentColor" strokeWidth="1.4" fill="none" />
    </svg>
  )
}

function FolderIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path d="M2.3 4.4c0-.7.6-1.2 1.2-1.2h2.7l1.3 1.4h5c.7 0 1.2.6 1.2 1.2v5.8c0 .7-.6 1.2-1.2 1.2H3.5c-.7 0-1.2-.6-1.2-1.2V4.4z" fill="none" stroke="currentColor" strokeWidth="1.3" strokeLinejoin="round" />
    </svg>
  )
}

function CaretDownIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path d="M4 6.3 8 10.1 12 6.3" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function CaretRightIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path d="M6.3 4 10.1 8 6.3 12" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function ChevronLeftIcon() {
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path d="M10.1 4 6.3 8 10.1 12" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function GearIcon() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path fillRule="evenodd" clipRule="evenodd" d="M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58a.49.49 0 0 0 .12-.61l-1.92-3.32a.488.488 0 0 0-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54a.484.484 0 0 0-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.09.63-.09.94s.02.64.07.94l-2.03 1.58a.49.49 0 0 0-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z" />
    </svg>
  )
}

export function PluginManagerModal({catalog, state, setState, onClose, onUploaded, onChanged, onRefresh}) {
  const managerRef = useRef(null)
  const [tab, setTab] = useState('installed')
  const [uploadOpen, setUploadOpen] = useState(false)
  const [pluginActionID, setPluginActionID] = useState('')
  const [rescanning, setRescanning] = useState(false)
  const [rescanMessage, setRescanMessage] = useState('')
  const [rescanStatus, setRescanStatus] = useState('')
  const [downloadPlatform, setDownloadPlatform] = useState('windows/amd64')
  const [market, setMarket] = useState(null)
  const [marketLoading, setMarketLoading] = useState(false)
  const [marketError, setMarketError] = useState('')
  const [marketSearch, setMarketSearch] = useState('')
  const [marketSourceID, setMarketSourceID] = useState('')
  const [sourceManagerOpen, setSourceManagerOpen] = useState(false)
  const [installedView, setInstalledView] = useState('list')
  const [installedSearch, setInstalledSearch] = useState('')
  const [downloadMenu, setDownloadMenu] = useState(null)
  const [marketView, setMarketView] = useState('list')
  const [installingKey, setInstallingKey] = useState('')
  const [exportOpen, setExportOpen] = useState(false)
  const [shareTargets, setShareTargets] = useState([])
  const [shareTargetsLoading, setShareTargetsLoading] = useState(true)
  const [shareTargetsMessage, setShareTargetsMessage] = useState('')
  const [sharePlugin, setSharePlugin] = useState(null)
  const plugins = useMemo(() => [...(catalog?.plugins || [])].sort((left, right) => String(left.id || '').localeCompare(String(right.id || ''), 'zh-CN')), [catalog])
  const filteredPlugins = useMemo(() => {
    const keyword = installedSearch.trim().toLowerCase()
    if (!keyword) return plugins
    return plugins.filter(item => [
      item.id,
      item.name,
      item.description,
      item.version,
      item.disabled ? '已禁用 disabled' : '启用中 enabled'
    ].filter(Boolean).join(' ').toLowerCase().includes(keyword))
  }, [plugins, installedSearch])
  const userWorkflows = useMemo(() => {
    return [...(catalog?.workflows || [])]
      .filter(item => item.source?.plugin_id === 'user.workflows')
      .sort((left, right) => String(left.name || left.id).localeCompare(String(right.name || right.id), 'zh-CN'))
  }, [catalog])
  const workflowCountForPlugin = pluginID => (catalog?.workflows || []).filter(item => item.source?.plugin_id === pluginID).length
  const marketSources = market?.sources || []
  const shareReady = shareTargets.length > 0
  const shareHint = shareTargetsMessage || '未配置远程插件市场（Nexus），分享功能不可用。可在「插件市场 → 来源配置」中新增 Nexus 来源。'

  useModalEscape(onClose, managerRef)

  useEffect(() => {
    setState(current => (current?.message || current?.response) ? {message: ''} : current)
  }, [setState])

  useEffect(() => {
    let cancelled = false
    async function loadShareTargets() {
      setShareTargetsLoading(true)
      try {
        const body = await fetchNexusShareTargets()
        if (cancelled) return
        const data = body.data || {}
        setShareTargets(Array.isArray(data.targets) ? data.targets : [])
        setShareTargetsMessage(data.message || '')
      } catch (err) {
        if (!cancelled) {
          setShareTargets([])
          setShareTargetsMessage(readableAPIError(err, '读取远程插件市场配置失败，分享功能暂不可用。'))
        }
      } finally {
        if (!cancelled) setShareTargetsLoading(false)
      }
    }
    loadShareTargets()
    return () => {
      cancelled = true
    }
  }, [])
  useEffect(() => {
    if (marketSourceID && !marketSources.some(source => source.id === marketSourceID)) {
      setMarketSourceID('')
    }
  }, [marketSources, marketSourceID])
  const marketItems = useMemo(() => {
    const items = []
    for (const source of marketSources) {
      for (const item of source.plugins || []) {
        items.push({...item, source_id: item.source_id || source.id, source_name: item.source_name || source.name})
      }
    }
    return items
  }, [market])
  const filteredMarketItems = useMemo(() => {
    const keyword = marketSearch.trim().toLowerCase()
    return marketItems.filter(item => {
      if (marketSourceID && item.source_id !== marketSourceID) return false
      if (!keyword) return true
      const haystack = [
        item.id,
        item.name,
        item.description,
        item.author,
        ...(item.tags || []),
        ...(item.categories || []).map(category => `${category.id} ${category.name}`)
      ].join(' ').toLowerCase()
      return haystack.includes(keyword)
    })
  }, [marketItems, marketSearch, marketSourceID])
  const marketRows = useMemo(() => filteredMarketItems.map(item => {
    const status = item.install_status || 'not_installed'
    const key = `${item.source_id}/${item.id}@${item.version}`
    const installing = installingKey === key
    const disabled = Boolean(installingKey) || item.compatible === false || ['installed', 'local_newer', 'disabled'].includes(status)
    const label = installing ? '处理中...' : status === 'update_available' ? '更新' : status === 'installed' ? '已安装' : status === 'local_newer' ? '本地较新' : status === 'disabled' ? '已禁用' : '安装'
    return {...item, status, key, installing, disabled, label}
  }), [filteredMarketItems, installingKey])
  const DOWNLOAD_MENU_HEIGHT = {type: 148, platform: 272}
  const downloadMenuTop = (anchorTop, anchorBottom, height) => {
    if (anchorBottom + height + 8 <= window.innerHeight) return anchorBottom + 8
    if (anchorTop - height - 8 >= 8) return anchorTop - height - 8
    return Math.max(8, window.innerHeight - height - 8)
  }

  function toggleDownloadMenu(item, event) {
    if (downloadMenu?.pluginID === item.id) {
      setDownloadMenu(null)
      return
    }
    const rect = event.currentTarget.getBoundingClientRect()
    const width = 248
    const anchorTop = rect.top
    const anchorBottom = rect.bottom
    setDownloadMenu({
      pluginID: item.id,
      item,
      stage: 'type',
      width,
      anchorTop,
      anchorBottom,
      left: Math.max(8, Math.min(rect.right - width, window.innerWidth - width - 8)),
      top: downloadMenuTop(anchorTop, anchorBottom, DOWNLOAD_MENU_HEIGHT.type)
    })
  }

  function switchDownloadStage(stage) {
    setDownloadMenu(previous => previous ? {
      ...previous,
      stage,
      top: downloadMenuTop(previous.anchorTop, previous.anchorBottom, DOWNLOAD_MENU_HEIGHT[stage])
    } : previous)
  }

  useEffect(() => {
    if (!downloadMenu) return
    function handlePointerDown(event) {
      if (!(event.target instanceof Element)) return
      if (event.target.closest('.pluginDownloadPopover')) return
      if (event.target.closest('[data-download-toggle]')) return
      setDownloadMenu(null)
    }
    function handleKey(event) {
      if (event.key === 'Escape') setDownloadMenu(null)
    }
    const close = () => setDownloadMenu(null)
    document.addEventListener('mousedown', handlePointerDown)
    document.addEventListener('keydown', handleKey)
    window.addEventListener('resize', close)
    window.addEventListener('scroll', close, true)
    return () => {
      document.removeEventListener('mousedown', handlePointerDown)
      document.removeEventListener('keydown', handleKey)
      window.removeEventListener('resize', close)
      window.removeEventListener('scroll', close, true)
    }
  }, [downloadMenu])
  const targets = [
    ['windows/amd64', 'Windows amd64'],
    ['windows/arm64', 'Windows arm64'],
    ['linux/amd64', 'Linux amd64'],
    ['linux/arm64', 'Linux arm64'],
    ['darwin/amd64', 'macOS amd64'],
    ['darwin/arm64', 'macOS arm64']
  ]
  const runtimeBase = item => `/api/plugins/${encodeURIComponent(item.id)}/runtime`

  async function loadMarket(refresh = false) {
    setMarketLoading(true)
    setMarketError('')
    try {
      const body = await fetchMarket(refresh)
      setMarket(body.data || {sources: []})
    } catch (err) {
      setMarketError(readableAPIError(err, '加载插件市场失败。'))
    } finally {
      setMarketLoading(false)
    }
  }

  useEffect(() => {
    if (tab === 'market' && !market && !marketLoading) loadMarket(false)
  }, [tab])

  // 打开插件管理时先按磁盘重建注册表：插件目录被外部新增或替换后，
  // 服务内存里的注册表会滞后，直接读列表会出现“插件明明在磁盘上却看不到”“版本号还是旧的”。
  useEffect(() => {
    rescan(true)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function rescan(silent = false) {
    setRescanning(true)
    setRescanStatus('')
    if (!silent) setRescanMessage('正在同步插件目录...')
    try {
      const body = await rescanPlugins()
      const data = body.data || {}
      if (onRefresh) await onRefresh()
      if (silent) {
        setRescanMessage('')
      } else {
        setRescanMessage(`插件目录已同步：插件 ${data.plugins ?? 0} 个，工具 ${data.tools ?? 0} 个。`)
        setRescanStatus('success')
      }
    } catch (err) {
      setRescanMessage(readableAPIError(err, '刷新插件目录失败。'))
      setRescanStatus('error')
    } finally {
      setRescanning(false)
    }
  }

  async function installMarketPlugin(item) {
    const replace = item.install_status === 'update_available'
    const action = replace ? '更新' : '安装'
    if (!window.confirm(`将从「${item.source_name || item.source_id}」${action}插件 ${item.name || item.id}@${item.version}。\n\n插件会写入本机插件目录，并在执行工具时运行本地脚本，请确认来源可信。`)) return
    const key = `${item.source_id}/${item.id}@${item.version}`
    setInstallingKey(key)
    setState({message: `正在从插件市场${action} ${item.id}...`})
    try {
      const body = await postMarketInstall({source_id: item.source_id, plugin_id: item.id, version: item.version, replace})
      setState({message: `插件 ${item.id} ${body.status === 'updated' ? '更新' : '安装'}成功。`, response: body})
      await onUploaded(body)
      await loadMarket(true)
    } catch (err) {
      setState({message: readableAPIError(err, `${action}插件失败。`), response: err.body})
    } finally {
      setInstallingKey('')
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

  function installedPluginActions(item) {
    const shareDisabled = !shareReady || pluginActionID === item.id
    return (
      <div className="pluginItemActions">
        {!item.disabled && (
          <button
            className="secondary pluginDownloadToggle"
            type="button"
            data-download-toggle
            aria-haspopup="menu"
            aria-expanded={downloadMenu?.pluginID === item.id}
            title="选择下载插件 ZIP 或完整运行包 ZIP"
            onClick={event => toggleDownloadMenu(item, event)}
          >
            下载
            <CaretDownIcon />
          </button>
        )}
        {!item.disabled && (
          <button
            className="secondary"
            type="button"
            disabled={shareDisabled}
            title={shareReady ? '打包插件并上传到远程插件市场（Nexus）' : shareHint}
            onClick={() => setSharePlugin(item)}
          >
            分享
          </button>
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
    )
  }

  function marketPluginAction(item) {
    return (
      <div className="marketPluginActions">
        <button className={item.status === 'update_available' ? 'primary' : 'secondary'} type="button" disabled={item.disabled} onClick={() => installMarketPlugin(item)}>{item.label}</button>
      </div>
    )
  }

  function installedPluginCard(item) {
    const workflowCount = workflowCountForPlugin(item.id)
    return (
      <div className={item.disabled ? 'pluginExportItem pluginDisabled' : 'pluginExportItem'} key={item.id}>
        <div>
          <strong>{item.name || item.id}</strong>
          <span>{item.id}@{item.version || '-'}</span>
          <small>{item.disabled ? '状态：已禁用' : '状态：启用中'}{workflowCount > 0 ? ` · 含 ${workflowCount} 个工作流` : ''}</small>
          {item.description && <small>{item.description}</small>}
        </div>
        {installedPluginActions(item)}
      </div>
    )
  }

  function marketPluginCard(item) {
    return (
      <article className="marketPluginCard" key={item.key}>
        <div>
          <div className="marketPluginTitle">
            <strong>{item.name || item.id}</strong>
            <span className="marketPluginVersion">v{item.version}</span>
            <span className={`marketBadge ${item.status}`}>{marketStatusLabels[item.status] || item.status}</span>
          </div>
          <span className="marketPluginID">{item.id} · {item.source_name || item.source_id}</span>
          {item.description && <small>{item.description}</small>}
          <div className="marketPluginMeta">
            {item.author && <span>作者：{item.author}</span>}
            <span>{item.tool_count || 0} 个工具</span>
            <span>{item.workflow_count || 0} 个工作流</span>
            {item.installed && item.installed_version && <span>本地版本：{item.installed_version}</span>}
          </div>
          {(item.tags || []).length > 0 && (
            <div className="marketTagRow">
              {item.tags.map(tag => <span className="marketTag" key={tag}>{tag}</span>)}
            </div>
          )}
          {item.compatible === false && <small className="marketIncompatible">{item.compatibility_message || '当前版本不兼容此插件。'}</small>}
          {item.status === 'disabled' && <small className="marketIncompatible">插件已禁用，请先在「已安装」页签启用后再更新。</small>}
        </div>
        {marketPluginAction(item)}
      </article>
    )
  }

  return (
    <>
    <div className="modalBackdrop" onClick={onClose}>
      <div className="modal pluginManagerModal" ref={managerRef} onClick={event => event.stopPropagation()}>
        <div className="modalHeader">
          <div>
            <h3>插件管理</h3>
            <p>上传插件、浏览插件市场，并管理本机已安装插件。</p>
          </div>
          <button className="modalClose" onClick={onClose}>×</button>
        </div>
        <div className="pluginManagerTabs" role="tablist" aria-label="插件管理页签">
          <button type="button" role="tab" aria-selected={tab === 'installed'} className={tab === 'installed' ? 'active' : ''} onClick={() => { setTab('installed'); setSourceManagerOpen(false) }}>已安装</button>
          <button type="button" role="tab" aria-selected={tab === 'market'} className={tab === 'market' ? 'active' : ''} onClick={() => setTab('market')}>插件市场</button>
        </div>
        {tab === 'installed' ? (
        <div className="pluginModalActions">
          <section className="pluginQuickActions">
            <button type="button" className="primary" onClick={() => setUploadOpen(true)}>安装插件</button>
            <button type="button" className="secondary" onClick={() => setExportOpen(true)}>导出用户工作流</button>
          </section>
          <section className="pluginInstalledListSection">
            {rescanMessage && <div className={`pluginOperationMessage${rescanStatus ? ` ${rescanStatus}` : ''}`}>{rescanMessage}</div>}
            <div className="pluginInstalledHeader">
              <div>
                <strong>已安装插件</strong>
                <span>{installedSearch.trim() ? `匹配 ${filteredPlugins.length} / ${plugins.length} 个插件，删除前需要先禁用。` : `${plugins.length} 个插件，删除前需要先禁用。`}</span>
              </div>
            </div>
            <div className="pluginInstalledToolbar">
              <input value={installedSearch} onChange={event => setInstalledSearch(event.target.value)} placeholder="搜索插件名称、ID、描述..." aria-label="搜索已安装插件" />
              <div className="pluginInstalledToolbarActions">
                <div className="viewSwitch" role="group" aria-label="已安装插件视图">
                  <button type="button" aria-pressed={installedView === 'list'} className={installedView === 'list' ? 'active' : ''} onClick={() => setInstalledView('list')}>
                    <ListViewIcon />
                    <span>列表</span>
                  </button>
                  <button type="button" aria-pressed={installedView === 'table'} className={installedView === 'table' ? 'active' : ''} onClick={() => setInstalledView('table')}>
                    <GridViewIcon />
                    <span>表格</span>
                  </button>
                </div>
                <button className="secondary" type="button" disabled={rescanning} title="刷新插件目录，同步磁盘上外部新增或替换的插件" onClick={() => rescan(false)}>{rescanning ? '刷新中...' : '刷新'}</button>
              </div>
            </div>
            {!shareTargetsLoading && !shareReady && (
              <small className="marketHint pluginShareHint">{shareHint}</small>
            )}
            {installedView === 'list' ? (
              <div className="pluginExportList">
                {filteredPlugins.map(installedPluginCard)}
                {filteredPlugins.length === 0 && <div className="empty small">{plugins.length === 0 ? '当前没有已安装插件。' : '没有匹配的插件。'}</div>}
              </div>
            ) : (
              <div className="pluginGrid">
                {filteredPlugins.map(installedPluginCard)}
                {filteredPlugins.length === 0 && <div className="empty small">{plugins.length === 0 ? '当前没有已安装插件。' : '没有匹配的插件。'}</div>}
              </div>
            )}
          </section>
        </div>
        ) : (
          <div className="pluginMarketPanel">
            <div className="marketToolbar">
              <input value={marketSearch} onChange={event => setMarketSearch(event.target.value)} placeholder="搜索插件名称、ID、标签..." />
              <div className="marketToolbarActions">
                {marketSources.length > 1 && (
                  <select value={marketSourceID} onChange={event => setMarketSourceID(event.target.value)}>
                    <option value="">全部来源</option>
                    {marketSources.map(source => <option key={source.id} value={source.id}>{source.name || source.id}{source.type === 'local' ? '（本地）' : source.type === 'remote' ? '（远程）' : source.type === 'nexus' ? '（Nexus）' : ''}</option>)}
                  </select>
                )}
                <div className="viewSwitch" role="group" aria-label="插件市场视图">
                  <button type="button" aria-pressed={marketView === 'list'} className={marketView === 'list' ? 'active' : ''} onClick={() => setMarketView('list')}>
                    <ListViewIcon />
                    <span>列表</span>
                  </button>
                  <button type="button" aria-pressed={marketView === 'table'} className={marketView === 'table' ? 'active' : ''} onClick={() => setMarketView('table')}>
                    <GridViewIcon />
                    <span>表格</span>
                  </button>
                </div>
                <button className="secondary" type="button" disabled={marketLoading} onClick={() => loadMarket(true)}>{marketLoading ? '刷新中...' : '刷新'}</button>
                <button className="secondary iconButton" type="button" title="市场来源配置" aria-label="市场来源配置" onClick={() => setSourceManagerOpen(true)}>
                  <GearIcon />
                </button>
              </div>
            </div>
            <small className="marketHint">插件会写入本机插件目录，并在执行时运行本地脚本，安装前请确认来源可信。</small>
            {marketSources.length === 1 && (
              <small className="marketHint">
                当前来源：{marketSources[0].name || marketSources[0].id}
                {marketSources[0].type === 'local' ? `（本地目录：${marketSources[0].location || marketSources[0].url}）` : marketSources[0].type === 'nexus' ? '（Nexus 仓库）' : ''}
              </small>
            )}
            {marketError && <div className="marketSourceError">{marketError}</div>}
            {marketSources.filter(source => source.status === 'error').map(source => (
              <div className="marketSourceError" key={source.id}>
                <strong>{source.name || source.id}</strong>：{source.error}
              </div>
            ))}
            {!marketLoading && marketSources.length === 0 && !marketError && (
              <div className="empty small">未找到可用的插件市场源。点击右上角齿轮按钮新增本地目录、远程索引或 Nexus 仓库。</div>
            )}
            {marketLoading && marketItems.length === 0 && <div className="empty small">正在加载插件市场...</div>}
            {marketView === 'list' ? (
              <div className="marketPluginList">
                {marketRows.map(marketPluginCard)}
              </div>
            ) : (
              <div className="pluginGrid">
                {marketRows.map(marketPluginCard)}
              </div>
            )}
            {!marketLoading && marketSources.some(source => source.status === 'ok') && marketRows.length === 0 && <div className="empty small">没有匹配的插件。</div>}
          </div>
        )}
        {sourceManagerOpen && (
          <MarketSourcesModal
            onClose={() => setSourceManagerOpen(false)}
            onChanged={() => {
              setMarket(null)
              setMarketError('')
              loadMarket(true)
            }}
          />
        )}
        {uploadOpen && (
          <PluginUploadModal
            onClose={() => setUploadOpen(false)}
            onUploaded={onUploaded}
            onRefresh={onRefresh}
          />
        )}
        {exportOpen && (
          <WorkflowExportModal
            workflows={userWorkflows}
            categories={catalog?.categories || []}
            onClose={() => setExportOpen(false)}
          />
        )}
        {sharePlugin && (
          <PluginShareModal
            plugin={sharePlugin}
            targets={shareTargets}
            onClose={() => setSharePlugin(null)}
            onShared={() => setMarket(null)}
          />
        )}
        {state.message && <pre className="modalResult">{state.message}</pre>}
      </div>
    </div>
    {downloadMenu && createPortal(
      <div
        className="pluginDownloadPopover"
        role="menu"
        aria-label="下载类型"
        style={{top: downloadMenu.top, left: downloadMenu.left, width: downloadMenu.width}}
      >
        {downloadMenu.stage === 'platform' ? (
          <>
            <button
              className="pluginDownloadBack"
              type="button"
              onClick={() => switchDownloadStage('type')}
            >
              <ChevronLeftIcon />
              选择运行包平台
            </button>
            {targets.map(([value, label]) => {
              const [goosValue, goarchValue] = value.split('/')
              const active = value === downloadPlatform
              return (
                <a
                  key={value}
                  role="menuitem"
                  className={active ? 'pluginDownloadPlatform active' : 'pluginDownloadPlatform'}
                  href={`${runtimeBase(downloadMenu.item)}.zip?goos=${encodeURIComponent(goosValue)}&goarch=${encodeURIComponent(goarchValue)}`}
                  title={`下载 ${label} 完整运行包，解压即可执行`}
                  onClick={() => {
                    setDownloadPlatform(value)
                    setDownloadMenu(null)
                  }}
                >
                  <span>{label}</span>
                  {active && <small>上次下载</small>}
                </a>
              )
            })}
          </>
        ) : (
          <>
            <a
              role="menuitem"
              href={`/api/plugins/${encodeURIComponent(downloadMenu.item.id)}.zip`}
              title="插件 ZIP 会自动包含归属于该插件的工作流"
              onClick={() => setDownloadMenu(null)}
            >
              <span>插件 ZIP</span>
              <small>插件配置与归属工作流，用于分享或装到其他环境</small>
            </a>
            <button
              className="pluginDownloadNext"
              role="menuitem"
              type="button"
              onClick={() => switchDownloadStage('platform')}
            >
              <span className="pluginDownloadNextText">
                <span>运行包 ZIP</span>
                <small>选择运行包平台后下载</small>
              </span>
              <CaretRightIcon />
            </button>
          </>
        )}
      </div>,
      document.body
    )}
    </>
  )
}

function PluginUploadModal({onClose, onUploaded, onRefresh}) {
  const uploadRef = useRef(null)
  const confirmRef = useRef(null)
  const [file, setFile] = useState(null)
  const [uploading, setUploading] = useState(false)
  const [message, setMessage] = useState('')
  const [messageError, setMessageError] = useState(false)
  const [confirmDetail, setConfirmDetail] = useState(null)

  function requestClose() {
    if (uploading) return
    onClose()
  }

  useModalEscape(requestClose, uploadRef)
  useModalEscape(() => setConfirmDetail(null), confirmRef, Boolean(confirmDetail))

  function selectFile(event) {
    const nextFile = event.target.files?.[0] || null
    // 清空 input 的 value：重新打包同名 ZIP 后再选同一个文件名时，浏览器不会触发 change，
    // 状态会一直停留在旧文件上，导致上传的仍是上一次的内容（版本号永远不变）。
    event.target.value = ''
    setFile(nextFile)
    setConfirmDetail(null)
    setMessageError(false)
    setMessage(nextFile ? `已选择 ${nextFile.name}（${formatBytes(nextFile.size)}，修改时间 ${new Date(nextFile.lastModified).toLocaleString()}），点击上传开始安装。` : '')
  }

  async function upload(replace = false) {
    if (!file) {
      setMessage('请先选择插件 ZIP 包。')
      setMessageError(true)
      return
    }
    setUploading(true)
    setMessageError(false)
    setMessage(replace ? '正在更新插件...' : '正在安装插件...')
    try {
      const body = await postPluginZip(file, replace)
      setMessage(`插件${body.status === 'updated' ? '更新' : '安装'}成功。`)
      setConfirmDetail(null)
      setFile(null)
      await onUploaded(body)
    } catch (err) {
      const detail = err.body?.data
      if (err.status === 409 && detail?.existing) {
        setConfirmDetail(detail)
        setMessage('插件已存在，需确认覆盖后才会写入。')
        // 同步一次插件目录：插件可能是外部新增/替换的，当前列表尚未包含它。
        if (onRefresh) Promise.resolve(onRefresh()).catch(() => {})
      } else {
        setMessage(readableAPIError(err, '安装插件失败。'))
        setMessageError(true)
      }
    } finally {
      setUploading(false)
    }
  }

  return (
    <div className="modalBackdrop modalBackdropNested" onMouseDown={event => { if (event.target === event.currentTarget) requestClose() }}>
      <div className="modal pluginOperationModal" ref={uploadRef} role="dialog" aria-modal="true" aria-labelledby="plugin-upload-title">
        <div className="modalHeader">
          <div>
            <h3 id="plugin-upload-title">安装插件</h3>
            <p>上传插件 ZIP 包，安装到本机插件目录。安装后如需更新同 ID 插件，可再次上传并确认更新。</p>
          </div>
          <button className="modalClose" type="button" onClick={requestClose} aria-label="关闭">×</button>
        </div>
        <div className="pluginUploadForm">
          <label>
            <span>插件 ZIP 包</span>
            <input type="file" accept=".zip,application/zip" onChange={selectFile} />
          </label>
          {message && <div className={messageError ? 'pluginOperationMessage error' : 'pluginOperationMessage'}>{message}</div>}
          <div className="buttonRow">
            <button className="primary" type="button" disabled={!file || uploading} onClick={() => upload(false)}>{uploading ? '正在处理...' : '上传'}</button>
            <a className="secondary downloadTemplate" href="/api/dev/toolkit.zip">下载插件模板</a>
            <button className="secondary" type="button" onClick={requestClose}>关闭</button>
          </div>
        </div>
      </div>
      {confirmDetail && (
        <div className="modalBackdrop modalBackdropNested" onMouseDown={event => { if (event.target === event.currentTarget) setConfirmDetail(null) }}>
          <div className="modal pluginOperationModal pluginConfirmModal" ref={confirmRef} role="dialog" aria-modal="true" aria-labelledby="plugin-confirm-title">
            <div className="modalHeader">
              <div>
                <h3 id="plugin-confirm-title">覆盖已安装插件</h3>
                <p>{confirmDetail.plugin_id || '该插件'} 已安装在本机，继续将用本次上传的 ZIP 包整体替换现有插件目录。</p>
              </div>
              <button className="modalClose" type="button" onClick={() => setConfirmDetail(null)} aria-label="关闭">×</button>
            </div>
            <div className="pluginVersionCompare">
              <div>
                <span>已安装版本</span>
                <strong>{confirmDetail.existing_version || '-'}</strong>
              </div>
              <div>
                <span>本次上传版本</span>
                <strong>{confirmDetail.version || '-'}</strong>
              </div>
            </div>
            <p className="pluginConfirmHint">{uploadVersionHint(confirmDetail)}</p>
            <div className="modalFooter">
              <button className="secondary" type="button" disabled={uploading} onClick={() => setConfirmDetail(null)}>取消</button>
              <button className="primary" type="button" disabled={uploading} onClick={() => upload(true)}>{uploading ? '正在覆盖...' : '确认覆盖'}</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function uploadVersionHint(detail) {
  if (!detail) return ''
  if (detail.same_version) {
    return `版本号未变化（${detail.version || '-'}），将以本次上传内容覆盖现有插件；请确认 ZIP 是重新打包后的最新文件。`
  }
  return `将把已安装的 ${detail.existing_version || '-'} 覆盖为 ${detail.version || '-'}，现有插件目录会被整体替换。`
}

function WorkflowExportModal({workflows, categories = [], onClose}) {
  const exportRef = useRef(null)
  const [selection, setSelection] = useState(() => workflows.map(item => item.id))
  const [includePlugins, setIncludePlugins] = useState(true)
  const [exporting, setExporting] = useState(false)
  const [message, setMessage] = useState('')
  const [messageError, setMessageError] = useState(false)
  const [activeCategory, setActiveCategory] = useState('')
  const [search, setSearch] = useState('')

  useModalEscape(onClose, exportRef)
  const selectedWorkflows = useMemo(() => workflows.filter(item => selection.includes(item.id)), [workflows, selection])
  const categoryLabels = useMemo(() => new Map(categories.map(item => [item.id, item.name || item.id])), [categories])
  const categoryGroups = useMemo(() => {
    const map = new Map()
    for (const workflow of workflows) {
      const category = String(workflow.category || '').trim()
      const key = category || '__uncategorized__'
      const group = map.get(key)
      if (group) group.count += 1
      else map.set(key, {id: key, label: workflowCategoryDisplayName(category, categoryLabels), count: 1})
    }
    return [...map.values()].sort((left, right) => {
      if (left.id === '__uncategorized__') return 1
      if (right.id === '__uncategorized__') return -1
      return left.label.localeCompare(right.label, 'zh-CN')
    })
  }, [workflows, categoryLabels])
  const visibleWorkflows = useMemo(() => {
    const keyword = search.trim().toLowerCase()
    return workflows.filter(item => {
      if (activeCategory) {
        const category = String(item.category || '').trim() || '__uncategorized__'
        if (category !== activeCategory) return false
      }
      if (!keyword) return true
      const category = String(item.category || '').trim()
      const categoryLabel = workflowCategoryDisplayName(category, categoryLabels)
      return [item.name || '', item.id || '', category, categoryLabel].join(' ').toLowerCase().includes(keyword)
    })
  }, [workflows, activeCategory, search, categoryLabels])
  const activeCategoryLabel = activeCategory ? (categoryGroups.find(group => group.id === activeCategory)?.label || '分类') : '全部工作流'
  const visibleSelectedCount = visibleWorkflows.filter(item => selection.includes(item.id)).length
  const dependencies = useMemo(() => {
    const map = new Map()
    for (const workflow of selectedWorkflows) {
      for (const dependency of workflow.dependencies || []) {
        map.set(dependency.id, dependency)
      }
    }
    return [...map.values()].sort((left, right) => String(left.id).localeCompare(String(right.id), 'zh-CN'))
  }, [selectedWorkflows])
  const missingTools = useMemo(() => {
    const tools = new Set()
    for (const workflow of selectedWorkflows) {
      for (const tool of workflow.missing_tools || []) tools.add(tool)
    }
    return [...tools].sort((left, right) => left.localeCompare(right, 'zh-CN'))
  }, [selectedWorkflows])

  function toggleWorkflow(workflowID) {
    setSelection(current => current.includes(workflowID) ? current.filter(id => id !== workflowID) : [...current, workflowID])
  }

  function selectVisibleWorkflows() {
    setSelection(current => [...new Set([...current, ...visibleWorkflows.map(item => item.id)])])
  }

  async function exportWorkflows() {
    const ids = selectedWorkflows.map(item => item.id)
    if (ids.length === 0) {
      setMessage('请至少选择一个用户工作流。')
      setMessageError(true)
      return
    }
    setExporting(true)
    setMessageError(false)
    setMessage(`正在导出 ${ids.length} 个用户工作流...`)
    try {
      const filename = await downloadUserWorkflowExport(ids, includePlugins)
      const pluginText = includePlugins && dependencies.length > 0 ? `及 ${dependencies.length} 个依赖插件` : ''
      setMessage(`已导出 ${ids.length} 个用户工作流${pluginText}：${filename}`)
    } catch (err) {
      setMessage(readableAPIError(err, '导出用户工作流失败。'))
      setMessageError(true)
    } finally {
      setExporting(false)
    }
  }

  return (
    <div className="modalBackdrop modalBackdropNested" onMouseDown={event => { if (event.target === event.currentTarget) onClose() }}>
      <div className="modal pluginOperationModal workflowExportModal" ref={exportRef} role="dialog" aria-modal="true" aria-labelledby="workflow-export-title">
        <div className="modalHeader">
          <div>
            <h3 id="workflow-export-title">导出用户工作流</h3>
            <p>选择要导出的用户工作流，导出的 ZIP 可直接作为 user.workflows 插件安装。</p>
          </div>
          <button className="modalClose" type="button" onClick={onClose} aria-label="关闭">×</button>
        </div>
        <div className="workflowExportPanel">
          <div className="workflowExportHeader">
            <div>
              <strong>选择要导出的工作流</strong>
              <span>共 {workflows.length} 个用户工作流，已选择 {selectedWorkflows.length} 个；可跨分类勾选。</span>
            </div>
            <div className="workflowExportHeaderActions">
              <button type="button" className="secondary" disabled={visibleWorkflows.length === 0} onClick={selectVisibleWorkflows}>全选当前</button>
              <button type="button" className="secondary" disabled={selectedWorkflows.length === 0} onClick={() => setSelection([])}>清空</button>
            </div>
          </div>
          <div className="workflowExportLayout">
            <nav className="workflowExportTree" aria-label="工作流目录">
              <strong>目录</strong>
              <button type="button" className={activeCategory ? '' : 'active'} onClick={() => setActiveCategory('')}>
                <FolderIcon />
                <span className="workflowExportTreeLabel">全部工作流</span>
                <span className="workflowExportTreeCount">{workflows.length}</span>
              </button>
              {categoryGroups.length > 0 && (
                <div className="workflowExportTreeChildren">
                  {categoryGroups.map(group => (
                    <button type="button" key={group.id} className={activeCategory === group.id ? 'treeChild active' : 'treeChild'} onClick={() => setActiveCategory(group.id)}>
                      <FolderIcon />
                      <span className="workflowExportTreeLabel">{group.label}</span>
                      <span className="workflowExportTreeCount">{group.count}</span>
                    </button>
                  ))}
                </div>
              )}
            </nav>
            <div className="workflowExportBrowser">
              <div className="workflowExportBrowserHeader">
                <div>
                  <strong>{activeCategoryLabel}</strong>
                  <span>{visibleWorkflows.length} 个工作流 · 已选 {visibleSelectedCount} 个</span>
                </div>
                <input className="workflowExportSearch" type="search" value={search} onChange={event => setSearch(event.target.value)} placeholder="搜索名称、ID、分类..." aria-label="搜索工作流" />
              </div>
              <div className="workflowExportList">
                {visibleWorkflows.map(item => (
                  <label className={selection.includes(item.id) ? 'workflowExportItem selected' : 'workflowExportItem'} key={item.id}>
                    <input type="checkbox" checked={selection.includes(item.id)} onChange={() => toggleWorkflow(item.id)} />
                    <span>
                      <strong>{item.name || item.id}</strong>
                      <small>{item.id}{item.category ? ` · ${workflowCategoryDisplayName(String(item.category).trim(), categoryLabels)}` : ''}</small>
                      {(item.dependencies || []).length > 0 && <small className="workflowExportDeps">依赖：{item.dependencies.map(dep => dep.name || dep.id).join('、')}</small>}
                      {(item.missing_tools || []).length > 0 && <small className="workflowExportWarning">缺少工具：{item.missing_tools.join('、')}</small>}
                    </span>
                  </label>
                ))}
                {visibleWorkflows.length === 0 && <div className="empty small">{workflows.length === 0 ? '当前没有用户工作流。' : '当前条件下没有匹配的工作流。'}</div>}
              </div>
            </div>
          </div>
          <label className="workflowExportDependencyToggle">
            <input type="checkbox" checked={includePlugins} onChange={event => setIncludePlugins(event.target.checked)} />
            <span>同时导出引用的插件{dependencies.length > 0 ? `（${dependencies.length} 个）` : ''}</span>
          </label>
          {includePlugins && dependencies.length > 0 && (
            <div className="workflowExportDependencyList">
              {dependencies.map(dependency => (
                <span className="marketTag" key={dependency.id}>{dependency.name || dependency.id}@{dependency.version || '-'}{dependency.disabled ? '（已禁用）' : ''}</span>
              ))}
            </div>
          )}
          {includePlugins && missingTools.length > 0 && (
            <small className="workflowExportWarning">以下工具没有找到对应插件：{missingTools.join('、')}</small>
          )}
          {message && <div className={messageError ? 'pluginOperationMessage error' : 'pluginOperationMessage'}>{message}</div>}
          <div className="workflowExportFooter">
            <button type="button" className="primary" disabled={selectedWorkflows.length === 0 || exporting} onClick={exportWorkflows}>{exporting ? '正在导出...' : `导出 ${selectedWorkflows.length} 个工作流`}</button>
            <small>依赖插件会以 ZIP 形式放在导出包的 dependencies/ 目录中，需要单独上传安装。</small>
          </div>
        </div>
      </div>
    </div>
  )
}

function PlatformSettingsModal({onClose, onSaved}) {
  const [dirty, setDirty] = useState(false)
  const modalRef = useRef(null)

  function requestClose() {
    if (dirty && !window.confirm('平台设置还有未保存的修改，确认关闭并丢弃？')) return
    onClose()
  }

  useModalEscape(requestClose, modalRef)

  return (
    <div className="modalBackdrop" onMouseDown={event => { if (event.target === event.currentTarget) requestClose() }}>
      <div className="modal platformSettingsModal" ref={modalRef} role="dialog" aria-modal="true" aria-labelledby="platform-settings-title">
        <div className="modalHeader">
          <div>
            <h3 id="platform-settings-title">平台设置</h3>
            <p>配置平台服务、插件目录、路径和界面偏好。</p>
          </div>
          <button className="modalClose" type="button" onClick={requestClose} aria-label="关闭">×</button>
        </div>
        <GlobalConfigPanel modalMode onBack={requestClose} onDirtyChange={setDirty} onSaved={onSaved} />
      </div>
    </div>
  )
}

function GlobalEnvModal({onClose, onSaved}) {
  const [dirty, setDirty] = useState(false)
  const modalRef = useRef(null)

  function requestClose() {
    if (dirty && !window.confirm('全局变量还有未保存的修改，确认关闭并丢弃？')) return
    onClose()
  }

  useModalEscape(requestClose, modalRef)

  return (
    <div className="modalBackdrop" onMouseDown={event => { if (event.target === event.currentTarget) requestClose() }}>
      <div className="modal platformSettingsModal globalEnvModal" ref={modalRef} role="dialog" aria-modal="true" aria-labelledby="global-env-title">
        <div className="modalHeader">
          <div>
            <h3 id="global-env-title">全局变量</h3>
            <p>配置工具和插件脚本运行时可引用的环境变量，保存后立即重新加载。</p>
          </div>
          <button className="modalClose" type="button" onClick={requestClose} aria-label="关闭">×</button>
        </div>
        <GlobalEnvConfigPanel modalMode onBack={requestClose} onDirtyChange={setDirty} onSaved={onSaved} />
      </div>
    </div>
  )
}

const SETTINGS_SECTIONS = [
  {id: 'app', label: '应用信息', hint: '名称、描述与版本'},
  {id: 'server', label: '服务与端口', hint: '监听地址与端口'},
  {id: 'plugins', label: '插件与命令', hint: '目录、严格模式、命令白名单'},
  {id: 'market', label: '插件市场', hint: '来源、账号与接入状态'},
  {id: 'paths', label: '路径与清理', hint: '运行目录、日志与旧记录'},
  {id: 'versions', label: '版本快照', hint: '保存、载入与回滚配置'},
  {id: 'ui', label: '界面', hint: '日志终端显示'},
  {id: 'upload', label: '上传目录', hint: '上传节点可写入的绝对目录'},
  {id: 'host', label: '宿主配置白名单', hint: '允许读取的宿主目录'}
]

function GlobalConfigPanel({onBack, onSaved, onDirtyChange, modalMode = false}) {
  const [content, setContent] = useState('')
  const [path, setPath] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('正在加载配置...')
  const [saveError, setSaveError] = useState('')
  const [editMode, setEditMode] = useState('form') // 'form' | 'yaml'
  const [activeSection, setActiveSection] = useState('app')
  const [dirty, setDirty] = useState(false)
  const [cleanupKeep, setCleanupKeep] = useState(20)
  const [cleanupRunning, setCleanupRunning] = useState(false)
  const [cleanupMessage, setCleanupMessage] = useState('点击“预览清理”查看会被清理的旧运行记录。')
  const [marketSources, setMarketSources] = useState([])
  const [marketSourcesLoading, setMarketSourcesLoading] = useState(false)
  const [marketSourcesError, setMarketSourcesError] = useState('')
  const [marketSourcesOpen, setMarketSourcesOpen] = useState(false)
  const [configVersions, setConfigVersions] = useState([])
  const [versionsLoading, setVersionsLoading] = useState(false)
  const [versionsError, setVersionsError] = useState('')
  const [versionMessage, setVersionMessage] = useState('')
  const [versionName, setVersionName] = useState('')
  const [versionDescription, setVersionDescription] = useState('')
  const [versionBusy, setVersionBusy] = useState('')
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
    uiLogFontSize: 14,
    hostConfigAllowedDirs: '',
    uploadAllowedDirs: ''
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
            uiLogFontSize: normalizeLogFontSize(config.ui?.log_font_size),
            hostConfigAllowedDirs: (config.host_config_files?.allowed_dirs || []).join('\n'),
            uploadAllowedDirs: (config.uploads?.allowed_dirs || []).join('\n')
          })
        } catch (parseErr) {
          console.warn('YAML 解析失败，使用默认表单值', parseErr)
        }

        setMessage('配置已加载')
        setSaveError('')
        setDirty(false)
      } catch (err) {
        if (!cancelled) setMessage(readableAPIError(err, '加载配置失败'))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    loadConfig()
    return () => { cancelled = true }
  }, [])

  useEffect(() => {
    onDirtyChange?.(dirty)
  }, [dirty, onDirtyChange])

  useEffect(() => {
    loadMarketSources()
    loadConfigVersions()
  }, [])

  async function loadConfigVersions() {
    setVersionsLoading(true)
    setVersionsError('')
    try {
      const body = await fetchConfigVersions()
      setConfigVersions(body.data?.versions || [])
    } catch (err) {
      setVersionsError(readableAPIError(err, '读取配置版本快照失败。'))
    } finally {
      setVersionsLoading(false)
    }
  }

  async function createConfigVersion() {
    if (dirty) {
      setVersionMessage('当前有未保存的修改，请先保存设置后再创建版本快照。')
      return
    }
    const name = versionName.trim()
    if (!/^[A-Za-z0-9_-]+$/.test(name)) {
      setVersionMessage('版本名称只能包含字母、数字、连字符和下划线。')
      return
    }
    setVersionBusy('create')
    setVersionMessage('正在把当前已保存的配置写入版本快照...')
    try {
      await saveConfigVersion({name, description: versionDescription.trim()})
      setVersionName('')
      setVersionDescription('')
      setVersionMessage(`版本 ${name} 已保存。快照基于磁盘上的配置，创建前请先保存当前修改。`)
      await loadConfigVersions()
    } catch (err) {
      setVersionMessage(readableAPIError(err, '保存版本快照失败。'))
    } finally {
      setVersionBusy('')
    }
  }

  async function loadConfigVersionIntoEditor(version) {
    if (dirty && !window.confirm('当前配置有未保存的修改，载入版本会覆盖编辑器内容，确认继续？')) return
    setVersionBusy(version.name)
    setVersionMessage(`正在载入版本 ${version.name}...`)
    try {
      const body = await fetchConfigVersion(version.name)
      const data = body.data || {}
      if (!data.exists) {
        setVersionMessage(`版本 ${version.name} 的快照文件不存在。`)
        return
      }
      setContent(data.content || '')
      setEditMode('yaml')
      setDirty(true)
      setVersionMessage(`版本 ${version.name} 已载入编辑器，点击“保存设置”后才会生效。`)
    } catch (err) {
      setVersionMessage(readableAPIError(err, '载入版本快照失败。'))
    } finally {
      setVersionBusy('')
    }
  }

  async function markDefaultConfigVersion(version) {
    setVersionBusy(version.name)
    setVersionMessage(`正在把 ${version.name} 设为默认版本...`)
    try {
      await setDefaultConfigVersion(version.name)
      setVersionMessage(`版本 ${version.name} 已标记为默认版本。该标记不会自动覆盖当前配置。`)
      await loadConfigVersions()
    } catch (err) {
      setVersionMessage(readableAPIError(err, '设置默认版本失败。'))
    } finally {
      setVersionBusy('')
    }
  }

  async function removeConfigVersion(version) {
    if (!window.confirm(`确认删除配置版本快照 ${version.name}？`)) return
    setVersionBusy(version.name)
    setVersionMessage(`正在删除版本 ${version.name}...`)
    try {
      await deleteConfigVersion(version.name)
      setVersionMessage(`版本 ${version.name} 已删除。`)
      await loadConfigVersions()
    } catch (err) {
      setVersionMessage(readableAPIError(err, '删除版本快照失败。'))
    } finally {
      setVersionBusy('')
    }
  }

  async function loadMarketSources() {
    setMarketSourcesLoading(true)
    setMarketSourcesError('')
    try {
      const body = await fetchMarketSources()
      setMarketSources(body.data?.sources || [])
    } catch (err) {
      setMarketSourcesError(readableAPIError(err, '读取市场来源失败。'))
    } finally {
      setMarketSourcesLoading(false)
    }
  }

  function updateFormField(field, value) {
    setDirty(true)
    setSaveError('')
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
      ui: {
        ...(base.ui || {}),
        log_font_size: normalizeLogFontSize(formData.uiLogFontSize)
      },
      host_config_files: {
        ...(base.host_config_files || {}),
        allowed_dirs: formLines(formData.hostConfigAllowedDirs)
      },
      uploads: {
        ...(base.uploads || {}),
        allowed_dirs: formLines(formData.uploadAllowedDirs)
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
        uiLogFontSize: normalizeLogFontSize(config.ui?.log_font_size),
        hostConfigAllowedDirs: (config.host_config_files?.allowed_dirs || []).join('\n'),
        uploadAllowedDirs: (config.uploads?.allowed_dirs || []).join('\n')
      })
      setEditMode('form')
    } catch (err) {
      setMessage('YAML 格式错误，无法切换到表单模式')
    }
  }

  async function saveConfig() {
    setSaving(true)
    setMessage('正在保存...')
    setSaveError('')
    try {
      let yamlToSave = content
      if (editMode === 'form') {
        // 表单模式：把表单字段合并回当前 YAML，保留 plugins.disabled 等未在表单中编辑的配置
        yamlToSave = yaml.dump(buildConfigFromForm(content), {indent: 2, lineWidth: -1})
      }
      const body = await putJSON('/api/config/global', {content: yamlToSave})
      setMessage('设置已保存')
      setDirty(false)
      await onSaved(body)
    } catch (err) {
      const detail = readableAPIError(err, '保存失败')
      setSaveError(detail)
      setMessage(detail)
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

  const dirtyNotice = dirty && !saving && !loading
  // 保存失败时必须优先展示错误，否则会被“有未保存的修改”覆盖，看起来像点击没反应。
  const statusText = loading ? '正在加载配置...' : saveError ? saveError : dirtyNotice ? '有未保存的修改，保存后生效' : message
  const statusClass = saveError
    ? 'settingsStatus error'
    : dirtyNotice
      ? 'settingsStatus dirty'
      : message.startsWith('设置已保存') ? 'settingsStatus success' : 'settingsStatus'

  const body = (
    <div className={editMode === 'form' ? 'settingsLayout' : 'settingsLayout settingsLayoutSolo'}>
      {editMode === 'form' && (
        <nav className="settingsNav" aria-label="设置分区">
          {SETTINGS_SECTIONS.map(section => (
            <button
              key={section.id}
              type="button"
              className={activeSection === section.id ? 'settingsNavItem active' : 'settingsNavItem'}
              aria-current={activeSection === section.id ? 'true' : undefined}
              onClick={() => setActiveSection(section.id)}
            >
              <strong>{section.label}</strong>
              <small>{section.hint}</small>
            </button>
          ))}
        </nav>
      )}
      <div className="settingsContent" key={editMode === 'form' ? activeSection : 'yaml'}>
        {editMode === 'form' ? renderSection() : (
          <section className="settingsCard settingsCardYaml">
            <div className="settingsCardHead">
              <h4>高级编辑</h4>
              <p>直接编辑配置文件原文，适合批量调整表单未覆盖的字段；保存后立即生效。</p>
            </div>
            <label className="settingsField settingsWide">
              <span>配置内容（YAML 格式）</span>
              <textarea className="settingsYamlEditor" value={content} disabled={loading || saving} placeholder="app:&#10;  name: 运维控制台&#10;server:&#10;  port: 8080" onChange={event => { setDirty(true); setSaveError(''); setContent(event.target.value) }} />
            </label>
          </section>
        )}
      </div>
    </div>
  )

  const panel = (
    <>
      <div className="settingsMeta">
        <span className="settingsMetaLabel">配置文件</span>
        <code>{path || 'configs/ops.yaml'}</code>
        <span className="settingsMetaHint">{editMode === 'form' ? '表单模式' : 'YAML 模式'}</span>
      </div>
      {body}
      <div className="settingsFooter">
        <span className={statusClass} role="status">{statusText}</span>
        <div className="settingsFooterActions">
          <button className="secondary" disabled={saving} onClick={onBack}>{modalMode ? '关闭' : '返回'}</button>
          {editMode === 'form' ? (
            <button className="secondary" disabled={loading || saving} onClick={switchToYaml}>高级编辑（YAML）</button>
          ) : (
            <button className="secondary" disabled={loading || saving} onClick={switchToForm}>返回表单</button>
          )}
          <button className="primary" disabled={loading || saving} onClick={saveConfig}>{saving ? '正在保存...' : '保存设置'}</button>
        </div>
      </div>
    </>
  )

  if (!modalMode) {
    return (
      <div className="grid">
        <section className="card pluginConfigCard platformSettingsPanel">
          <div className="cardHeader">
            <div>
              <h3>框架设置</h3>
              <p>定义应用名称、插件路径、服务器端口等框架设置</p>
            </div>
          </div>
          {panel}
        </section>
      </div>
    )
  }
  return <div className="platformSettingsPanel">{panel}</div>

  function renderSection() {
    switch (activeSection) {
      case 'server':
        return (
          <section className="settingsCard">
            <div className="settingsCardHead">
              <h4>服务与端口</h4>
              <p>控制 Web 控制台与 API 的监听地址、端口。修改后需重启服务生效。</p>
            </div>
            <div className="settingsGrid">
              <label className="checkboxLabel settingsWide settingsToggle">
                <input type="checkbox" checked={formData.serverEnabled} disabled={loading || saving} onChange={e => updateFormField('serverEnabled', e.target.checked)} />
                <span>启用内置服务器</span>
              </label>
              <label className="settingsField">
                <span>监听地址</span>
                <input type="text" value={formData.serverHost} disabled={loading || saving} placeholder="127.0.0.1" onChange={e => updateFormField('serverHost', e.target.value)} />
              </label>
              <label className="settingsField">
                <span>监听端口</span>
                <input type="number" value={formData.serverPort} disabled={loading || saving} placeholder="8080" onChange={e => updateFormField('serverPort', parseInt(e.target.value) || 8080)} />
              </label>
              <p className="settingsHint settingsWide">127.0.0.1 仅本机可访问；需要局域网访问时改为 0.0.0.0，并注意暴露风险。</p>
            </div>
          </section>
        )
      case 'plugins':
        return (
          <section className="settingsCard">
            <div className="settingsCardHead">
              <h4>插件与命令</h4>
              <p>插件扫描目录、加载策略以及允许通过 PATH 调用的外部命令。</p>
            </div>
            <div className="settingsGrid">
              <label className="settingsField settingsWide">
                <span>插件目录（逗号分隔）</span>
                <input type="text" value={formData.pluginsPaths} disabled={loading || saving} placeholder="plugins" onChange={e => updateFormField('pluginsPaths', e.target.value)} />
              </label>
              <label className="checkboxLabel settingsWide settingsToggle">
                <input type="checkbox" checked={formData.pluginsStrict} disabled={loading || saving} onChange={e => updateFormField('pluginsStrict', e.target.checked)} />
                <span>严格模式：插件加载失败时中断启动</span>
              </label>
              <label className="settingsField settingsWide">
                <span>允许 PATH 命令（每行一个命令）</span>
                <textarea className="settingsTextarea" value={formData.pluginsAllowedCommands} disabled={loading || saving} placeholder="java&#10;ansible-playbook" onChange={e => updateFormField('pluginsAllowedCommands', e.target.value)} />
              </label>
              <p className="settingsHint settingsWide">仅裸 command 命中此白名单时，才会通过运行环境 PATH 执行；带路径 command 仍必须位于插件目录内，参数请继续通过 args 数组声明。</p>
            </div>
          </section>
        )
      case 'market':
        return (
          <section className="settingsCard">
            <div className="settingsCardHead">
              <h4>插件市场</h4>
              <p>平台从以下来源拉取插件清单与安装包，来源的增删改在独立面板中维护。</p>
            </div>
            <div className="platformMarketSources">
              {marketSourcesLoading ? (
                <div className="empty small">正在读取市场来源...</div>
              ) : marketSourcesError ? (
                <div className="marketSourceError">{marketSourcesError}</div>
              ) : marketSources.length === 0 ? (
                <div className="empty small">还没有配置市场来源。</div>
              ) : (
                <div className="platformMarketSourceList">
                  {marketSources.map(source => (
                    <div className="platformMarketSourceItem" key={source.id}>
                      <div className="platformMarketSourceHead">
                        <strong>{source.name || source.id}</strong>
                        <span className={`marketSourceType ${source.type}`}>{source.type === 'nexus' ? 'Nexus' : source.type === 'remote' ? '远程市场' : '本地市场'}</span>
                        {!source.enabled && <span className="marketBadge">已禁用</span>}
                        <span className={`marketBadge ${source.ready ? 'installed' : ''}`}>{source.ready ? '可用' : '待完善'}</span>
                      </div>
                      <small>
                        {source.url}
                        {source.type === 'nexus' && source.repository ? ` · ${source.repository}` : ''}
                        {' · '}
                        {source.auth_type === 'basic' ? `账号：${source.username || '-'}` : '匿名访问'}
                      </small>
                    </div>
                  ))}
                </div>
              )}
              <div className="settingsCardActions">
                <button className="secondary" type="button" onClick={() => setMarketSourcesOpen(true)}>管理市场来源</button>
              </div>
            </div>
            {marketSourcesOpen && (
              <MarketSourcesModal onClose={() => setMarketSourcesOpen(false)} onChanged={loadMarketSources} />
            )}
          </section>
        )
      case 'paths':
        return (
          <section className="settingsCard">
            <div className="settingsCardHead">
              <h4>路径与清理</h4>
              <p>运行记录、日志的落盘目录，以及旧记录的批量清理策略。</p>
            </div>
            <div className="settingsGrid">
              <label className="settingsField">
                <span>运行目录</span>
                <input type="text" value={formData.pathsRuns} disabled={loading || saving} placeholder="runs" onChange={e => updateFormField('pathsRuns', e.target.value)} />
              </label>
              <label className="settingsField">
                <span>日志目录</span>
                <input type="text" value={formData.pathsLogs} disabled={loading || saving} placeholder="runs/logs" onChange={e => updateFormField('pathsLogs', e.target.value)} />
              </label>
              <div className="runCleanupBox settingsWide">
                <div>
                  <strong>运行日志清理</strong>
                  <span>清理工具和工作流的旧运行记录，支持包导出目录会保留。</span>
                </div>
                <label className="inlineNumberField">
                  <span>保留最近</span>
                  <input type="number" min="0" value={cleanupKeep} disabled={loading || saving || cleanupRunning} onChange={e => setCleanupKeep(e.target.value)} />
                  <span>条</span>
                </label>
                <div className="settingsCardActions">
                  <button className="secondary" type="button" disabled={loading || saving || cleanupRunning} onClick={() => cleanupRuns(true)}>预览清理</button>
                  <button className="secondary danger" type="button" disabled={loading || saving || cleanupRunning} onClick={() => cleanupRuns(false)}>清理旧记录</button>
                </div>
                <pre className="runCleanupResult" aria-live="polite">{cleanupMessage}</pre>
              </div>
            </div>
          </section>
        )
      case 'versions':
        return (
          <section className="settingsCard">
            <div className="settingsCardHead">
              <h4>配置版本快照</h4>
              <p>把磁盘上已保存的配置存为命名快照；载入快照只会写入当前编辑器，点击“保存设置”后才会真正回滚。</p>
            </div>
            <div className="configVersionCreate">
              <label className="settingsField">
                <span>版本名称</span>
                <input value={versionName} disabled={Boolean(versionBusy)} placeholder="例如 before-upgrade" onChange={event => setVersionName(event.target.value)} />
              </label>
              <label className="settingsField">
                <span>说明（可选）</span>
                <input value={versionDescription} disabled={Boolean(versionBusy)} placeholder="记录本次配置变更原因" onChange={event => setVersionDescription(event.target.value)} />
              </label>
              <button className="secondary" type="button" disabled={Boolean(versionBusy) || !versionName.trim()} onClick={createConfigVersion}>
                {versionBusy === 'create' ? '保存中...' : '保存当前配置为版本'}
              </button>
            </div>
            <small className="settingsHint">版本快照保存的是已经写入磁盘的配置；当前草稿有未保存修改时，请先保存再创建快照。</small>
            {versionsError && <div className="marketSourceError">{versionsError}</div>}
            {versionMessage && <small className="nexusMessage">{versionMessage}</small>}
            <div className="configVersionList">
              {versionsLoading ? (
                <div className="empty small">正在读取版本快照...</div>
              ) : configVersions.length === 0 ? (
                <div className="empty small">还没有版本快照。填写版本名称后保存当前配置，即可创建第一个快照。</div>
              ) : configVersions.map(version => (
                <article className="configVersionItem" key={version.name}>
                  <div className="configVersionMain">
                    <div className="configVersionTitle">
                      <strong>{version.name}</strong>
                      {version.default && <span className="marketBadge installed">默认版本</span>}
                    </div>
                    <small>{version.description || '无说明'}</small>
                    <small>{version.created_at ? new Date(version.created_at).toLocaleString() : '创建时间未知'}{version.created_by ? ` · ${version.created_by}` : ''}</small>
                  </div>
                  <div className="configVersionActions">
                    <button className="secondary" type="button" disabled={Boolean(versionBusy)} onClick={() => loadConfigVersionIntoEditor(version)}>载入编辑器</button>
                    <button className="secondary" type="button" disabled={Boolean(versionBusy) || version.default} onClick={() => markDefaultConfigVersion(version)}>{version.default ? '默认版本' : '设为默认'}</button>
                    <button className="secondary danger" type="button" disabled={Boolean(versionBusy)} onClick={() => removeConfigVersion(version)}>删除</button>
                  </div>
                </article>
              ))}
            </div>
            <div className="settingsCardActions">
              <button className="secondary" type="button" disabled={versionsLoading} onClick={loadConfigVersions}>{versionsLoading ? '刷新中...' : '刷新版本列表'}</button>
            </div>
          </section>
        )
      case 'ui':
        return (
          <section className="settingsCard">
            <div className="settingsCardHead">
              <h4>界面</h4>
              <p>控制台展示偏好，仅影响当前浏览器的呈现效果。</p>
            </div>
            <div className="settingsGrid">
              <label className="settingsField">
                <span>日志字体大小（px）</span>
                <input type="number" min="12" max="20" value={formData.uiLogFontSize} disabled={loading || saving} placeholder="14" onChange={e => updateFormField('uiLogFontSize', e.target.value)} />
              </label>
              <p className="settingsHint settingsWide">日志终端字体大小，支持 12-20；非法值保存时会回退为 14。</p>
            </div>
          </section>
        )
      case 'upload':
        return (
          <section className="settingsCard">
            <div className="settingsCardHead">
              <h4>上传目录白名单</h4>
              <p>上传节点的“目标目录”可以填写这里的绝对目录，文件会直接写入该目录；未列出的绝对路径会被拒绝。</p>
            </div>
            <div className="settingsGrid">
              <label className="settingsField settingsWide">
                <span>允许的绝对目录（每行一个）</span>
                <textarea className="settingsTextarea settingsTextareaHost" value={formData.uploadAllowedDirs} disabled={loading || saving} placeholder="/opt/app/release&#10;D:\deploy\packages" onChange={e => updateFormField('uploadAllowedDirs', e.target.value)} />
              </label>
              <p className="settingsHint settingsWide">留空时上传节点只允许写入 runs/uploads 下的相对子目录；白名单内的子目录同样允许。</p>
            </div>
          </section>
        )
      case 'host':
        return (
          <section className="settingsCard">
            <div className="settingsCardHead">
              <h4>宿主配置文件白名单</h4>
              <p>限定宿主 mapping 与工作流配置文件可读取的范围，未命中白名单的目录会被拒绝访问。</p>
            </div>
            <div className="settingsGrid">
              <label className="settingsField settingsWide">
                <span>允许目录（每行一个绝对目录）</span>
                <textarea className="settingsTextarea settingsTextareaHost" value={formData.hostConfigAllowedDirs} disabled={loading || saving} placeholder="/etc/myapp&#10;C:\ProgramData\Vendor" onChange={e => updateFormField('hostConfigAllowedDirs', e.target.value)} />
              </label>
              <p className="settingsHint settingsWide">仅允许目录白名单；宿主 mapping 的 config_dir 必须落在这些目录内。</p>
            </div>
          </section>
        )
      default:
        return (
          <section className="settingsCard">
            <div className="settingsCardHead">
              <h4>应用信息</h4>
              <p>控制台名称、描述与版本，会显示在侧栏和页面标题中。</p>
            </div>
            <div className="settingsGrid">
              <label className="settingsField">
                <span>应用名称</span>
                <input type="text" value={formData.appName} disabled={loading || saving} placeholder="运维控制台" onChange={e => updateFormField('appName', e.target.value)} />
              </label>
              <label className="settingsField">
                <span>应用版本</span>
                <input type="text" value={formData.appVersion} disabled={loading || saving} placeholder="0.1.0" onChange={e => updateFormField('appVersion', e.target.value)} />
              </label>
              <label className="settingsField settingsWide">
                <span>应用描述</span>
                <input type="text" value={formData.appDescription} disabled={loading || saving} placeholder="运维工具执行控制台" onChange={e => updateFormField('appDescription', e.target.value)} />
              </label>
            </div>
          </section>
        )
    }
  }
}

function GlobalEnvConfigPanel({onBack, onSaved, onDirtyChange, modalMode = false}) {
  const [content, setContent] = useState('')
  const [savedContent, setSavedContent] = useState('')
  const [path, setPath] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('正在读取全局环境配置...')
  const [messageTone, setMessageTone] = useState('loading')
  const dirty = content !== savedContent
  const envStats = useMemo(() => summarizeEnvContent(content), [content])

  useEffect(() => {
    onDirtyChange?.(dirty)
  }, [dirty, onDirtyChange])

  useEffect(() => {
    let cancelled = false
    async function loadConfig() {
      setLoading(true)
      setMessage('正在读取全局环境配置...')
      setMessageTone('loading')
      try {
        const body = await fetchJSON('/api/config/global-env')
        if (cancelled) return
        const loadedContent = body.data?.content || ''
        setContent(loadedContent)
        setSavedContent(loadedContent)
        setPath(body.data?.path || 'configs/global-env.conf')
        setMessage('已加载全局环境配置文件。')
        setMessageTone('ok')
      } catch (err) {
        if (!cancelled) {
          setMessage(readableAPIError(err, '读取全局环境配置失败。'))
          setMessageTone('error')
        }
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
    setMessageTone('loading')
    try {
      const body = await putJSON('/api/config/global-env', {content})
      setSavedContent(content)
      setMessage('全局环境配置已保存并重新加载运行时配置。')
      setMessageTone('ok')
      await onSaved(body)
    } catch (err) {
      setMessage(readableAPIError(err, '保存全局环境配置失败。'))
      setMessageTone('error')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="globalEnvPanel">
      <section className="globalEnvBody">
        <div className="globalEnvMeta">
          <code title={path || 'configs/global-env.conf'}>{path || 'configs/global-env.conf'}</code>
          <span className="globalEnvCount">{envStats.variables} 个变量 · {envStats.comments} 行注释</span>
          <span className={dirty ? 'globalEnvState dirty' : 'globalEnvState'}>{dirty ? '有未保存修改' : '已保存'}</span>
        </div>
        <div className="globalEnvEditor">
          <div className="globalEnvEditorHead">
            <strong>.env 文件内容</strong>
            <span>每行一个 KEY=value，# 开头为注释；保存后变量会注入工具进程环境。</span>
          </div>
          <div className="globalEnvEditorSurface">
            <textarea
              className="globalEnvTextarea"
              value={content}
              disabled={loading || saving}
              spellCheck={false}
              wrap="off"
              aria-label="全局变量 .env 文件内容"
              placeholder="# 数据库配置&#10;DB_HOST=localhost&#10;DB_PORT=5432&#10;&#10;# API 配置&#10;API_BASE_URL=https://api.example.com&#10;&#10;# 环境标识&#10;ENVIRONMENT=dev"
              onChange={event => setContent(event.target.value)}
            />
          </div>
        </div>
      </section>
      <div className="globalEnvFooter">
        <div className={`globalEnvStatus ${messageTone}`} role="status">{message}</div>
        <div className="globalEnvActions">
          <button type="button" className="secondary" disabled={saving} onClick={onBack}>{modalMode ? '关闭' : '返回列表'}</button>
          <button type="button" className="primary" disabled={loading || saving || !dirty} onClick={saveConfig}>{saving ? '正在保存...' : '保存全局变量'}</button>
        </div>
      </div>
    </div>
  )
}

function summarizeEnvContent(content) {
  let variables = 0
  let comments = 0
  String(content || '').split(/\r?\n/).forEach(line => {
    const trimmed = line.trim()
    if (!trimmed) return
    if (trimmed.startsWith('#')) {
      comments += 1
      return
    }
    if (/^[A-Za-z_][A-Za-z0-9_]*\s*=/.test(trimmed)) variables += 1
  })
  return {variables, comments}
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

function normalizeWorkflowConfigFile(file) {
  if (typeof file === 'string') {
    return {
      id: file,
      label: file,
      path: file,
      displayPath: file,
      display_root: '',
      scope: 'plugin',
      access: 'read_write',
      create: true,
      exists: true,
      readable: true,
      writable: true,
      reason: ''
    }
  }
  const path = file.path || file.id || ''
  const configDir = file.config_dir || file.configDir || ''
  const displayRoot = file.display_root || file.displayRoot || configDir || ''
  return {
    ...file,
    id: file.id || path,
    label: file.label || file.name || file.id || path,
    path,
    config_dir: configDir || file.config_dir,
    display_root: displayRoot,
    displayPath: file.display_path || file.displayPath || [displayRoot, path].filter(Boolean).join('/') || path || file.id || '',
    scope: file.scope || 'plugin',
    access: file.access || 'read_write'
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

function workflowConfigFileSaveBlockReason(file) {
  return pluginConfigFileSaveBlockReason(file)
}

function canDeleteConfigFile(file) {
  return Boolean(file?.id)
}

function ConfigReplaceToolbar({content, setContent, disabled, onMessage}) {
  const [findText, setFindText] = useState('')
  const [replacement, setReplacement] = useState('')
  const matchCount = useMemo(() => countTextMatches(content, findText), [content, findText])

  function applyReplace(mode) {
    if (!findText) {
      onMessage?.('请输入要查找的文本。')
      return
    }
    const result = replaceTextDraft(content, findText, replacement, mode)
    if (result.replacements === 0) {
      onMessage?.('未找到匹配文本。')
      return
    }
    setContent(result.content)
    onMessage?.(`已替换 ${result.replacements} 处文本，保存后才会写入文件。`)
  }

  return (
    <section className="configReplaceToolbar" aria-label="配置文本替换">
      <div className="configReplaceHeader">
        <strong>文本替换</strong>
        <span>{findText ? `匹配 ${matchCount} 处` : '输入查找文本后可替换当前草稿'}</span>
      </div>
      <div className="configReplaceFields">
        <label>
          <span>查找文本</span>
          <input value={findText} disabled={disabled} placeholder="例如 127.0.0.1" onChange={event => setFindText(event.target.value)} />
        </label>
        <label>
          <span>替换为</span>
          <input value={replacement} disabled={disabled} placeholder="例如 172.16.51.200" onChange={event => setReplacement(event.target.value)} />
        </label>
      </div>
      <div className="configReplaceActions">
        <button className="secondary" type="button" disabled={disabled || !findText || matchCount === 0} onClick={() => applyReplace('first')}>替换第一个</button>
        <button className="secondary" type="button" disabled={disabled || !findText || matchCount === 0} onClick={() => applyReplace('all')}>全部替换</button>
      </div>
    </section>
  )
}

function PluginConfigFileEditor({pluginID, file, onBack}) {
  const [content, setContent] = useState('')
  const [originalContent, setOriginalContent] = useState('')
  const [showDiff, setShowDiff] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [message, setMessage] = useState('正在读取配置文件...')
  const fileID = file?.id || ''
  const saveBlockReason = pluginConfigFileSaveBlockReason(file)
  const canSave = !saveBlockReason
  const canDelete = canDeleteConfigFile(file)
  const diff = useMemo(() => buildTextDiff(originalContent, content), [originalContent, content])

  useEffect(() => {
    setShowDiff(false)
    loadFile()
  }, [pluginID, fileID])

  async function loadFile() {
    setLoading(true)
    setMessage('正在读取配置文件...')
    try {
      const body = await fetchJSON(`/api/plugins/${encodeURIComponent(pluginID)}/files/${encodeURIComponent(fileID)}`)
      const loadedContent = body.data?.content || ''
      setContent(loadedContent)
      setOriginalContent(loadedContent)
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
      setOriginalContent(content)
      setShowDiff(false)
      setMessage('配置文件已保存。')
    } catch (err) {
      setMessage(readableAPIError(err, '保存配置文件失败。'))
    } finally {
      setSaving(false)
    }
  }

  function requestBack() {
    if (content !== originalContent && !window.confirm('当前配置文件还有未保存的修改，确认返回列表并丢弃？')) return
    onBack()
  }

  async function deleteFile() {
    if (!canDelete) return
    const displayName = file?.displayPath || file?.path || fileID
    if (file?.exists !== false && !window.confirm(`将删除配置文件 ${displayName}，是否继续？`)) return
    setDeleting(true)
    setMessage('正在删除配置文件...')
    try {
      await deleteJSON(`/api/plugins/${encodeURIComponent(pluginID)}/files/${encodeURIComponent(fileID)}`)
      setMessage('配置文件已删除。')
      onBack()
    } catch (err) {
      setMessage(readableAPIError(err, '删除配置文件失败。'))
    } finally {
      setDeleting(false)
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
      <ConfigReplaceToolbar
        content={content}
        setContent={setContent}
        disabled={loading || saving || !canSave}
        onMessage={setMessage}
      />
      <label>
        <span>文件内容</span>
        <textarea
          value={content}
          disabled={loading || saving || !canSave}
          placeholder="输入配置文件内容..."
          onChange={event => setContent(event.target.value)}
        />
      </label>
      <div className="configDiffToolbar">
        <div>
          <strong>{diff.changed ? '当前草稿有未保存差异' : '当前草稿与已加载内容一致'}</strong>
          <span>+{diff.stats.added} / -{diff.stats.removed} / 未变 {diff.stats.unchanged}</span>
        </div>
        <button className="secondary" type="button" disabled={loading} onClick={() => setShowDiff(current => !current)}>
          {showDiff ? '隐藏差异' : '查看差异'}
        </button>
      </div>
      {showDiff && <ConfigDiffView diff={diff} />}
      <div className="buttonRow">
        <button className="primary" disabled={loading || saving || deleting || !canSave} onClick={saveFile}>保存配置文件</button>
        <button className="secondary danger" disabled={loading || saving || deleting || !canDelete} onClick={deleteFile}>删除配置文件</button>
        <button className="secondary" disabled={saving || deleting} onClick={requestBack}>返回列表</button>
      </div>
      <pre className="modalResult">{message}</pre>
    </div>
  )
}

function WorkflowConfigFileEditor({workflowID, file, onBack}) {
  const [content, setContent] = useState('')
  const [originalContent, setOriginalContent] = useState('')
  const [showDiff, setShowDiff] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [message, setMessage] = useState('正在读取工作流配置文件...')
  const fileID = file?.id || ''
  const saveBlockReason = workflowConfigFileSaveBlockReason(file)
  const canSave = !saveBlockReason
  const canDelete = canDeleteConfigFile(file)
  const externalBinding = file?.managed === false
  const diff = useMemo(() => buildTextDiff(originalContent, content), [originalContent, content])

  useEffect(() => {
    setShowDiff(false)
    loadFile()
  }, [workflowID, fileID])

  async function loadFile() {
    setLoading(true)
    setMessage('正在读取工作流配置文件...')
    try {
      const body = await fetchJSON(`/api/workflows/${encodeURIComponent(workflowID)}/files/${encodeURIComponent(fileID)}`)
      const loadedContent = body.data?.content || ''
      setContent(loadedContent)
      setOriginalContent(loadedContent)
      setMessage('已加载工作流配置文件。')
    } catch (err) {
      setMessage(readableAPIError(err, '读取工作流配置文件失败。'))
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
    setMessage('正在保存工作流配置文件...')
    try {
      await putJSON(`/api/workflows/${encodeURIComponent(workflowID)}/files/${encodeURIComponent(fileID)}`, {content})
      setOriginalContent(content)
      setShowDiff(false)
      setMessage('工作流配置文件已保存。')
    } catch (err) {
      setMessage(readableAPIError(err, '保存工作流配置文件失败。'))
    } finally {
      setSaving(false)
    }
  }

  function requestBack() {
    if (content !== originalContent && !window.confirm('当前工作流配置文件还有未保存的修改，确认返回列表并丢弃？')) return
    onBack()
  }

  async function deleteFile() {
    if (!canDelete) return
    const displayName = file?.displayPath || file?.path || fileID
    const confirmText = externalBinding
      ? `将解除工作流对 ${displayName} 的绑定，原文件会保留，是否继续？`
      : `将删除工作流配置文件 ${displayName}，是否继续？`
    if (file?.exists !== false && !window.confirm(confirmText)) return
    setDeleting(true)
    setMessage(externalBinding ? '正在解除绑定...' : '正在删除工作流配置文件...')
    try {
      const body = await deleteJSON(`/api/workflows/${encodeURIComponent(workflowID)}/files/${encodeURIComponent(fileID)}`)
      setMessage(body?.data?.message || (externalBinding ? '已解除绑定，原文件保留。' : '工作流配置文件已删除。'))
      onBack()
    } catch (err) {
      setMessage(readableAPIError(err, externalBinding ? '解除绑定失败。' : '删除工作流配置文件失败。'))
    } finally {
      setDeleting(false)
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
        {externalBinding && <small className="configFileSaveHint">该文件位于工作流配置目录之外，删除时仅解除绑定，不会删除原文件。</small>}
        {saveBlockReason && <small className="configFileSaveHint">{saveBlockReason}</small>}
      </div>
      <ConfigReplaceToolbar
        content={content}
        setContent={setContent}
        disabled={loading || saving || deleting || !canSave}
        onMessage={setMessage}
      />
      <label>
        <span>文件内容</span>
        <textarea
          value={content}
          disabled={loading || saving || deleting || !canSave}
          placeholder="输入配置文件内容..."
          onChange={event => setContent(event.target.value)}
        />
      </label>
      <div className="configDiffToolbar">
        <div>
          <strong>{diff.changed ? '当前草稿有未保存差异' : '当前草稿与已加载内容一致'}</strong>
          <span>+{diff.stats.added} / -{diff.stats.removed} / 未变 {diff.stats.unchanged}</span>
        </div>
        <button className="secondary" type="button" disabled={loading} onClick={() => setShowDiff(current => !current)}>
          {showDiff ? '隐藏差异' : '查看差异'}
        </button>
      </div>
      {showDiff && <ConfigDiffView diff={diff} />}
      <div className="buttonRow">
        <button className="primary" disabled={loading || saving || deleting || !canSave} onClick={saveFile}>保存工作流配置文件</button>
        <button className="secondary danger" disabled={loading || saving || deleting || !canDelete} onClick={deleteFile}>{externalBinding ? '解除绑定' : '删除配置文件'}</button>
        <button className="secondary" disabled={saving || deleting} onClick={requestBack}>返回列表</button>
      </div>
      <pre className="modalResult">{message}</pre>
    </div>
  )
}

function buildWorkflowConfigFileTree(files, workflowID) {
  const root = {key: 'root', type: 'root', children: []}
  const roots = new Map()

  files.forEach(file => {
    const rootLabel = file.display_root || file.config_dir || '运行根目录'
    const rootKey = `workflow:${rootLabel}`
    let rootNode = roots.get(rootKey)
    if (!rootNode) {
      rootNode = {key: rootKey, type: 'dir', name: rootLabel, path: rootLabel, scope: file.scope || 'plugin', children: [], childMap: new Map()}
      roots.set(rootKey, rootNode)
      root.children.push(rootNode)
    }

    const rawPath = file.path || file.displayPath || file.id || file.label || ''
    const segments = splitConfigPath(rawPath)
    insertConfigFileTreeNode(rootNode, segments.length > 0 ? segments : [file.label || file.id || '未命名配置文件'], file)
  })

  sortConfigFileTree(root)
  return root
}

function WorkflowConfigFilesPanel({workflowID, onBack, onSaved}) {
  const [files, setFiles] = useState([])
  const [loading, setLoading] = useState(true)
  const [message, setMessage] = useState('正在加载工作流配置文件列表...')
  const [editingFile, setEditingFile] = useState(null)
  const [bindMode, setBindMode] = useState('')
  const [bindFile, setBindFile] = useState(null)
  const [bindPath, setBindPath] = useState('')
  const [bindLabel, setBindLabel] = useState('')
  const [bindSubdir, setBindSubdir] = useState('')
  const [binding, setBinding] = useState(false)
  const bindFileInputRef = useRef(null)
  const fileTree = useMemo(() => buildWorkflowConfigFileTree(files, workflowID), [files, workflowID])

  useEffect(() => {
    loadFiles()
  }, [workflowID])

  async function loadFiles({silent = false} = {}) {
    if (!silent) {
      setLoading(true)
      setMessage('正在加载工作流配置文件列表...')
    }
    try {
      const body = await fetchJSON(`/api/workflows/${encodeURIComponent(workflowID)}/files`)
      const fileList = (body.data?.files || []).map(normalizeWorkflowConfigFile)
      setFiles(fileList)
      if (!silent) {
        setMessage(fileList.length > 0 ? `已加载 ${fileList.length} 个工作流配置文件。` : '当前没有工作流配置文件。')
      }
    } catch (err) {
      if (!silent) {
        setMessage(readableAPIError(err, '加载工作流配置文件列表失败。'))
      }
    } finally {
      if (!silent) setLoading(false)
    }
  }

  function openBind(mode) {
    setBindMode(mode)
    setBindFile(null)
    setBindPath('')
    setBindLabel('')
    setBindSubdir('')
    if (bindFileInputRef.current) bindFileInputRef.current.value = ''
    setMessage(mode === 'upload'
      ? '选择本地文件，上传到该工作流配置目录并写入工作流声明。'
      : '填写运行根目录内的相对路径，或白名单目录内的宿主绝对路径。')
  }

  function cancelBind() {
    setBindMode('')
    setBindFile(null)
    setBindPath('')
    setBindLabel('')
    setBindSubdir('')
    if (bindFileInputRef.current) bindFileInputRef.current.value = ''
    setMessage('已取消绑定。')
  }

  async function submitBind(event) {
    event.preventDefault()
    if (binding) return
    const path = bindPath.trim()
    if (bindMode === 'upload' && !bindFile) {
      setMessage('请选择要上传的配置文件。')
      return
    }
    if (bindMode === 'existing' && !path) {
      setMessage('请填写要绑定的配置文件路径。')
      return
    }
    setBinding(true)
    setMessage(bindMode === 'upload' ? '正在上传并绑定配置文件...' : '正在绑定配置文件...')
    try {
      const body = bindMode === 'upload'
        ? await uploadWorkflowConfigFile(workflowID, bindFile, bindSubdir.trim())
        : await bindWorkflowConfigPath(workflowID, {path, label: bindLabel.trim()})
      const bound = normalizeWorkflowConfigFile(body.data?.file || {})
      setBindMode('')
      setBindFile(null)
      setBindPath('')
      setBindLabel('')
      setBindSubdir('')
      if (bindFileInputRef.current) bindFileInputRef.current.value = ''
      setMessage(`已绑定工作流配置文件：${bound.displayPath || bound.path || bound.id}`)
      await loadFiles({silent: true})
      setEditingFile(bound)
    } catch (err) {
      setMessage(readableAPIError(err, bindMode === 'upload' ? '上传并绑定失败。' : '绑定配置文件失败。'))
    } finally {
      setBinding(false)
    }
  }

  if (editingFile) {
    return (
      <WorkflowConfigFileEditor
        workflowID={workflowID}
        file={editingFile}
        onBack={() => {
          setEditingFile(null)
          loadFiles()
          onSaved?.()
        }}
      />
    )
  }

  return (
    <div className="pluginConfigFilesPanel">
      <div className="empty small">工作流配置文件来自真实挂载路径；可在此直接查看和编辑。目录默认展开，可按目录折叠查找。</div>
      <div className="workflowConfigBindBar">
        <button className="secondary" type="button" disabled={binding} onClick={() => openBind('upload')}>上传并绑定</button>
        <button className="secondary" type="button" disabled={binding} onClick={() => openBind('existing')}>绑定已有路径</button>
      </div>
      {bindMode && (
        <form className="workflowConfigBindForm" onSubmit={submitBind}>
          <strong>{bindMode === 'upload' ? '上传并绑定' : '绑定已有路径'}</strong>
          {bindMode === 'upload' ? (
            <>
              <label>
                <span>配置文件</span>
                <input ref={bindFileInputRef} type="file" disabled={binding} onChange={event => setBindFile(event.target.files?.[0] || null)} />
              </label>
              <label>
                <span>保存子目录（可选）</span>
                <input value={bindSubdir} disabled={binding} placeholder="例如 deployment；留空保存到工作流配置目录根目录" onChange={event => setBindSubdir(event.target.value)} />
              </label>
            </>
          ) : (
            <label>
              <span>文件路径</span>
              <input value={bindPath} disabled={binding} placeholder="例如 runs/uploads/app.conf 或 /etc/nginx/nginx.conf" onChange={event => setBindPath(event.target.value)} />
            </label>
          )}
          <label>
            <span>显示名称（可选）</span>
            <input value={bindLabel} disabled={binding} placeholder="留空使用文件名" onChange={event => setBindLabel(event.target.value)} />
          </label>
          <div className="workflowConfigBindHint">
            上传会写入该工作流自己的配置目录并登记到工作流 YAML；绑定已有路径不会移动原文件。宿主绝对路径必须位于 configs/ops.yaml 的 host_config_files.allowed_dirs 白名单内（可在“平台设置 → 宿主配置文件白名单”维护），解除绑定时不会删除原文件。文件大小上限 1MB。
          </div>
          <div className="buttonRow">
            <button className="primary" type="submit" disabled={binding}>{binding ? '处理中...' : bindMode === 'upload' ? '上传并绑定' : '绑定路径'}</button>
            <button className="secondary" type="button" disabled={binding} onClick={cancelBind}>取消</button>
          </div>
        </form>
      )}
      <div className="configFilesTree" role="tree" aria-label="工作流配置文件目录树">
        {fileTree.children.map(node => (
          <ConfigFileTreeNode key={node.key} node={node} depth={0} onSelectFile={setEditingFile} />
        ))}
        {files.length === 0 && !loading && (
          <div className="empty">当前没有工作流配置文件。可点击“上传并绑定”保存新文件，或“绑定已有路径”引用运行根目录内或白名单目录内的现有文件。</div>
        )}
      </div>
      <div className="buttonRow">
        <button className="secondary" onClick={onBack}>返回列表</button>
      </div>
      <pre className="modalResult">{message}</pre>
    </div>
  )
}

function WorkflowConfigPanel({workflow, onBack, onSaved, onOpenPluginConfig}) {
  const workflowID = workflow?.id || ''
  const referencedPlugins = workflow?.referencedPlugins || []

  return (
    <div className="grid">
      <section className="card pluginConfigCard">
        <div className="cardHeader">
          <div>
            <h3>工作流配置文件</h3>
            <p>{workflow.name || workflowID}：管理该工作流挂载的真实配置文件</p>
          </div>
        </div>
        <WorkflowConfigFilesPanel workflowID={workflowID} onBack={onBack} onSaved={onSaved} />
      </section>
      {referencedPlugins.length > 0 && (
        <section className="card pluginConfigCard">
          <div className="cardHeader">
            <div>
              <h3>引用的插件配置</h3>
              <p>工作流节点用到了 {referencedPlugins.length} 个带配置文件的插件。文件仍由插件持有，点击可跳转到插件配置修改。</p>
            </div>
          </div>
          <div className="list">
            {referencedPlugins.map(plugin => (
              <button
                key={plugin.id}
                className={plugin.disabled ? 'listItem pluginDisabled' : 'listItem'}
                onClick={() => onOpenPluginConfig?.({id: plugin.id, name: plugin.name || plugin.id})}
                title={`打开插件 ${plugin.name || plugin.id} 的配置文件`}
              >
                <div>
                  <strong>{plugin.name || plugin.id}</strong>
                  <small className="configTypeLabel">插件</small>
                  {plugin.disabled && <small>状态：插件已禁用</small>}
                  {plugin.tools?.length > 0 && <small>引用工具：{plugin.tools.join('、')}</small>}
                  <div className="configFilePaths">
                    {plugin.files.map((file, index) => (
                      <small key={index} className="configFilePath">
                        {file.label && <span className="fileLabel">{file.label}：</span>}
                        {file.path}
                      </small>
                    ))}
                  </div>
                </div>
              </button>
            ))}
          </div>
        </section>
      )}
    </div>
  )
}

function ConfigDiffView({diff}) {
  if (!diff.changed) {
    return <div className="configDiffEmpty">没有检测到内容差异。</div>
  }
  return (
    <div className="configDiffView" aria-label="配置文件差异">
      <div className="configDiffHeader">
        <span>原行</span>
        <span>新行</span>
        <span>内容</span>
      </div>
      <div className="configDiffRows">
        {diff.rows.map((row, index) => (
          <div key={`${row.type}-${index}-${row.oldLine}-${row.newLine}`} className={`configDiffRow ${row.type}`}>
            <span>{row.oldLine}</span>
            <span>{row.newLine}</span>
            <code>{diffRowPrefix(row.type)}{row.text || ' '}</code>
          </div>
        ))}
      </div>
    </div>
  )
}

function diffRowPrefix(type) {
  if (type === 'add') return '+ '
  if (type === 'remove') return '- '
  return '  '
}

function ResultView({result, expanded = false}) {
  const className = expanded ? 'resultView resultViewExpanded' : 'resultView'
  const progress = result?.uploadProgress
  if (typeof result === 'string') {
    return <div className={className}><pre>{result}</pre></div>
  }
  if (result?.batch?.runs?.length) {
    return <div className={className}>{progress && <UploadProgressPanel progress={progress} />}<BatchRunResult batch={result.batch} /></div>
  }
  if (result?.run?.id && !result?.detail?.data) {
    return <div className={className}>{progress && <UploadProgressPanel progress={progress} />}<RunDetail detail={{record: result.run, logs: {}}} run={result.run} /></div>
  }
  if (result?.detail?.data) {
    return <div className={className}>{progress && <UploadProgressPanel progress={progress} />}<RunDetail detail={result.detail.data} run={result.run} /></div>
  }
  if (result?.response) {
    return <div className={className}>{progress && <UploadProgressPanel progress={progress} />}<MessageWithDetails message={result.message} details={result.response} /></div>
  }
  return <div className={className}>{progress && <UploadProgressPanel progress={progress} />}<pre>{result?.message || '暂无结果'}</pre></div>
}

function BatchRunResult({batch}) {
  const runs = batch?.runs || []
  const [activeRunID, setActiveRunID] = useState('')
  const [activeDetail, setActiveDetail] = useState(null)

  useEffect(() => {
    if (runs.length === 0) return
    if (!runs.some(run => run.id === activeRunID)) setActiveRunID(runs[0].id)
  }, [runs, activeRunID])

  const activeRun = runs.find(run => run.id === activeRunID) || runs[0] || {}

  useEffect(() => {
    if (!activeRunID) {
      setActiveDetail(null)
      return undefined
    }
    let cancelled = false
    fetchRunDetail(activeRunID)
      .then(body => { if (!cancelled) setActiveDetail(body?.data || body || null) })
      .catch(() => { if (!cancelled) setActiveDetail(null) })
    return () => { cancelled = true }
  }, [activeRunID, activeRun.status])

  const counts = {}
  for (const run of runs) {
    const status = run.status || 'unknown'
    counts[status] = (counts[status] || 0) + 1
  }

  return (
    <div className="batchRun">
      <div className="runSummary">
        <span>批次：{batch.batchID || '-'}</span>
        <span>共 {runs.length} 个目标</span>
        {Object.entries(counts).map(([status, count]) => (
          <span key={status} className={`runStatusText ${status}`}>{status} {count}</span>
        ))}
      </div>
      <div className="batchHostTabs">
        {runs.map(run => (
          <button
            key={run.id}
            type="button"
            className={['batchHostTab', run.status || '', run.id === activeRun.id ? 'active' : ''].filter(Boolean).join(' ')}
            onClick={() => setActiveRunID(run.id)}
          >
            <strong>{run.host_name || run.host_id || '未知操作机'}</strong>
            <span>{run.status || '-'}</span>
          </button>
        ))}
      </div>
      <div className="runSummary">
        <span className={`runStatusText ${activeRun.status || ''}`}>{activeRun.status || '-'}</span>
        {(activeRun.host_name || activeRun.host_id) && <span>目标：{activeRun.host_name || activeRun.host_id}</span>}
        <span>运行 ID：{activeRun.id || '-'}</span>
        {activeRun.id && <a href={`/api/runs/${encodeURIComponent(activeRun.id)}/support.zip`}>导出支持包</a>}
      </div>
      {activeRun.error && <div className="batchRunError">{activeRun.error}</div>}
      {activeRun.id ? (
        <React.Suspense fallback={<div className="empty small">正在加载实时日志终端...</div>}>
          <LiveRunTerminal key={activeRun.id} runID={activeRun.id} running={activeRun.status === 'running'} initialItems={activeDetail?.record?.id === activeRun.id ? activeDetail.logs?.items || [] : []} />
        </React.Suspense>
      ) : null}
    </div>
  )
}

function batchResultMessage(runs) {
  const succeeded = runs.filter(run => run.status === 'succeeded').length
  const failed = runs.length - succeeded
  if (failed === 0) return `全部 ${succeeded} 个目标执行成功。`
  return `${succeeded} 个成功，${failed} 个未成功。`
}

function UploadProgressPanel({progress}) {
  return (
    <div className={`uploadProgressPanel uploadProgress${capitalizeProgressStatus(progress.status)}`}>
      <div className="uploadProgressMeta">
        <strong>{progress.nodeID}</strong>
        <span>{progress.label || '上传中'}</span>
        <span>{formatBytes(progress.uploaded)} / {formatBytes(progress.total)} · {progress.percent}%</span>
      </div>
      <div className="uploadProgressTrack" role="progressbar" aria-valuenow={progress.percent} aria-valuemin="0" aria-valuemax="100">
        <span style={{width: `${progress.percent}%`}} />
      </div>
    </div>
  )
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
  const runID = record.id || run?.id
  const status = record.status || run?.status
  const items = logs.items || []
  // 终端仍然是运行记录页的默认视图（原始流、支持 SSE 实时追加）；
  // 「列表」用的是右栏节点详情同一套 RunLogView：按节点分组、级别过滤、搜索、导出。
  const [logView, setLogView] = useState('terminal')
  const runningStepID = (record.steps || []).find(step => step.status === 'running')?.id || ''
  const stepNames = useMemo(() => {
    const names = {}
    items.forEach(item => { if (item?.id) names[item.id] = item.title || item.id })
    return names
  }, [items])
  return (
    <div className="runDetail">
      <div className="runSummary">
        <span className={`runStatusText ${status || ''}`}>{status || '-'}</span>
        <span>目标：{record.target || '-'}</span>
        {(record.host_name || record.host_id) && <span>操作机：{record.host_name || record.host_id}</span>}
        <span>运行 ID：{runID || '-'}</span>
        {runID && <a href={`/api/runs/${encodeURIComponent(runID)}/support.zip`}>导出支持包</a>}
        {runID && status && (
          <div className="segmentedControl" role="group" aria-label="日志视图">
            <button type="button" className={logView === 'list' ? 'active' : ''} onClick={() => setLogView('list')}>列表</button>
            <button type="button" className={logView === 'terminal' ? 'active' : ''} onClick={() => setLogView('terminal')}>终端</button>
          </div>
        )}
      </div>
      {runID ? (
        logView === 'list' && items.length > 0 ? (
          <React.Suspense fallback={<div className="empty small">正在加载日志列表...</div>}>
            <RunLogView items={items} nodeNames={stepNames} runningOwnerID={runningStepID} runID={runID} scopeToggle={false} />
          </React.Suspense>
        ) : (
          <React.Suspense fallback={<div className="empty small">正在加载实时日志终端...</div>}>
            <LiveRunTerminal runID={runID} running={status === 'running'} initialItems={items} />
          </React.Suspense>
        )
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

function RunPanel({entries, totalEntries, selected, params, setParams, selectEntry, runSelected, searchText, setSearchText, activeTag, setActiveTag, availableTags, hosts = [], runHosts = [], setRunHosts = () => {}, runLocal = true, setRunLocal = () => {}, isRunning = false, onCancelRun, isCancelling = false}) {
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [targetPickerOpen, setTargetPickerOpen] = useState(false)
  const targetPickerRef = useRef(null)
  const runCardRef = useRef(null)
  const paginationEnabled = true
  const totalPages = paginationEnabled ? Math.max(1, Math.ceil(entries.length / pageSize)) : 1
  const safePage = Math.min(currentPage, totalPages)
  const visibleEntries = paginationEnabled ? entries.slice((safePage - 1) * pageSize, safePage * pageSize) : entries
  const selectedHosts = hosts.filter(host => runHosts.includes(host.id))
  const missingHosts = runHosts.filter(id => !hosts.some(host => host.id === id))
  const enabledHosts = hosts.filter(host => host.enabled)
  const selectedNames = [...selectedHosts.map(host => host.name || host.id), ...missingHosts]
  const targetNames = [...(runLocal ? ['本机'] : []), ...selectedNames]
  const targetSummary = runLocal && runHosts.length === 0
    ? '本机执行'
    : runLocal && runHosts.length === 1
      ? `本机 + 操作机：${selectedNames[0] || runHosts[0]}`
      : runLocal
        ? `本机 + ${runHosts.length} 台操作机并行执行`
        : runHosts.length === 0
          ? '未选择执行目标'
          : runHosts.length === 1
            ? `操作机：${selectedNames[0] || runHosts[0]}`
            : `${runHosts.length} 台操作机并行执行`
  const targetDescription = targetNames.length === 0
    ? '请至少选择一个执行目标'
    : runLocal && runHosts.length === 0
      ? '在控制台所在机器运行'
      : targetNames.join('、')
  const missingTargets = !runLocal && runHosts.length === 0

  useEffect(() => {
    setTargetPickerOpen(false)
    if (!selected?.id) return undefined
    const frame = window.requestAnimationFrame(() => {
      const element = runCardRef.current
      if (!element) return
      if (element.getBoundingClientRect().top > window.innerHeight * 0.55) {
        element.scrollIntoView({behavior: 'smooth', block: 'start'})
      }
    })
    return () => window.cancelAnimationFrame(frame)
  }, [selected?.id])

  useEffect(() => {
    if (!targetPickerOpen) return
    function closeOnOutside(event) {
      if (!targetPickerRef.current?.contains(event.target)) setTargetPickerOpen(false)
    }
    function closeOnEscape(event) {
      if (event.key === 'Escape') setTargetPickerOpen(false)
    }
    document.addEventListener('mousedown', closeOnOutside)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('mousedown', closeOnOutside)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [targetPickerOpen])

  function toggleHost(hostID) {
    setRunHosts(previous => previous.includes(hostID) ? previous.filter(id => id !== hostID) : [...previous, hostID])
  }

  function selectAllHosts() {
    setRunHosts(enabledHosts.map(host => host.id))
  }

  function selectLocalOnly() {
    setRunLocal(true)
    setRunHosts([])
  }

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

      <section className="card runCard" ref={runCardRef}>
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
              <div className="runTargetField">
                <span>执行目标</span>
                <div className="runTargetPicker" ref={targetPickerRef}>
                  <button
                    type="button"
                    className={targetPickerOpen ? 'runTargetSelect open' : 'runTargetSelect'}
                    disabled={isRunning}
                    aria-haspopup="dialog"
                    aria-expanded={targetPickerOpen}
                    onClick={() => setTargetPickerOpen(open => !open)}
                  >
                    <span className="runTargetSelectText">
                      <strong>{targetSummary}</strong>
                      <small title={targetDescription}>{targetDescription}</small>
                    </span>
                    <span className="runTargetSelectCaret" aria-hidden="true">{targetPickerOpen ? '▴' : '▾'}</span>
                  </button>
                  {targetPickerOpen && (
                    <div className="runTargetPopover" role="dialog" aria-label="选择执行目标">
                      <label className={runLocal ? 'runTargetChoice active' : 'runTargetChoice'}>
                        <input type="checkbox" checked={runLocal} onChange={() => setRunLocal(value => !value)} />
                        <strong>本机执行</strong>
                        <small>在控制台所在机器运行，可与操作机同时选中。</small>
                      </label>
                      <div className="runTargetPopoverHeader">
                        <span>操作机</span>
                        <span>{hosts.length} 台</span>
                      </div>
                      {hosts.length === 0 ? (
                        <div className="empty small">还没有配置操作机，可在顶部「操作机」中添加。</div>
                      ) : (
                        <div className="runTargetOptionList">
                          {hosts.map(host => {
                            const checked = runHosts.includes(host.id)
                            return (
                              <label key={host.id} className={['runTargetChoice', checked ? 'active' : '', host.enabled ? '' : 'disabled'].filter(Boolean).join(' ')}>
                                <input type="checkbox" checked={checked} disabled={!host.enabled} onChange={() => toggleHost(host.id)} />
                                <strong>{host.name || host.id}</strong>
                                <small>{host.user}@{host.address}:{host.port} · {sshHostStatusLabel(host.status)}{host.enabled ? '' : ' · 已停用'}</small>
                              </label>
                            )
                          })}
                        </div>
                      )}
                      <div className="runTargetPopoverFooter">
                        <button type="button" className="secondary" disabled={enabledHosts.length === 0} onClick={selectAllHosts}>全选操作机</button>
                        <button type="button" className="secondary" disabled={runLocal && runHosts.length === 0} onClick={selectLocalOnly}>仅本机</button>
                        <button type="button" className="primary" onClick={() => setTargetPickerOpen(false)}>完成</button>
                      </div>
                    </div>
                  )}
                </div>
                {runLocal && runHosts.length > 0 && <small>已选择本机和 {runHosts.length} 台操作机，将并行执行。</small>}
                {!runLocal && runHosts.length > 0 && <small>已选择 {runHosts.length} 台操作机，将并行执行。</small>}
                {missingTargets && <small className="targetWarning">请至少选择本机或一台操作机。</small>}
                {missingHosts.length > 0 && <small className="targetWarning">已选择的操作机不存在：{missingHosts.join('、')}，请重新选择。</small>}
                {selectedHosts.some(host => host.status !== 'online') && <small className="targetWarning">部分操作机不在线，执行时将实时建立连接。</small>}
                {selectedHosts.length === 1 && selectedHosts[0].description && <small>{selectedHosts[0].description}</small>}
              </div>
              {(selected.parameters || []).map(param => (
                <label key={param.name} className={param.required && !hasConfiguredParamValue(params[param.name]) ? 'requiredField' : ''}>
                  <span>{param.description || param.name}{param.required ? ' *' : ''}</span>
                  {param.type === 'bool' ? (
                    <input type="checkbox" checked={parseBoolParamValue(params[param.name])} onChange={event => setParams({...params, [param.name]: event.target.checked})} />
                  ) : param.type === 'file' ? (
                    <FileUploadInput value={params[param.name] ?? ''} onChange={value => setParams({...params, [param.name]: value})} />
                  ) : Array.isArray(param.options) && param.options.length > 0 ? (
                    <select aria-required={param.required || undefined} value={params[param.name] ?? ''} onChange={event => setParams({...params, [param.name]: event.target.value})}>
                      <option value="">{param.default ? `默认：${param.default}` : '请选择...'}</option>
                      {param.options.map(option => <option key={option} value={option}>{option || '（空）'}</option>)}
                    </select>
                  ) : (
                    <input aria-required={param.required || undefined} value={params[param.name] ?? ''} placeholder={param.name} onChange={event => setParams({...params, [param.name]: event.target.value})} />
                  )}
                </label>
              ))}
              {(selected.parameters || []).length === 0 && <div className="empty small">无需参数。</div>}
              <div className="buttonRow">
                <button className="primary" disabled={isRunning || missingTargets || (selected.parameters || []).some(param => param.required && !hasConfiguredParamValue(params[param.name]))} onClick={runSelected}>{isRunning ? '执行中...' : '开始执行'}</button>
                {isRunning && onCancelRun && (
                  <button className="secondary danger" type="button" disabled={isCancelling} onClick={onCancelRun}>{isCancelling ? '取消中...' : '取消执行'}</button>
                )}
              </div>
            </div>
          </>
        )}
      </section>
    </div>
  )
}

const rootElement = document.getElementById('root')
if (rootElement) {
  bootstrapAuthTokenFromURL().finally(() => {
    // 独立草稿窗口只挂载画布，不初始化主控制台的其它面板。
    createRoot(rootElement).render(isDraftWindowLocation(window.location.search)
      ? <React.Suspense fallback={<div className="empty">正在打开独立草稿窗口...</div>}><DraftWindowShell /></React.Suspense>
      : <App />)
  })
}

function parseBoolParamValue(value) {
  return value === true || value === 'true' || value === '1' || value === 'yes' || value === 'on'
}

function hasConfiguredParamValue(value) {
  if (value === undefined || value === null) return false
  if (Array.isArray(value)) return value.length > 0
  if (typeof value === 'object') return Object.keys(value).length > 0
  return String(value).trim() !== ''
}

function workflowCategoryDisplayName(category, labels) {
  if (!category) return '未分类'
  if (category === 'global') return '全局'
  return labels.get(category) || category
}
