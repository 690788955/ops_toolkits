export function buildConfigItemCollections(catalog, activeCategory) {
  return {
    toolConfigItems: buildToolConfigItems(catalog, activeCategory),
    workflowConfigItems: buildWorkflowConfigItems(catalog, activeCategory)
  }
}

function buildToolConfigItems(catalog, activeCategory) {
  const items = []

  const plugins = catalog?.plugins || []
  const tools = catalog?.tools || []
  const workflows = catalog?.workflows || []

  plugins.forEach(plugin => {
    const configFiles = []
    const seenFiles = new Set()
    tools.forEach(tool => {
      if (tool.source?.plugin_id !== plugin.id) {
        return
      }
      ;(tool.config_file_entries || tool.config_files || []).forEach(file => {
        const key = typeof file === 'string' ? file : (file.id || file.path)
        if (key && !seenFiles.has(key)) {
          seenFiles.add(key)
          configFiles.push(normalizeConfigFileItem(file, plugin.id))
        }
      })
    })

    if (configFiles.length === 0) {
      return
    }

    const relatedCategories = new Set()
    tools.forEach(tool => {
      if (tool.source?.plugin_id === plugin.id && tool.category) {
        relatedCategories.add(tool.category)
      }
    })
    workflows.forEach(wf => {
      if (wf.source?.plugin_id === plugin.id && wf.category) {
        relatedCategories.add(wf.category)
      }
    })

    if (activeCategory && !relatedCategories.has(activeCategory)) {
      return
    }

    configFiles.sort((a, b) => `${a.path || ''}${a.label || ''}`.localeCompare(`${b.path || ''}${b.label || ''}`))

    items.push({
      type: 'plugin',
      id: plugin.id,
      name: plugin.name || plugin.id,
      typeLabel: '插件',
      description: plugin.description || `插件 ${plugin.id} 声明的配置文件`,
      files: configFiles,
      disabled: plugin.disabled,
      version: plugin.version,
      relatedCategories: Array.from(relatedCategories)
    })
  })

  return items
}

function buildWorkflowConfigItems(catalog, activeCategory) {
  return (catalog?.workflows || [])
    .filter(workflow => !activeCategory || workflow.category === activeCategory)
    .filter(workflow => (workflow.config_files || []).length > 0)
    .map(workflow => {
      const files = (workflow.config_files || []).map(normalizeWorkflowConfigListItem)
      return {
        type: 'workflow',
        id: workflow.id,
        name: workflow.name || workflow.id,
        typeLabel: '工作流',
        description: workflow.description || `工作流 ${workflow.id} 的真实挂载路径配置文件`,
        files,
        category: workflow.category,
        source: workflow.source
      }
    })
    .sort((left, right) => String(left.name || left.id).localeCompare(String(right.name || right.id), 'zh-CN'))
}

function normalizeConfigFileItem(file, pluginID) {
  if (typeof file === 'string') {
    return {path: `plugins/${pluginID}/${file}`, label: '配置文件'}
  }
  const scope = file.scope || 'plugin'
  const configDir = file.config_dir || (scope === 'plugin' ? 'config' : '')
  const itemPath = file.path || file.id
  const displayPath = scope === 'host_absolute' ? [configDir, itemPath].filter(Boolean).join('/') : `plugins/${pluginID}/${[configDir, itemPath].filter(Boolean).join('/')}`
  const accessLabel = file.access === 'read_write' ? '可读写' : '只读'
  return {
    path: displayPath,
    label: file.label || (scope === 'host_absolute' ? `宿主配置文件（${accessLabel}）` : '配置文件')
  }
}

function normalizeWorkflowConfigListItem(file) {
  if (typeof file === 'string') {
    return {path: file, label: '配置文件'}
  }
  const path = file.path || file.id || ''
  const configDir = file.display_root || file.displayRoot || file.config_dir || file.configDir || ''
  const displayPath = file.display_path || file.displayPath || [configDir, path].filter(Boolean).join('/') || path
  return {
    path: displayPath,
    label: file.label || '配置文件'
  }
}
