// 人工审批等待的统一判定工具（纯 JS，不含 React）。
//
// 为什么单独成文件：工作流编辑器的运行日志面板和「运行记录」页都要判断“这次运行是不是
// 卡在审批节点上”，而 main.jsx 对 WorkflowEditor 是懒加载的——为了复用函数去 import
// 编辑器，会把整块画布打进首屏。放到独立模块后两边都能引，首屏体积不变。

// 一次运行可能留下多条审批记录（历史上通过/驳回的也在里面），只有最后一条仍在 waiting 的
// 才需要人处理；运行结束后 approvals 里不再有 waiting，审批条随之消失。
export function pendingRunApproval(record) {
  const approvals = Array.isArray(record?.approvals) ? record.approvals : []
  const waiting = approvals.filter(item => item?.status === 'waiting' && item.node_id)
  return waiting.length > 0 ? waiting[waiting.length - 1] : null
}

// 列表项和详情的数据形状不同：列表项只有后端给的 awaiting_approval/approval 摘要，
// 详情才有完整 approvals 数组。这里统一成一种返回值，JSX 里就不用手写 .some()。
export function runAwaitingApproval(entry) {
  if (!entry) return null
  const fromDetail = pendingRunApproval(entry)
  if (fromDetail) return fromDetail
  if (entry.awaiting_approval) {
    const approval = entry.approval || {}
    return {node_id: approval.node_id || '', message: approval.message || '', since: approval.since || ''}
  }
  return null
}

// 判断列表项是否处于待审批（徽标、轮询、按钮显隐都用它）。
export function isRunAwaiting(entry) {
  return Boolean(entry?.awaiting_approval) || Boolean(pendingRunApproval(entry))
}

// 把 “开始等待时间” 换算成易读时长。审批常常等几十秒到几小时，展示 3 分钟比展示时间戳
// 更容易让人判断“是不是刚刚才出现的”。非法时间返回空串，让调用方自己决定隐藏这一行。
export function formatApprovalWait(since, now = Date.now()) {
  const started = new Date(since).getTime()
  const current = typeof now === 'number' ? now : new Date(now).getTime()
  if (!Number.isFinite(started) || !Number.isFinite(current)) return ''
  const seconds = Math.max(0, Math.floor((current - started) / 1000))
  if (seconds < 60) return '不到 1 分钟'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} 分钟`
  const hours = Math.floor(minutes / 60)
  const restMinutes = minutes % 60
  if (hours < 24) return restMinutes > 0 ? `${hours} 小时 ${restMinutes} 分` : `${hours} 小时`
  const days = Math.floor(hours / 24)
  const restHours = hours % 24
  return restHours > 0 ? `${days} 天 ${restHours} 小时` : `${days} 天`
}