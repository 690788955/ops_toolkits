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
