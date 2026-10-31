import React from 'react'
import {Handle, Position} from '@xyflow/react'
import {conditionBranchRows, conditionOperators, defaultCondition, normalizeArchiveConfig, normalizeApprovalConfig, normalizeChecksumConfig, normalizeExtractConfig, normalizeHttpRequestConfig, normalizeNotifyConfig, normalizeScriptConfig, normalizeWaitConfig} from './model.js'
import {controlNodeCategory, controlNodeHelp, controlNodeMeta} from './catalog.js'

function FlowchartShape({kind, marker}) {
  return (
    <span className={`flowchartShape ${kind}`} aria-hidden="true">
      <span>{marker}</span>
    </span>
  )
}

// 节点的形状、符号、分类全部来自 catalog.js，组件不再按 type 写分支。
function controlShape(type) {
  const meta = controlNodeMeta(type)
  const category = controlNodeCategory(type)
  return {
    meta,
    category,
    shape: meta?.shape || category.shape || 'gateway',
    symbol: meta?.symbol || '·'
  }
}

function controlShapeMarker(type) {
  return controlShape(type).symbol
}

function controlShapeLabel(type) {
  const meta = controlNodeMeta(type)
  if (!meta) return '流程节点'
  return `${meta.secondary} ${meta.title}`
}

function CategoryChip({category, title}) {
  return (
    <span className={`nodeCategoryChip ${category.id}`} title={title || category.description}>{category.label}</span>
  )
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

function toolParamStatus(tool, params = {}) {
  if (!tool) {
    return {
      missingRequired: 0,
      toolMissing: true,
      label: '工具未注册，无法检查参数',
      title: '参数状态：工具未注册，无法检查参数定义'
    }
  }
  const parameters = tool.parameters || []
  const total = parameters.length
  const configured = parameters.filter(param => hasConfiguredParamValue(params?.[param.name])).length
  const missingRequired = parameters.filter(param => param.required && !hasConfiguredParamValue(params?.[param.name])).length
  const label = total === 0
    ? '无需参数'
    : missingRequired > 0
      ? `参数 ${configured}/${total} · 缺 ${missingRequired} 必填`
      : `参数 ${configured}/${total} · 必填已就绪`
  return {
    missingRequired,
    toolMissing: false,
    label,
    title: `参数状态：共 ${total} 个，已配置 ${configured} 个，缺失必填 ${missingRequired} 个`
  }
}

function hasConfiguredParamValue(value) {
  if (value === undefined || value === null) return false
  if (Array.isArray(value)) return value.length > 0
  if (typeof value === 'object') return Object.keys(value).length > 0
  return String(value).trim() !== ''
}

function ToolNode({id, data, selected}) {
  const runTitle = formatNodeRunTitle(data.run)
  const paramStatus = data.paramStatus || toolParamStatus(null, data.params || {})
  const sourceLabel = data.toolMeta?.sourceLabel || '插件工具'
  const nodeTitle = [data.name || id, sourceLabel, data.tool, data.hostLabel, paramStatus.title, runTitle].filter(Boolean).join('\n')
  return (
    <div className={nodeRunClass('toolNode', selected, data.run)} title={nodeTitle}>
      <Handle type="target" position={Position.Left} />
      <RunStatusBadge run={data.run} />
      <NodeDeleteButton id={id} onRemove={data.onRemove} />
      <NodeRerunButton id={id} onRerun={data.onRerun} disabled={data.rerunDisabled} />
      <div className="nodeTopBar">
        <span className="nodeHeaderShape process" aria-hidden="true" />
        <strong>{data.name || id}</strong>
      </div>
      <div className="nodeContent">
        <span className="toolTypeLine">{sourceLabel}</span>
        {data.hostLabel && <span className="nodeHostLine">{data.hostLabel}</span>}
        <span className={paramStatus.missingRequired > 0 || paramStatus.toolMissing ? 'toolParamStatus warning' : 'toolParamStatus'}>{paramStatus.label}</span>
        {data.tool && <span className="nodeHoverMeta">{data.tool}</span>}
      </div>
      <Handle type="source" position={Position.Right} />
    </div>
  )
}


function ConditionNode({id, data, selected}) {
  const condition = data.condition || defaultCondition()
  const status = conditionNodeStatus(condition)
  const branches = conditionBranchRows(condition)
  const category = controlNodeCategory('condition')
  const runTitle = formatNodeRunTitle(data.run)
  const nodeTitle = [data.name || id, category.label, conditionSummary(condition), conditionCaseSummary(condition), status.label, runTitle].filter(Boolean).join('\n')
  return (
    <div className={nodeRunClass(`conditionNode cat-${category.id}`, selected, data.run)} title={nodeTitle}>
      <Handle type="target" position={Position.Left} />
      <RunStatusBadge run={data.run} />
      <NodeDeleteButton id={id} onRemove={data.onRemove} />
      <NodeRerunButton id={id} onRerun={data.onRerun} disabled={data.rerunDisabled} />
      <div className="conditionNodeMain">
        <div className="nodeTopBar conditionTopBar">
          <span className="conditionDiamond" aria-hidden="true"><span>?</span></span>
          <strong>{data.name || id}</strong>
        </div>
        <div className="conditionInfoCard">
          <CategoryChip category={category} title="条件节点：按输入值选择后续分支" />
          <div className="conditionInputSummary">{conditionSummary(condition)}</div>
          <small>{conditionCaseSummary(condition)}</small>
          <small className={status.ready ? 'conditionState ready' : 'conditionState warning'}>{status.label}</small>
        </div>
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
  const type = data.controlType
  const {category, shape, symbol} = controlShape(type)
  const typeLabel = controlShapeLabel(type)
  const loop = data.loop || {}
  const loopSummary = loop.tool ? `工具 ${loop.tool} × ${loop.max_iterations || 0}` : '未配置循环工具'
  const runLoopSummary = data.run?.loopIterations ? `实际迭代 ${data.run.loopIterations} 次` : ''
  const uploadSummary = data.upload?.target_dir ? `目标 uploads/${data.upload.target_dir}` : '目标默认 uploads 目录'
  const extractConfig = normalizeExtractConfig(data.extract_config || {})
  const extractSummary = extractConfig.source_type === 'directory'
    ? extractConfig.files.length > 0
      ? `${extractConfig.source_dir || '未填源目录'} → ${extractConfig.files.length} 个文件`
      : '未配置目录文件'
    : extractConfig.target_path
      ? `${extractConfig.file_name || '未填源文件'} → ${extractConfig.target_path}`
      : '未配置提取映射'
  const httpRequest = normalizeHttpRequestConfig(data.http || {})
  const httpURL = httpRequest.url.length > 46 ? `${httpRequest.url.slice(0, 46)}…` : httpRequest.url
  const httpSummary = httpURL
    ? `${httpRequest.method} ${httpURL}`
    : '未配置请求 URL'
  const script = normalizeScriptConfig(data.script || {})
  const scriptFirstLine = script.script.split('\n').map(line => line.trim()).find(Boolean) || ''
  const scriptSummary = scriptFirstLine
    ? `${scriptFirstLine.length > 40 ? `${scriptFirstLine.slice(0, 40)}…` : scriptFirstLine}${script.timeout ? ` · 超时 ${script.timeout}` : ''}`
    : '未填写脚本内容'
  const checksum = normalizeChecksumConfig(data.checksum || {})
  const checksumSummary = checksum.path
    ? `${checksum.algorithm.toUpperCase()} · ${checksum.path}${checksum.expected ? ' · 需匹配' : ''}`
    : '未配置文件路径'
  const archive = normalizeArchiveConfig(data.archive || {})
  const archiveSummary = archive.path && archive.target_dir
    ? `${archive.path} → ${archive.target_dir}`
    : '未配置压缩包或目标目录'
  const wait = normalizeWaitConfig(data.wait || {})
  const waitSummary = wait.mode === 'poll'
    ? `轮询 ${wait.poll_url || wait.poll_file || '未配置目标'}${wait.max_attempts ? ` · ${wait.max_attempts} 次` : ''}${wait.max_wait ? ` · 最长 ${wait.max_wait}` : ''}`
    : `固定等待 ${wait.seconds || 0}s`
  const notify = normalizeNotifyConfig(data.notify || {})
  const notifyTarget = notify.channel === 'email'
    ? `${notify.smtp_host || '未配置 SMTP'} → ${notify.to || '未配置收件人'}`
    : notify.webhook || '未配置 Webhook'
  const notifySummary = `${{webhook: 'Webhook', wecom: '企业微信', email: '邮件'}[notify.channel] || '通知'} · ${notifyTarget}`
  const approval = normalizeApprovalConfig(data.approval || {})
  const approvalSummary = `${approval.message || '未填写审批提示'}${approval.timeout ? ` · 超时 ${approval.timeout}` : ''}`
  // 审批节点等待时显示的是「等待审批」而不是泛泛的「等待中」，让画布上能一眼看出是卡在人身上。
  const approvalWaitingLabel = type === 'approval' && data.run?.type === 'approval' ? '等待审批' : ''
  const helpText = type === 'loop'
    ? [loopSummary, runLoopSummary].filter(Boolean).join('；')
    : type === 'upload'
      ? uploadSummary
      : type === 'extract_config'
        ? extractSummary
        : type === 'http_request'
          ? httpSummary
          : type === 'shell_script'
            ? scriptSummary
            : type === 'file_checksum'
              ? checksumSummary
              : type === 'archive'
                ? archiveSummary
                : type === 'wait'
                  ? waitSummary
                  : type === 'notify'
                    ? notifySummary
                    : type === 'approval'
                      ? approvalSummary
                      : controlNodeHelp(type)
  const runTitle = formatNodeRunTitle(data.run)
  const nodeTitle = [data.name || id, category.label, typeLabel, helpText, data.hostLabel, runTitle].filter(Boolean).join('\n')
  return (
    <div className={nodeRunClass(`controlNode cat-${category.id} ${type || ''}`, selected, data.run)} title={nodeTitle}>
      <Handle type="target" position={Position.Left} />
      <RunStatusBadge run={data.run} waitingLabel={approvalWaitingLabel} />
      <NodeDeleteButton id={id} onRemove={data.onRemove} />
      <NodeRerunButton id={id} onRerun={data.onRerun} disabled={data.rerunDisabled} />
      <div className="nodeTopBar controlTopBar">
        <FlowchartShape kind={`${shape} ${category.id}`} marker={symbol} />
        <strong>{data.name || id}</strong>
      </div>
      <div className="controlNodeText">
        <CategoryChip category={category} />
        <small>{typeLabel} · {helpText}</small>
        {data.hostLabel && <small className="nodeHostLine">{data.hostLabel}</small>}
      </div>
      <Handle type="source" position={Position.Right} />
    </div>
  )
}

function NodeDeleteButton({id, onRemove}) {
  if (!onRemove) return null
  return (
    <button
      className="nodeDelete nodrag nopan"
      type="button"
      onPointerDown={event => event.stopPropagation()}
      onMouseDown={event => event.stopPropagation()}
      onClick={event => { event.stopPropagation(); onRemove(id) }}
      title="删除节点"
    >
      ×
    </button>
  )
}

function NodeRerunButton({id, onRerun, disabled}) {
  if (!onRerun) return null
  return (
    <button
      type="button"
      className="nodeRerun nodrag nopan"
      disabled={disabled}
      title={disabled ? '当前运行中，不能重跑节点' : '从该节点开始重跑'}
      onPointerDown={event => event.stopPropagation()}
      onMouseDown={event => event.stopPropagation()}
      onClick={event => {
        event.stopPropagation()
        if (!disabled) onRerun(id)
      }}
    >
      重跑
    </button>
  )
}

function nodeRunClass(baseClass, selected, run) {
  const runStatus = run?.status
  return [baseClass, selected ? 'selected' : '', runStatus && runStatus !== 'idle' ? `run${capitalizeStatus(runStatus)}` : ''].filter(Boolean).join(' ')
}

function RunStatusBadge({run, waitingLabel = ''}) {
  if (!run?.status || run.status === 'idle') return null
  const label = waitingLabel && normalizeRunStatus(run.status) === 'waiting' ? waitingLabel : runStatusLabel(run.status)
  return <span className={`runStatusBadge run${capitalizeStatus(run.status)}`} title={formatNodeRunTitle(run)}>{label}</span>
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

export function runStatusLabel(status) {
  const normalized = normalizeRunStatus(status)
  if (normalized === 'succeeded') return '成功'
  if (normalized === 'failed') return '失败'
  if (normalized === 'skipped') return '跳过'
  if (normalized === 'running') return '运行中'
  if (normalized === 'waiting') return '等待中'
  if (normalized === 'pending') return '待处理'
  if (normalized === 'rolled_back') return '已回滚'
  if (normalized === 'cancelled') return '已取消'
  return '未运行'
}

export function normalizeRunStatus(status) {
  const value = String(status || '').toLowerCase()
  if (value === 'success' || value === 'succeeded' || value === 'ok') return 'succeeded'
  if (value === 'fail' || value === 'failed' || value === 'error') return 'failed'
  if (value === 'skip' || value === 'skipped') return 'skipped'
  if (value === 'running') return 'running'
  if (value === 'cancel' || value === 'cancelled' || value === 'canceled') return 'cancelled'
  if (value === 'pending') return 'pending'
  if (value === 'rolled_back' || value === 'rolled-back' || value === 'rolledback') return 'rolled_back'
  if (value === 'waiting' || value === 'queued') return 'waiting'
  return value || 'idle'
}

function capitalizeStatus(status) {
  const normalized = normalizeRunStatus(status)
  return normalized.charAt(0).toUpperCase() + normalized.slice(1)
}

export const nodeTypes = {toolNode: ToolNode, conditionNode: ConditionNode, controlNode: ControlNode}
export {controlShapeMarker}
