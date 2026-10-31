import React, {useEffect, useMemo, useState} from 'react'
import {normalizeRunStatus, runStatusLabel} from '../nodes.jsx'
import RunLogView from './RunLogView.jsx'
import {baseStepID, runCountsByStatus, runDurationText, stepDurationText} from './runLog.js'

// 运行信息、操作与日志分层排列，窄栏不把按钮和长 ID 挤在同一行。
export default function RunLogPanel({
  focusNodeID = '',
  focusNodeName = '',
  node = null,
  run = null,
  runState = null,
  record = null,
  logItems = [],
  machineLabel = '',
  runNodeNames = {},
  activeRunID = '',
  isRunning = false,
  runningNodeID = '',
  rerunDisabledReason = '',
  recoveryAvailable = false,
  recoveryPanel = null,
  onRequestRecovery,
  onRerun,
  onClearFocus,
  onExpand,
  onClose,
  onClearRunOverlay,
  supportHref
}) {
  const [overviewOpen, setOverviewOpen] = useState(false)
  const [recoveryOpen, setRecoveryOpen] = useState(false)

  // 换运行记录 / 换工作流时把折叠区收起来，避免上一个运行的结论看起来像这一条的。
  useEffect(() => {
    setOverviewOpen(false)
    setRecoveryOpen(false)
  }, [activeRunID, focusNodeID])

  const counts = useMemo(() => runCountsByStatus(runState), [runState])
  const steps = useMemo(() => {
    if (!focusNodeID) return []
    return (record?.steps || []).filter(step => baseStepID(step.id) === focusNodeID)
  }, [record, focusNodeID])
  const status = normalizeRunStatus(record?.status || runState?.status)
  const focusNodeStatus = focusNodeID ? normalizeRunStatus(run?.status || steps[steps.length - 1]?.status) : ''

  function toggleRecovery() {
    if (!recoveryOpen) onRequestRecovery?.()
    setRecoveryOpen(!recoveryOpen)
  }

  return (
    <section className="workflowRunDock open" aria-label="运行详情">
      <header className="workflowRunDockHeader">
        <div className="workflowRunDockHeading">
          <div className="workflowRunDockTitle">
            <strong>运行日志</strong>
            <span className={`workflowRunStatusChip ${status}`}>{runStatusLabel(status)}</span>
          </div>
          <div className="workflowRunDockWindowActions">
            {onExpand && <button type="button" className="secondary small" aria-label="放大运行日志" onClick={onExpand}>放大</button>}
            {onClose && <button type="button" className="secondary small" onClick={onClose}>收起</button>}
          </div>
        </div>
        <div className="workflowRunDockMeta">
          {activeRunID && <code title={`运行 ID：${activeRunID}`}>{activeRunID}</code>}
          {machineLabel && <span title={`执行机器：${machineLabel}`}>执行机器：{machineLabel}</span>}
        </div>
        <div className="workflowRunDockActions">
          <button
            type="button"
            className={overviewOpen ? 'secondary active' : 'secondary'}
            aria-pressed={overviewOpen}
            title={focusNodeID ? '查看该节点的结论、参数与单节点重试' : '查看这次运行的结论统计'}
            onClick={() => setOverviewOpen(open => !open)}
          >概览</button>
          {recoveryAvailable && (
            <button
              type="button"
              className={recoveryOpen ? 'secondary active' : 'secondary'}
              aria-pressed={recoveryOpen}
              title="配置已变更但后续检查失败时的重试、回滚与通知处理"
              onClick={toggleRecovery}
            >恢复</button>
          )}
          {!isRunning && onClearRunOverlay && <button type="button" className="secondary" onClick={onClearRunOverlay}>关闭运行结果</button>}
        </div>
      </header>
      {focusNodeID && (
        <div className="workflowRunFocusRow">
          <span className="workflowRunFocusChip" title={`聚焦：${focusNodeName || focusNodeID}`}>
            聚焦：{focusNodeName || focusNodeID}{focusNodeStatus ? `（${runStatusLabel(focusNodeStatus)}）` : ''}
          </span>
          {onClearFocus && <button type="button" className="secondary small" onClick={onClearFocus}>跟随运行</button>}
        </div>
      )}
      {overviewOpen && (
        <div className="workflowRunPanelSection nodeRunOverviewSection">
          {focusNodeID ? (
            <NodeOverview step={steps[steps.length - 1]} run={run} steps={steps} node={node} rerunDisabledReason={rerunDisabledReason} onRerun={onRerun} />
          ) : (
            <RunOverview record={record} counts={counts} machineLabel={machineLabel} activeRunID={activeRunID} supportHref={supportHref} />
          )}
        </div>
      )}
      {recoveryOpen && (
        <div className="workflowRunPanelSection nodeRunRecovery">
          {recoveryPanel || <div className="empty small">正在读取待处理运行...</div>}
        </div>
      )}
      <div className="workflowRunPanelLogs">
        <RunLogView
          key={activeRunID || record?.id || 'run'}
          items={logItems}
          nodeNames={runNodeNames}
          runningOwnerID={runningNodeID}
          focusOwnerID={focusNodeID}
          runID={activeRunID || record?.id || 'run'}
          scopeToggle={false}
        />
      </div>
    </section>
  )
}

function NodeOverview({step, run, steps, node, rerunDisabledReason, onRerun}) {
  const params = node?.data?.params || {}
  const paramEntries = Object.keys(params)
  const duration = stepDurationText(step)
  const error = run?.error || step?.error || ''
  const skippedReason = run?.skippedReason || step?.skipped_reason || ''
  return (
    <div className="nodeRunOverview">
      <dl className="nodeRunFacts">
        <div><dt>步骤</dt><dd>{step?.type || '-'}</dd></div>
        <div><dt>工具</dt><dd>{step?.tool || '-'}</dd></div>
        <div><dt>执行机器</dt><dd>{step?.host_name || step?.host_id || '本机'}</dd></div>
        <div><dt>开始</dt><dd>{formatClock(step?.started_at)}</dd></div>
        <div><dt>结束</dt><dd>{formatClock(step?.ended_at)}</dd></div>
        <div><dt>耗时</dt><dd>{duration || '-'}</dd></div>
        {Number(step?.http_status) > 0 && <div><dt>HTTP</dt><dd>{step.http_status}</dd></div>}
        {step?.matched_case && <div><dt>命中分支</dt><dd>{step.matched_case}</dd></div>}
        {Number(step?.loop_iterations) > 0 && <div><dt>循环次数</dt><dd>{step.loop_iterations}</dd></div>}
        {steps.length > 1 && <div><dt>迭代步骤</dt><dd>{steps.length} 条</dd></div>}
      </dl>
      {error && <div className="nodeRunError"><strong>错误</strong><span>{error}</span></div>}
      {skippedReason && <div className="nodeRunNotice"><strong>跳过原因</strong><span>{skippedReason}</span></div>}
      <div className="nodeRunParams">
        <strong>节点参数</strong>
        {paramEntries.length === 0 ? <span className="empty small">该节点没有参数。</span> : (
          <ul>
            {paramEntries.map(key => (
              <li key={key}><code>{key}</code><span>{formatParamValue(params[key])}</span></li>
            ))}
          </ul>
        )}
      </div>
      {onRerun && (
        <div className="nodeRunActions">
          <button type="button" className="secondary" disabled={Boolean(rerunDisabledReason)} title={rerunDisabledReason || undefined} onClick={() => onRerun()}>重试该节点</button>
          {rerunDisabledReason && <span className="workflowTopBarHint">{rerunDisabledReason}</span>}
        </div>
      )}
    </div>
  )
}

function RunOverview({record, counts, machineLabel, activeRunID, supportHref}) {
  const skipped = Number(counts.skipped || 0) + Number(counts.waiting || 0)
  return (
    <div className="nodeRunOverview">
      <dl className="nodeRunFacts">
        <div><dt>运行 ID</dt><dd><code>{activeRunID || '-'}</code></dd></div>
        <div><dt>开始</dt><dd>{formatClock(record?.started_at)}</dd></div>
        <div><dt>结束</dt><dd>{formatClock(record?.ended_at)}</dd></div>
        <div><dt>耗时</dt><dd>{runDurationText(record?.started_at, record?.ended_at) || '-'}</dd></div>
        <div><dt>执行机器</dt><dd>{machineLabel || '-'}</dd></div>
        <div><dt>成功 / 失败</dt><dd>{counts.succeeded || 0} / {counts.failed || 0}</dd></div>
        <div><dt>运行中 / 未执行</dt><dd>{counts.running || 0} / {skipped}</dd></div>
      </dl>
      {record?.error && <div className="nodeRunError"><strong>运行错误</strong><span>{record.error}</span></div>}
      {supportHref && (
        <div className="nodeRunActions">
          <a className="secondary" href={supportHref}>导出支持包</a>
        </div>
      )}
    </div>
  )
}

function formatClock(value) {
  const date = new Date(value || '')
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleTimeString()
}

function formatParamValue(value) {
  if (value === null || value === undefined) return ''
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}
