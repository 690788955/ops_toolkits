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

export const controlNodeCatalog = [
  {
    type: 'condition',
    title: '条件分支',
    secondary: 'Switch / Case',
    description: '根据上游输出或工作流参数选择后续分支',
    help: '适合根据巡检结果、返回文本、参数值做分流',
    enabled: true
  },
  {
    type: 'parallel',
    title: '并行分支',
    secondary: 'Parallel',
    description: '将后续任务拆分为多个分支路径',
    help: '用于明确 fan-out 分支结构；当前运行按 DAG 顺序调度',
    enabled: true
  },
  {
    type: 'join',
    title: '合流',
    secondary: 'Join',
    description: '等待多个上游分支完成后继续流程',
    help: '用于明确 fan-in 汇聚点；入边完成后节点记为成功',
    enabled: true
  },
  {
    type: 'loop',
    title: '循环',
    secondary: 'Loop',
    description: '按固定次数重复执行一个内嵌选择的插件工具',
    help: '执行到循环节点时，按最大次数重复运行已选择的插件工具',
    enabled: true
  },
  {
    type: 'upload',
    title: '上传文件',
    secondary: 'Upload',
    description: '运行前上传本地文件、批量文件或目录到平台受控目录',
    help: '上传节点会在工作流启动前选择文件或目录，运行时输出上传结果 JSON',
    enabled: true
  },
  {
    type: 'extract_config',
    title: '提取配置',
    secondary: 'Extract Config',
    description: '从上传结果提取文件到工作流配置目录',
    help: '把上传结果里的文件按映射复制成稳定的工作流配置文件',
    enabled: true
  }
]

export function controlNodeTitle(type) {
  const control = controlNodeCatalog.find(item => item.type === type)
  return control?.title || type || '编排节点'
}

export function controlNodeHelp(type) {
  const control = controlNodeCatalog.find(item => item.type === type)
  return control?.help || '编排控制节点'
}
