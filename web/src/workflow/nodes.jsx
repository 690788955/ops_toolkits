import React from 'react'
import {Handle, Position} from '@xyflow/react'
import {conditionBranchRows, conditionOperators, defaultCondition, normalizeExtractConfig} from './model.js'

function FlowchartShape({kind, marker}) {
  return (
    <span className={`flowchartShape ${kind}`} aria-hidden="true">
      <span>{marker}</span>
    </span>
  )
}

function controlShapeKind(type) {
  if (type === 'condition') return 'decision'
  if (type === 'parallel' || type === 'join' || type === 'loop' || type === 'upload' || type === 'extract_config') return `gateway ${type}`
  return 'planned'
}

function controlShapeMarker(type) {
  if (type === 'condition') return '?'
  if (type === 'parallel') return '+'
  if (type === 'join') return '∧'
  if (type === 'loop') return '↻'
  if (type === 'upload') return '↑'
  if (type === 'extract_config') return '⇩'
  return '·'
}

function controlShapeLabel(type) {
  if (type === 'condition') return 'Decision 条件判断'
  if (type === 'parallel') return 'Gateway 并行分支'
  if (type === 'join') return 'Gateway 合流'
  if (type === 'loop') return 'Loop 固定次数循环'
  if (type === 'upload') return 'Upload 上传文件'
  if (type === 'extract_config') return 'Extract Config 提取配置'
  return '流程节点'
}

function controlNodeHelp(type) {
  if (type === 'condition') return '根据上游输出或工作流参数选择后续分支'
  if (type === 'parallel') return '将后续任务拆分为多个分支路径'
  if (type === 'join') return '等待多个上游分支完成后继续流程'
  if (type === 'loop') return '按固定次数重复执行一个内嵌选择的插件工具'
  if (type === 'upload') return '运行前上传本地文件、批量文件或目录到平台目录'
  if (type === 'extract_config') return '从上传结果提取文件到工作流配置中心'
  return '编排控制节点'
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
  const nodeTitle = [data.name || id, sourceLabel, data.tool, paramStatus.title, runTitle].filter(Boolean).join('\n')
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
        <span className={paramStatus.missingRequired > 0 || paramStatus.toolMissing ? 'toolParamStatus warning' : 'toolParamStatus'}>{paramStatus.label}</span>
        {data.tool && <span className="nodeHoverMeta">{data.tool}</span>}
      </div>
      <Handle type="source" position={Position.Right} />
      <QuickAddDownstreamButton id={id} onAddDownstream={data.onAddDownstream} />
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
      <NodeDeleteButton id={id} onRemove={data.onRemove} />
      <NodeRerunButton id={id} onRerun={data.onRerun} disabled={data.rerunDisabled} />
      <div className="conditionNodeMain">
        <div className="nodeTopBar conditionTopBar">
          <span className="conditionDiamond" aria-hidden="true"><span>?</span></span>
          <strong>{data.name || id}</strong>
        </div>
        <div className="conditionInfoCard">
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
  const helpText = data.controlType === 'loop'
    ? [loopSummary, runLoopSummary].filter(Boolean).join('；')
    : data.controlType === 'upload'
      ? uploadSummary
      : data.controlType === 'extract_config'
        ? extractSummary
      : controlNodeHelp(data.controlType)
  const runTitle = formatNodeRunTitle(data.run)
  const nodeTitle = [data.name || id, controlShapeLabel(data.controlType), helpText, runTitle].filter(Boolean).join('\n')
  return (
    <div className={nodeRunClass(`controlNode ${data.controlType || ''}`, selected, data.run)} title={nodeTitle}>
      <Handle type="target" position={Position.Left} />
      <RunStatusBadge run={data.run} />
      <NodeDeleteButton id={id} onRemove={data.onRemove} />
      <NodeRerunButton id={id} onRerun={data.onRerun} disabled={data.rerunDisabled} />
      <div className="nodeTopBar controlTopBar">
        <FlowchartShape kind={controlShapeKind(data.controlType)} marker={controlShapeMarker(data.controlType)} />
        <strong>{data.name || id}</strong>
      </div>
      <div className="controlNodeText">
        <small>{controlShapeLabel(data.controlType)} · {helpText}</small>
      </div>
      <Handle type="source" position={Position.Right} />
      <QuickAddDownstreamButton id={id} onAddDownstream={data.onAddDownstream} />
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

function QuickAddDownstreamButton({id, onAddDownstream}) {
  if (!onAddDownstream) return null
  return (
    <button
      type="button"
      className="quickNodeActions quickNodeActionsTrigger nodrag nopan"
      title="添加下游节点并自动连接"
      aria-label="添加下游节点"
      onPointerDown={event => event.stopPropagation()}
      onMouseDown={event => event.stopPropagation()}
      onClick={event => {
        event.stopPropagation()
        onAddDownstream(id, event)
      }}
    >
      +
    </button>
  )
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

export function runStatusLabel(status) {
  const normalized = normalizeRunStatus(status)
  if (normalized === 'succeeded') return '成功'
  if (normalized === 'failed') return '失败'
  if (normalized === 'skipped') return '跳过'
  if (normalized === 'running') return '运行中'
  if (normalized === 'waiting') return '等待中'
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
  if (value === 'waiting' || value === 'pending' || value === 'queued') return 'waiting'
  return value || 'idle'
}

function capitalizeStatus(status) {
  const normalized = normalizeRunStatus(status)
  return normalized.charAt(0).toUpperCase() + normalized.slice(1)
}

export const nodeTypes = {toolNode: ToolNode, conditionNode: ConditionNode, controlNode: ControlNode}
export {controlShapeMarker}
