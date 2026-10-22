// 工作流节点目录（单一定义源）
//
// 画布节点分为两个层次：
//   1. 内置编排节点（本文件 controlNodeCatalog）：由平台自身实现，不依赖插件登记。
//      再按用途分成三类，便于在选择面板分区展示、在画布上用颜色和形状区分：
//        flow    流程控制：只决定执行路径与顺序，不执行业务动作
//        data    数据与文件：运行前后搬运、沉淀、解析文件与配置
//        builtin 内置能力：平台自带的执行器（HTTP 调用、脚本、通知等）
//   2. 插件工具节点：由插件注册表提供，通过 tool 字段引用。
//
// 新增节点时只需在 controlNodeCatalog 增加一条元数据；画布渲染、选择面板分组、
// 悬浮工具栏分区、搜索都会自动跟随，不要再在组件里按 type 写分支。

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

export const nodeCategories = [
  {
    id: 'flow',
    label: '流程控制',
    hint: '分支 / 并行 / 合流 / 循环',
    accent: '#4f46e5',
    surface: '#eef1ff',
    shape: 'decision',
    description: '只决定执行路径与顺序，本身不执行业务动作'
  },
  {
    id: 'data',
    label: '数据与文件',
    hint: '上传 / 提取 / 解析',
    accent: '#0f766e',
    surface: '#e8f6f3',
    shape: 'data',
    description: '在运行前后搬运、沉淀和解析文件与配置'
  },
  {
    id: 'builtin',
    label: '内置能力',
    hint: 'HTTP / 脚本 / 通知',
    accent: '#b45309',
    surface: '#fdf1e2',
    shape: 'builtin',
    description: '平台自带、无需插件登记即可使用的执行节点'
  }
]

const fallbackCategory = {
  id: 'other',
  label: '编排节点',
  hint: '',
  accent: '#4b5563',
  surface: '#f2f4f7',
  shape: 'gateway',
  description: '未分类的编排节点'
}

// 插件工具不是内置编排节点，但在画布上同样需要一个分类参与筛选与高亮。
export const toolNodeCategory = {
  id: 'tool',
  label: '插件工具',
  hint: '插件注册表',
  accent: '#2563eb',
  surface: '#e6f1fb',
  shape: 'process',
  description: '由插件注册表提供、画布上为蓝色矩形的工具节点'
}

export const allNodeCategories = [...nodeCategories, toolNodeCategory]

export const controlNodeCatalog = [
  {
    type: 'condition',
    category: 'flow',
    status: 'ready',
    shape: 'decision',
    symbol: '?',
    title: '条件分支',
    secondary: 'Switch / Case',
    description: '根据上游输出或工作流参数选择后续分支',
    help: '适合根据巡检结果、返回文本、参数值做分流'
  },
  {
    type: 'parallel',
    category: 'flow',
    status: 'ready',
    shape: 'gateway',
    symbol: '+',
    title: '并行分支',
    secondary: 'Parallel',
    description: '将后续任务拆分为多个分支路径',
    help: '用于明确 fan-out 分支结构；当前运行按 DAG 顺序调度'
  },
  {
    type: 'join',
    category: 'flow',
    status: 'ready',
    shape: 'gateway',
    symbol: '∧',
    title: '合流',
    secondary: 'Join',
    description: '等待多个上游分支完成后继续流程',
    help: '用于明确 fan-in 汇聚点；入边完成后节点记为成功'
  },
  {
    type: 'loop',
    category: 'flow',
    status: 'ready',
    shape: 'gateway',
    symbol: '↻',
    title: '循环',
    secondary: 'Loop',
    description: '按固定次数重复执行一个内嵌选择的插件工具',
    help: '执行到循环节点时，按最大次数重复运行已选择的插件工具'
  },
  {
    type: 'upload',
    category: 'data',
    status: 'ready',
    shape: 'data',
    symbol: '↑',
    title: '上传文件',
    secondary: 'Upload',
    description: '运行前上传本地文件、批量文件或目录到平台受控目录',
    help: '上传节点会在工作流启动前选择文件或目录，运行时输出上传结果 JSON'
  },
  {
    type: 'extract_config',
    category: 'data',
    status: 'ready',
    shape: 'data',
    symbol: '⇩',
    title: '提取配置',
    secondary: 'Extract Config',
    description: '从上传结果提取文件到工作流配置目录',
    help: '把上传结果里的文件按映射复制成稳定的工作流配置文件'
  },
  {
    type: 'http_request',
    category: 'builtin',
    status: 'ready',
    shape: 'builtin',
    symbol: '⇄',
    title: 'HTTP 调用',
    secondary: 'CURL / REST',
    description: '直接发起 HTTP 请求并把响应写入节点输出',
    help: '等价于一条 curl 命令：支持方法、Header、Body、期望状态码，响应体可直接被下游引用'
  },
  {
    type: 'shell_script',
    category: 'builtin',
    status: 'planned',
    shape: 'builtin',
    symbol: '$',
    title: 'Shell 脚本',
    secondary: 'Script',
    description: '在目标机上执行内联脚本，无需封装插件',
    help: '规划中：适合一次性排障命令与轻量胶水逻辑'
  },
  {
    type: 'notify',
    category: 'builtin',
    status: 'planned',
    shape: 'builtin',
    symbol: '✉',
    title: '消息通知',
    secondary: 'Notify',
    description: '把上游结果推送到邮件、企业微信等通道',
    help: '规划中：作为流程收尾的通用通知节点'
  }
]

export function controlNodeMeta(type) {
  return controlNodeCatalog.find(item => item.type === type) || null
}

export function controlNodeTitle(type) {
  return controlNodeMeta(type)?.title || type || '编排节点'
}

export function controlNodeHelp(type) {
  return controlNodeMeta(type)?.help || '编排控制节点'
}

export function nodeCategory(categoryID) {
  return nodeCategories.find(item => item.id === categoryID) || fallbackCategory
}

export function controlNodeCategory(type) {
  return nodeCategory(controlNodeMeta(type)?.category)
}

export function isControlReady(control) {
  return Boolean(control) && control.status !== 'planned'
}

// 按分类分组，保持 nodeCategories 的声明顺序；空分组会被跳过。
export function groupControlsByCategory(controls = controlNodeCatalog) {
  return nodeCategories
    .map(category => ({
      category,
      controls: controls.filter(control => control.category === category.id)
    }))
    .filter(group => group.controls.length > 0)
}

export function controlNodeMatchesKeyword(control, keyword) {
  const word = String(keyword || '').trim().toLowerCase()
  if (!word) return true
  return [control.title, control.secondary, control.description, control.help, control.type, nodeCategory(control.category).label]
    .filter(Boolean)
    .some(value => String(value).toLowerCase().includes(word))
}

// 画布节点（React Flow 节点）→ 分类 ID。用于筛选、高亮与统计。
export function flowNodeCategoryID(node) {
  if (!node) return 'other'
  if (node.type === 'toolNode') return 'tool'
  if (node.type === 'conditionNode') return 'flow'
  if (node.type === 'controlNode') return controlNodeCategory(node.data?.controlType).id
  return 'other'
}

export function countNodesByCategory(nodes = []) {
  const counts = {}
  for (const category of allNodeCategories) counts[category.id] = 0
  for (const node of nodes || []) {
    const id = flowNodeCategoryID(node)
    counts[id] = (counts[id] || 0) + 1
  }
  return counts
}
