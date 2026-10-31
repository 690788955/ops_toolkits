// 节点目录（单一定义源）
//
// 画布节点分为两个层次：
//   1. 内置编排节点（本文件 controlNodeCatalog）：由平台自身实现，不依赖插件登记。
//      再按用途分成三类，便于在选择面板分区展示、在画布上用颜色和形状区分：
//        flow    流程控制：决定执行路径、顺序与等待，不搬业务数据
//        data    数据与文件：运行前后搬运、沉淀、校验、解析文件与配置
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
    hint: '分支 / 循环 / 等待 / 审批',
    accent: '#4f46e5',
    surface: '#eef1ff',
    shape: 'decision',
    description: '决定执行路径、顺序与等待条件，不直接搬运业务数据'
  },
  {
    id: 'data',
    label: '数据与文件',
    hint: '上传 / 提取 / 校验 / 解压',
    accent: '#0f766e',
    surface: '#e8f6f3',
    shape: 'data',
    description: '在运行前后搬运、沉淀、校验与解析文件'
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
    help: '适合根据巡检结果、返回文本、参数值做分流；分支可以落在条件节点所在的阶段或更靠后的阶段，未命中的分支会被跳过',
    aliases: ['condition', 'switch', 'case', 'if', 'branch', '条件', '分支', '判断']
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
    help: '执行到循环节点时，按最大次数重复运行已选择的插件工具',
    aliases: ['loop', 'repeat', 'iterate', '循环', '重复', '迭代']
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
    help: '上传节点会在工作流启动前选择文件或目录，运行时输出上传结果 JSON',
    aliases: ['upload', 'file', 'artifact', '上传', '文件', '目录', '产物']
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
    help: '把上传结果里的文件按映射复制成稳定的工作流配置文件',
    aliases: ['config', 'extract', '配置', '提取', '沉淀']
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
    help: '等价于一条 curl 命令：支持方法、Header、Body、期望状态码，响应体可直接被下游引用',
    aliases: ['http', 'https', 'curl', 'rest', 'api', 'request', 'webhook', '请求', '接口']
  },
  {
    type: 'shell_script',
    category: 'builtin',
    status: 'ready',
    shape: 'builtin',
    symbol: '$',
    title: 'Shell 脚本',
    secondary: 'Script',
    description: '在目标机上执行内联脚本，无需封装插件',
    help: '适合一次性排障命令与轻量胶水逻辑；可指定工作目录、环境变量与超时',
    aliases: ['shell', 'bash', 'cmd', 'command', '脚本', '命令', '排障', '内联']
  },
  {
    type: 'file_checksum',
    category: 'data',
    status: 'ready',
    shape: 'data',
    symbol: '#',
    title: '文件校验',
    secondary: 'Checksum',
    description: '计算文件 MD5/SHA256，可要求与期望摘要一致',
    help: '用于校验上传包、发布产物是否完整；摘要会写入节点输出供下游引用',
    aliases: ['checksum', 'md5', 'sha256', 'hash', 'digest', '摘要', '哈希', '校验和', '完整性']
  },
  {
    type: 'archive',
    category: 'data',
    status: 'ready',
    shape: 'data',
    symbol: '⧉',
    title: '解压文件',
    secondary: 'Archive',
    description: '把 tar、tar.gz、zip 解压到目标目录',
    help: '自动拦截绝对路径与 .. 逃逸；可去掉顶层目录并控制是否覆盖',
    aliases: ['archive', 'unzip', 'untar', 'extract', 'tar', 'zip', '解压', '解包', '压缩包']
  },
  {
    type: 'wait',
    category: 'flow',
    status: 'ready',
    shape: 'gateway',
    symbol: '⏱',
    title: '等待 / 轮询',
    secondary: 'Wait / Poll',
    description: '固定等待，或有限轮询 HTTP 状态与文件出现',
    help: '轮询支持最大尝试次数与最大等待时长，运行取消会立即结束等待',
    aliases: ['wait', 'sleep', 'poll', 'delay', 'retry', '等待', '轮询', '延时', '重试']
  },
  {
    type: 'notify',
    category: 'builtin',
    status: 'ready',
    shape: 'builtin',
    symbol: '✉',
    title: '消息通知',
    secondary: 'Notify',
    description: '把上游结果推送到 Webhook、企业微信或邮件',
    help: '标题与正文支持运行期模板；发送失败会让节点失败，适合放在流程末尾',
    aliases: ['notify', 'webhook', 'wecom', 'wechat', 'email', 'smtp', 'mail', 'alert', '通知', '告警', '邮件', '企业微信', '推送']
  },
  {
    type: 'approval',
    category: 'flow',
    status: 'ready',
    shape: 'gateway',
    symbol: '✓',
    title: '人工审批',
    secondary: 'Manual Approval',
    description: '执行到这里暂停流水线，等待人工点击确认后才继续',
    help: '停机点：审批前可以先改配置文件或调整前置服务，确认后继续执行下游；驳回会让运行失败',
    aliases: ['approval', 'approve', 'reject', 'confirm', 'manual', 'gate', '审批', '确认', '人工', '暂停', '卡点', '驳回']
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

export function controlUnavailableReason(control) {
  if (control?.status === 'planned') return '规划中：暂不可添加'
  return ''
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
  return [control.title, control.secondary, control.description, control.help, control.type, nodeCategory(control.category).label, ...(control.aliases || [])]
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
