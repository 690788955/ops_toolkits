import {describe, expect, it} from 'vitest'
import {
  controlNodeCatalog,
  controlNodeCategory,
  controlNodeMatchesKeyword,
  controlUnavailableReason,
  countNodesByCategory,
  flowNodeCategoryID,
  groupControlsByCategory,
  isControlReady,
  nodeCategories
} from './catalog.js'

describe('节点分类目录', () => {
  it('每个节点都归属到已声明的分类', () => {
    const ids = new Set(nodeCategories.map(category => category.id))
    for (const control of controlNodeCatalog) {
      expect(ids.has(control.category)).toBe(true)
    }
  })

  it('按分类分组时保持声明顺序且不产生空分组', () => {
    const groups = groupControlsByCategory(controlNodeCatalog)
    expect(groups.map(group => group.category.id)).toEqual(['flow', 'data', 'builtin'])
    expect(groups[0].controls.map(control => control.type)).toEqual(['condition', 'loop', 'wait', 'approval'])
    expect(groups[1].controls.map(control => control.type)).toEqual(['upload', 'extract_config', 'file_checksum', 'archive'])
    expect(groups.every(group => group.controls.length > 0)).toBe(true)
  })

  it('并行/合流不再作为新建节点候选', () => {
    expect(controlNodeCatalog.some(control => ['parallel', 'join'].includes(control.type))).toBe(false)
    expect(controlUnavailableReason(controlNodeCatalog.find(control => control.type === 'notify'))).toBe('')
    expect(controlUnavailableReason(controlNodeCatalog.find(control => control.type === 'condition'))).toBe('')
  })

  it('内置能力与数据节点都已可执行', () => {
    const builtin = controlNodeCatalog.filter(control => control.category === 'builtin')
    expect(builtin.map(control => control.type)).toEqual(['http_request', 'shell_script', 'notify'])
    expect(isControlReady(builtin[0])).toBe(true)
    expect(isControlReady(builtin[1])).toBe(true)
    expect(isControlReady(builtin[2])).toBe(true)
    const data = controlNodeCatalog.filter(control => control.category === 'data')
    expect(data.every(control => isControlReady(control))).toBe(true)
  })

  it('搜索支持英文别名与中文用途词', () => {
    const byAlias = controlNodeCatalog.filter(control => controlNodeMatchesKeyword(control, 'unzip'))
    expect(byAlias.map(control => control.type)).toEqual(['archive'])
    const byPurpose = controlNodeCatalog.filter(control => controlNodeMatchesKeyword(control, '轮询'))
    expect(byPurpose.map(control => control.type)).toEqual(['wait'])
    expect(controlNodeCatalog.filter(control => controlNodeMatchesKeyword(control, 'sha256')).map(control => control.type)).toEqual(['file_checksum'])
  })

  it('已上线节点都带有形状与符号元数据', () => {
    for (const control of controlNodeCatalog) {
      expect(control.shape).toBeTruthy()
      expect(control.symbol).toBeTruthy()
    }
  })

  it('人工审批节点在流程控制分类里可直接添加，并能被中英文别名搜到', () => {
    const approval = controlNodeCatalog.find(control => control.type === 'approval')
    expect(approval).toBeTruthy()
    expect(approval.category).toBe('flow')
    expect(isControlReady(approval)).toBe(true)
    expect(approval.shape).toBe('gateway')
    expect(approval.symbol).toBe('✓')
    expect(controlNodeMatchesKeyword(approval, '审批')).toBe(true)
    expect(controlNodeMatchesKeyword(approval, 'confirm')).toBe(true)
    expect(controlNodeMatchesKeyword(approval, 'gate')).toBe(true)
    expect(controlNodeCatalog.filter(control => controlNodeMatchesKeyword(control, '人工')).map(control => control.type))
      .toEqual(expect.arrayContaining(['approval']))
  })

  it('未知类型回落到未分类而不是抛错', () => {
    expect(controlNodeCategory('not_exists').id).toBe('other')
    expect(isControlReady(null)).toBe(false)
  })

  it('搜索可按分类名命中节点', () => {
    const keyword = '内置能力'
    const matched = controlNodeCatalog.filter(control => controlNodeMatchesKeyword(control, keyword))
    expect(matched.every(control => control.category === 'builtin')).toBe(true)
    expect(matched.length).toBeGreaterThan(0)
  })
})

describe('画布节点分类统计', () => {
  const nodes = [
    {id: 'a', type: 'toolNode', data: {}},
    {id: 'b', type: 'conditionNode', data: {}},
    {id: 'c', type: 'controlNode', data: {controlType: 'upload'}},
    {id: 'd', type: 'controlNode', data: {controlType: 'loop'}}
  ]

  it('按节点类型映射到分类', () => {
    expect(flowNodeCategoryID(nodes[0])).toBe('tool')
    expect(flowNodeCategoryID(nodes[1])).toBe('flow')
    expect(flowNodeCategoryID(nodes[2])).toBe('data')
    expect(flowNodeCategoryID(nodes[3])).toBe('flow')
  })

  it('未知节点回落到 other', () => {
    expect(flowNodeCategoryID(null)).toBe('other')
    expect(flowNodeCategoryID({id: 'x', type: 'unknownNode'})).toBe('other')
  })

  it('统计结果包含全部分类的计数', () => {
    expect(countNodesByCategory(nodes)).toEqual({flow: 2, data: 1, builtin: 0, tool: 1})
  })
})
