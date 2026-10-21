import {describe, expect, it} from 'vitest'
import {buildConfigItemCollections} from './configItems.js'

const catalog = {
  plugins: [
    {id: 'plugin.template', name: '插件开发模板'},
    {id: 'user.workflows', name: '用户工作流'}
  ],
  tools: [
    {
      id: 'plugin.template.with-config',
      category: 'example',
      config_files: ['example.conf'],
      source: {plugin_id: 'plugin.template'}
    }
  ],
  workflows: [
    {
      id: 'everisk.pkg',
      category: 'docker',
      config_files: [{id: 'hosts', path: 'hosts'}],
      source: {plugin_id: 'user.workflows'}
    },
    {
      id: 'without-config',
      category: 'docker',
      config_files: [],
      source: {plugin_id: 'user.workflows'}
    }
  ]
}

function counts(activeCategory) {
  const {toolConfigItems, workflowConfigItems} = buildConfigItemCollections(catalog, activeCategory)
  return [toolConfigItems.length, workflowConfigItems.length, toolConfigItems.length + workflowConfigItems.length]
}

describe('配置项统计', () => {
  it('按当前分类统计顶部配置数量', () => {
    expect(counts('example')).toEqual([1, 0, 1])
    expect(counts('docker')).toEqual([0, 1, 1])
    expect(counts('')).toEqual([1, 1, 2])
  })

  it('不计入没有配置文件的工作流', () => {
    const {workflowConfigItems} = buildConfigItemCollections(catalog, 'docker')
    expect(workflowConfigItems.map(item => item.id)).toEqual(['everisk.pkg'])
  })
})

describe('工作流引用插件配置', () => {
  const catalogWithReferences = {
    ...catalog,
    workflows: [
      ...catalog.workflows,
      {
        id: 'referenced-only',
        category: 'docker',
        config_files: [],
        referenced_plugins: [
          {
            id: 'docker.exec',
            name: 'Docker 插件',
            tools: ['docker.exec.run'],
            config_files: [{id: 'daemon.yaml', path: 'daemon.yaml', config_dir: 'config', scope: 'plugin', access: 'read_write'}]
          },
          {id: 'system.core', name: '系统插件', tools: ['system.core.info'], config_files: []}
        ],
        source: {plugin_id: 'user.workflows'}
      }
    ]
  }

  it('只有引用插件配置文件的工作流也会进入工作流配置', () => {
    const {workflowConfigItems} = buildConfigItemCollections(catalogWithReferences, 'docker')
    const item = workflowConfigItems.find(entry => entry.id === 'referenced-only')
    expect(item).toBeTruthy()
    expect(item.files).toEqual([])
    expect(item.referencedPlugins.map(plugin => plugin.id)).toEqual(['docker.exec'])
    expect(item.referencedPlugins[0].files[0].path).toBe('plugins/docker.exec/config/daemon.yaml')
  })
})
