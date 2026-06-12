export const conditionOperators = [
  {value: 'eq', label: '等于'},
  {value: 'neq', label: '不等于'},
  {value: 'contains', label: '包含'},
  {value: 'not_contains', label: '不包含'},
  {value: 'in', label: '在列表中'},
  {value: 'not_in', label: '不在列表中'},
  {value: 'exists', label: '存在'},
  {value: 'empty', label: '为空'}
]

export function defaultCondition() {
  return {
    input: '',
    cases: [
      {id: 'case1', name: '分支 1', operator: 'contains', values: []},
      {id: 'case2', name: '分支 2', operator: 'contains', values: []}
    ],
    default_case: 'default'
  }
}

export function defaultLoop() {
  return {tool: '', params: {}, max_iterations: 3}
}

export function normalizeLoopConfig(loop) {
  const params = loop?.params && typeof loop.params === 'object' && !Array.isArray(loop.params) ? loop.params : {}
  return {
    tool: String(loop?.tool || '').trim(),
    params,
    max_iterations: clampLoopIterations(loop?.max_iterations || loop?.maxIterations || 1)
  }
}

export function clampLoopIterations(value) {
  const parsed = Number.parseInt(value, 10)
  if (!Number.isFinite(parsed)) return 1
  return Math.min(20, Math.max(1, parsed))
}

export function workflowScopeCategory(value, fallbackCategory = '') {
  if (value === 'global') return 'global'
  return value || fallbackCategory || 'global'
}

export function normalizeTags(tags) {
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

export function conditionBranchRows(condition) {
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
    rows.push({key: 'default', handleID: 'default', label: '默认分支', meta: 'default', kind: 'default', disabled: false})
  } else {
    rows.push({key: 'default-disabled', handleID: '', label: '默认分支', meta: '未启用', kind: 'default', disabled: true})
  }
  return rows
}

export function conditionCaseLabel(condition, caseID) {
  if (!caseID) return ''
  if (caseID === 'default') return 'default'
  const item = (condition?.cases || []).find(item => item.id === caseID)
  return item ? (item.name || item.id) : caseID
}

export function buildWorkflowDraft(workflow, nodes, edges, category, parameters) {
  const draftEdges = (edges || []).length > 0 ? edges : buildSequentialFlowEdges(nodes)
  return {
    ...workflow,
    category: workflowScopeCategory(workflow.category, category),
    tags: normalizeTags(workflow.tags || []),
    parameters: parameters || workflow.parameters || [],
    nodes: nodes.map(node => {
      if (node.type === 'conditionNode') {
        return {id: node.id, type: 'condition', name: node.data.name || node.id, condition: node.data.condition || defaultCondition()}
      }
      if (node.type === 'controlNode') {
        const draftNode = {id: node.id, type: node.data.controlType, name: node.data.name || node.id}
        if (node.data.controlType === 'loop') draftNode.loop = normalizeLoopConfig(node.data.loop || defaultLoop())
        return draftNode
      }
      return {id: node.id, type: 'tool', name: node.data.name || node.id, tool: node.data.tool, params: node.data.params || {}, on_failure: node.data.on_failure || 'stop'}
    }),
    edges: draftEdges.map(edge => {
      const sourceNode = nodes.find(node => node.id === edge.source)
      const out = {from: edge.source, to: edge.target}
      const edgeCase = sourceNode?.type === 'conditionNode' ? (edge.data?.case || edge.sourceHandle || '') : ''
      if (edgeCase) out.case = edgeCase
      return out
    })
  }
}

export function canBuildSequentialFlowEdges(nodes, edges = []) {
  return (nodes || []).length > 1 && (edges || []).length === 0 && (nodes || []).every(isLinearFlowNode)
}

export function buildSequentialFlowEdges(nodes, edges = []) {
  if (!canBuildSequentialFlowEdges(nodes, edges)) return []
  const ordered = orderedFlowNodes(nodes)
  return ordered.slice(1).map((node, index) => {
    const source = ordered[index]
    return {
      id: `${source.id}-${node.id}`,
      source: source.id,
      target: node.id
    }
  })
}

function isLinearFlowNode(node) {
  if (node?.type === 'toolNode') return true
  return node?.type === 'controlNode' && node.data?.controlType === 'loop'
}

function orderedFlowNodes(nodes) {
  return [...(nodes || [])]
    .map((node, index) => ({node, index}))
    .sort((left, right) => compareNodePosition(left, right))
    .map(item => item.node)
}

function compareNodePosition(left, right) {
  const leftX = numberOrNull(left.node?.position?.x)
  const rightX = numberOrNull(right.node?.position?.x)
  const leftY = numberOrNull(left.node?.position?.y)
  const rightY = numberOrNull(right.node?.position?.y)
  if (leftX !== null && rightX !== null && leftX !== rightX) return leftX - rightX
  if (leftY !== null && rightY !== null && leftY !== rightY) return leftY - rightY
  if (leftX !== null && rightX === null) return -1
  if (leftX === null && rightX !== null) return 1
  return left.index - right.index
}

function numberOrNull(value) {
  const number = Number(value)
  return Number.isFinite(number) ? number : null
}

export function autoLayoutNodes(nodes, edges) {
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

  const queue = nodes.filter(node => (incomingCounts.get(node.id) || 0) === 0).map(node => node.id)
  const visited = new Set()
  while (queue.length > 0) {
    const nodeID = queue.shift()
    if (visited.has(nodeID)) continue
    visited.add(nodeID)
    const nextDepth = (depths.get(nodeID) || 0) + 1
    ;(children.get(nodeID) || []).sort((left, right) => (nodeOrder.get(left) || 0) - (nodeOrder.get(right) || 0)).forEach(childID => {
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
  const orderedLayers = Array.from(layers.entries()).sort(([left], [right]) => left - right).map(([, layerNodes]) => layerNodes.sort((left, right) => (nodeOrder.get(left.id) || 0) - (nodeOrder.get(right.id) || 0)))
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
  return nodes.map(node => ({...node, position: positions.get(node.id) || node.position || {x: 80, y: 80}}))
}

export function autoLayoutNodeSize(node) {
  if (node.type === 'conditionNode') {
    const branchCount = conditionBranchRows(node.data.condition || defaultCondition()).length
    return {width: 440, height: Math.max(156, 72 + branchCount * 42)}
  }
  if (node.type === 'controlNode') return {width: 250, height: 82}
  return {width: 210, height: 74}
}

export function validateConditionDraft(nodes, edges) {
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

export function validateControlDraft(nodes, edges, tools = []) {
  const errors = []
  const toolMap = new Map((tools || []).map(tool => [tool.id, tool]))
  nodes.filter(node => node.type === 'controlNode').forEach(node => {
    if (node.data.controlType === 'loop') {
      const loop = normalizeLoopConfig(node.data.loop || {})
      if (!loop.tool) errors.push(`循环节点 ${node.id} 请选择循环工具。`)
      if (loop.tool && !toolMap.has(loop.tool)) errors.push(`循环节点 ${node.id} 引用了不存在的工具：${loop.tool}`)
      if (!Number.isInteger(loop.max_iterations) || loop.max_iterations < 1 || loop.max_iterations > 20) errors.push(`循环节点 ${node.id} 的最大循环次数必须在 1 到 20 之间。`)
    }
    if (node.data.controlType === 'parallel' && !edges.some(edge => edge.source === node.id)) errors.push(`并行分支节点 ${node.id} 至少需要一条出边。`)
    if (node.data.controlType === 'join' && !edges.some(edge => edge.target === node.id)) errors.push(`合流节点 ${node.id} 至少需要一条入边。`)
  })
  return errors
}

export function defaultParams(parameters) {
  const out = {}
  ;(parameters || []).forEach(param => { out[param.name] = param.default === undefined || param.default === null ? '' : param.default })
  return out
}

export function parseJSONList(value) {
  try {
    const parsed = JSON.parse(value || '[]')
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

export function findOutOfScopeToolNodes(nodes, tools, scopedCategory) {
  if (!scopedCategory) return []
  const toolMap = new Map((tools || []).map(tool => [tool.id, tool]))
  return nodes
    .filter(node => node.type === 'toolNode' || (node.type === 'controlNode' && node.data.controlType === 'loop'))
    .map(node => {
      const toolID = node.type === 'controlNode' ? node.data.loop?.tool : node.data.tool
      return {node, toolID, tool: toolMap.get(toolID)}
    })
    .filter(item => item.tool && item.tool.category !== scopedCategory)
    .map(item => ({nodeID: item.node.id, toolID: item.toolID, scopeName: scopedCategory}))
}

export function findMissingRequiredNodeParams(nodes, tools) {
  const toolMap = new Map((tools || []).map(tool => [tool.id, tool]))
  const missing = []
  nodes.forEach(node => {
    if (node.type === 'toolNode') {
      const tool = toolMap.get(node.data.tool)
      ;(tool?.parameters || []).forEach(param => {
        if (!param.required) return
        const value = node.data.params?.[param.name]
        if (value === undefined || value === null || String(value).trim() === '') {
          missing.push({nodeID: node.id, toolName: tool.name || tool.id, paramName: param.name})
        }
      })
      return
    }
    if (node.type === 'controlNode' && node.data.controlType === 'loop') {
      const loop = normalizeLoopConfig(node.data.loop || defaultLoop())
      const tool = toolMap.get(loop.tool)
      ;(tool?.parameters || []).forEach(param => {
        if (!param.required) return
        const value = loop.params?.[param.name]
        if (value === undefined || value === null || String(value).trim() === '') {
          missing.push({nodeID: node.id, toolName: tool.name || tool.id, paramName: param.name})
        }
      })
    }
  })
  return missing
}
