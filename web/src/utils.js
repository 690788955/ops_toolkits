export function summarizeAPIResponse(body, fallback) {
  if (body?.data?.valid === false) return readableValidationMessages(body.data).join('\n')
  if (body?.message) return body.message
  if (body?.error) return body.error
  return fallback
}

export function readableAPIError(err, fallback) {
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

export function combineWorkflowStepLogs(steps, record) {
  const rendered = []
  ;(record?.steps || []).forEach(step => {
    const parts = [`[${step.id}] ${displayStepType(step.type)} ${step.status}`]
    if (step.type === 'condition') {
      if (step.condition_input !== undefined) parts.push(`条件输入: ${step.condition_input}`)
      if (step.matched_case) parts.push(`命中分支: ${step.matched_case}`)
    } else if (step.type === 'loop') {
      if (step.tool) parts.push(`循环工具: ${step.tool}`)
      if (step.loop_target) parts.push(`兼容目标: ${step.loop_target}`)
      if (step.loop_iterations) parts.push(`循环次数: ${step.loop_iterations}`)
    } else if (step.type === 'parallel' || step.type === 'join') {
      parts.push('编排控制节点已完成')
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
  if (type === 'parallel') return '编排节点/并行分支'
  if (type === 'join') return '编排节点/合流'
  if (type === 'loop') return '编排节点/循环'
  return '工具节点'
}

export function tagsForEntries(entries) {
  const tags = new Set()
  entries.forEach(entry => (entry.tags || []).forEach(tag => tags.add(tag)))
  return Array.from(tags).sort((a, b) => a.localeCompare(b, 'zh-CN'))
}

export function filterEntries(entries, searchText, activeTag) {
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